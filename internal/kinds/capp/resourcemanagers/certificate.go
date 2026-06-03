package resourcemanagers

import (
	"fmt"

	certv1alpha1 "github.com/dana-team/cert-external-issuer/api/v1alpha1"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	Certificate                        = "certificate"
	eventCappCertificateCreationFailed = "CertificateCreationFailed"
	eventCappCertificateCreated        = "CertificateCreated"
	PrivateKeySize                     = 4096
	clusterIssuerKind                  = "ClusterIssuer"
	certificateUIDSecretLabelKey       = "networking.internal.knative.dev/certificate-uid"
)

type CertificateManager struct {
	rclient.ResourceManagerClient
	EventRecorder events.EventRecorder
}

// prepareResource prepares a Certificate resource based on the provided Capp.
func (c CertificateManager) prepareResource(capp cappv1alpha1.Capp) (cmapi.Certificate, error) {
	dnsConfig, err := utils.GetDNSConfig(c.Ctx, c.K8sclient)
	if err != nil {
		return cmapi.Certificate{}, err
	}

	resourceName := utils.GenerateResourceName(capp.Spec.RouteSpec.Hostname, dnsConfig.Zone)
	secretName := utils.GenerateSecretName(resourceName)

	certificate := cmapi.Certificate{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: capp.Namespace,
			Labels:    utils.ManagedResourceLabels(capp.Name),
		},
		Spec: cmapi.CertificateSpec{
			CommonName: utils.TruncateCommonName(resourceName),
			DNSNames:   []string{resourceName},
			PrivateKey: &cmapi.CertificatePrivateKey{
				Algorithm: cmapi.RSAKeyAlgorithm,
				Encoding:  cmapi.PKCS1,
				Size:      PrivateKeySize,
			},
			IsCA: false,
			IssuerRef: cmmeta.IssuerReference{
				Name:  dnsConfig.Issuer,
				Kind:  clusterIssuerKind,
				Group: certv1alpha1.GroupVersion.Group,
			},
			SecretName: secretName,
			SecretTemplate: &cmapi.CertificateSecretTemplate{
				Labels: map[string]string{
					// Add knative label to the secret so that kourier can fetch it.
					// See: https://docs.redhat.com/en/documentation/red_hat_openshift_serverless/1.33/html/serving/configuring-custom-domains-for-knative-services#serverless-ossm-secret-filtering-net-kourier_domain-mapping-custom-tls-cert
					certificateUIDSecretLabelKey: "",
				},
			},
		},
	}

	return certificate, nil
}

// CleanUp attempts to delete all Certificates associated with a given Capp resource.
func (c CertificateManager) CleanUp(capp cappv1alpha1.Capp) error {
	certificates, err := c.getPreviousCertificates(capp)
	if err != nil {
		return err
	}

	for _, certificate := range certificates.Items {
		if capp.DeletionTimestamp != nil {
			ok, err := controllerutil.HasOwnerReference(certificate.OwnerReferences, &capp, c.K8sclient.Scheme())
			if err != nil {
				return err
			}
			if ok {
				continue
			}
		}
		cert := rclient.GetBareCertificate(certificate.Name, certificate.Namespace)
		if err := c.DeleteResource(&cert); err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return err
		}
	}

	return nil
}

// IsRequired is responsible to determine if resource Certificate is required.
func (c CertificateManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return capp.Spec.RouteSpec.TlsEnabled && utils.IsCustomHostnameSet(capp.Spec.RouteSpec.Hostname)
}

// Manage creates or updates a Certificate resource based on the provided Capp if it's required.
// If it's not, then it cleans up the resource if it exists.
func (c CertificateManager) Manage(capp cappv1alpha1.Capp) error {
	if c.IsRequired(capp) {
		return c.reconcileCertificate(capp)
	}

	return c.CleanUp(capp)
}

// reconcileCertificate reconciles the cert-manager Certificate for this Capp.
func (c CertificateManager) reconcileCertificate(capp cappv1alpha1.Capp) error {
	certificateFromCapp, err := c.prepareResource(capp)
	if err != nil {
		return fmt.Errorf("failed to prepare Certificate: %w", err)
	}

	certificate := cmapi.Certificate{}

	if err := c.K8sclient.Get(c.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: certificateFromCapp.Name}, &certificate); err != nil {
		if errors.IsNotFound(err) {
			return createManagedResource(c.K8sclient, c.CreateResource, c.EventRecorder, &capp, &certificateFromCapp,
				"Certificate", eventCappCertificateCreated, eventCappCertificateCreationFailed)
		}
		return fmt.Errorf("failed to get Certificate %q: %w", certificateFromCapp.Name, err)
	}

	orig := certificate.DeepCopy()
	certificate.Spec = *certificateFromCapp.Spec.DeepCopy()
	if err := ensureOwnerReference(c.K8sclient, &capp, &certificate, "Certificate"); err != nil {
		return err
	}
	if err := updateManagedResourceIfNeeded(c.UpdateResource, &certificate, orig.Spec, certificate.Spec, orig.OwnerReferences); err != nil {
		return fmt.Errorf("update Certificate %q: %w", certificate.Name, err)
	}

	if capp.Status.RouteStatus.DomainMappingObjectStatus.URL != nil {
		if err := c.handlePreviousCertificates(capp, certificateFromCapp.Name); err != nil {
			return fmt.Errorf("failed to handle previous Certificates: %w", err)
		}
	}

	return nil
}

// handlePreviousCertificates takes care of removing unneeded Certificate objects. If the DNSRecord
// which corresponds to the latest Certificate object is not yet available then return early
// and do not delete the previous Certificates.
func (c CertificateManager) handlePreviousCertificates(capp cappv1alpha1.Capp, name string) error {
	var available bool
	var err error

	available, err = utils.IsDNSRecordAvailable(c.Ctx, c.K8sclient, name, capp.Namespace)
	if err != nil {
		return err
	}

	if !available {
		return nil
	}

	certificates, err := c.getPreviousCertificates(capp)
	if err != nil {
		return err
	}

	return c.deletePreviousCertificates(certificates, capp.Spec.RouteSpec.Hostname)
}

// getPreviousCertificates returns a list of all Certificate objects that are related to the given Capp.
func (c CertificateManager) getPreviousCertificates(capp cappv1alpha1.Capp) (cmapi.CertificateList, error) {
	certificates := cmapi.CertificateList{}

	set := labels.Set{
		utils.CappResourceKey: capp.Name,
	}
	listOptions := utils.GetListOptions(set)

	if err := c.K8sclient.List(c.Ctx, &certificates, &listOptions); err != nil {
		return certificates, fmt.Errorf("unable to list Certificates of Capp %q: %w", capp.Name, err)
	}

	return certificates, nil
}

// deletePreviousCertificates deletes all previous Certificates associated with a Capp.
func (c CertificateManager) deletePreviousCertificates(certificates cmapi.CertificateList, hostname string) error {
	for _, certificate := range certificates.Items {
		if certificate.Name != hostname {
			cert := rclient.GetBareCertificate(certificate.Name, certificate.Namespace)
			if err := c.DeleteResource(&cert); err != nil {
				return err
			}
		}
	}
	return nil
}
