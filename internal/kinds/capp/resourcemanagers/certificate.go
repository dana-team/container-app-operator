package resourcemanagers

import (
	"context"
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
func (c CertificateManager) prepareResource(ctx context.Context, capp cappv1alpha1.Capp) (cmapi.Certificate, error) {
	dnsConfig, err := utils.GetDNSConfig(ctx, c.K8sclient)
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
func (c CertificateManager) CleanUp(ctx context.Context, capp cappv1alpha1.Capp) error {
	certificates, err := c.getPreviousCertificates(ctx, capp)
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
		if err := c.DeleteResource(ctx, &cert); err != nil {
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
func (c CertificateManager) Manage(ctx context.Context, capp cappv1alpha1.Capp) error {
	if c.IsRequired(capp) {
		return c.reconcileCertificate(ctx, capp)
	}

	return c.CleanUp(ctx, capp)
}

// reconcileCertificate reconciles the cert-manager Certificate for this Capp.
func (c CertificateManager) reconcileCertificate(ctx context.Context, capp cappv1alpha1.Capp) error {
	certificateFromCapp, err := c.prepareResource(ctx, capp)
	if err != nil {
		return fmt.Errorf("failed to prepare Certificate: %w", err)
	}

	certificate := cmapi.Certificate{}

	if err := c.K8sclient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: certificateFromCapp.Name}, &certificate); err != nil {
		if errors.IsNotFound(err) {
			return createManagedResource(ctx, c.K8sclient, c.CreateResource, c.EventRecorder, &capp, &certificateFromCapp,
				"Certificate", eventCappCertificateCreated, eventCappCertificateCreationFailed)
		}
		return fmt.Errorf("failed to get Certificate %q: %w", certificateFromCapp.Name, err)
	}

	orig := certificate.DeepCopy()
	certificate.Spec = *certificateFromCapp.Spec.DeepCopy()
	if err := ensureOwnerReference(c.K8sclient, &capp, &certificate, "Certificate"); err != nil {
		return err
	}
	if err := updateManagedResourceIfNeeded(ctx, c.UpdateResource, &certificate, orig.Spec, certificate.Spec, orig.OwnerReferences); err != nil {
		return fmt.Errorf("update Certificate %q: %w", certificate.Name, err)
	}

	return nil
}

// getPreviousCertificates returns a list of all Certificate objects that are related to the given Capp.
func (c CertificateManager) getPreviousCertificates(ctx context.Context, capp cappv1alpha1.Capp) (cmapi.CertificateList, error) {
	certificates := cmapi.CertificateList{}

	set := labels.Set{
		utils.CappResourceKey: capp.Name,
	}
	listOptions := utils.GetListOptions(set)

	if err := c.K8sclient.List(ctx, &certificates, &listOptions); err != nil {
		return certificates, fmt.Errorf("unable to list Certificates of Capp %q: %w", capp.Name, err)
	}

	return certificates, nil
}
