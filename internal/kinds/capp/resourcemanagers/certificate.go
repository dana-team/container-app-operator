package resourcemanagers

import (
	"context"
	"fmt"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"k8s.io/client-go/tools/events"
)

const (
	Certificate                        = "Certificate"
	eventCappCertificateCreationFailed = "CertificateCreationFailed"
	eventCappCertificateCreated        = "CertificateCreated"
	eventCappCertificateUpdateFailed   = "CertificateUpdateFailed"
	PrivateKeySize                     = 4096
	maxCommonNameLength                = 64
	certificateUIDSecretLabelKey       = "networking.internal.knative.dev/certificate-uid"
)

type CertificateManager struct {
	rclient.ResourceManagerClient
	EventRecorder events.EventRecorder
	CappConfig    *cappv1alpha1.CappConfig
}

// prepareResource prepares a Certificate resource based on the provided Capp.
func (c CertificateManager) prepareResource(capp cappv1alpha1.Capp) cmapi.Certificate {
	dnsConfig := c.CappConfig.Spec.DNSConfig

	resourceName := GenerateResourceName(capp.Spec.RouteSpec.Hostname, dnsConfig.Zone)
	secretName := generateTLSSecretName(resourceName)

	return cmapi.Certificate{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: capp.Namespace,
			Labels:    cappmeta.ManagedResourceLabels(capp.Name),
		},
		Spec: cmapi.CertificateSpec{
			CommonName: resourceName[:min(len(resourceName), maxCommonNameLength)],
			DNSNames:   []string{resourceName},
			PrivateKey: &cmapi.CertificatePrivateKey{
				Algorithm: cmapi.RSAKeyAlgorithm,
				Encoding:  cmapi.PKCS1,
				Size:      PrivateKeySize,
			},
			IsCA: false,
			IssuerRef: cmmeta.IssuerReference{
				Name:  dnsConfig.IssuerRef.Name,
				Kind:  dnsConfig.IssuerRef.Kind,
				Group: dnsConfig.IssuerRef.Group,
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
}

// CleanUp attempts to delete all Certificates associated with a given Capp resource.
func (c CertificateManager) CleanUp(ctx context.Context, capp cappv1alpha1.Capp) error {
	certificates, err := c.getPreviousCertificates(ctx, capp)
	if err != nil {
		return err
	}
	resources := make([]*cmapi.Certificate, len(certificates.Items))
	for i := range certificates.Items {
		resources[i] = &certificates.Items[i]
	}
	return deleteOwnedResources(ctx, c.K8sClient, &capp, resources)
}

// IsRequired is responsible to determine if resource Certificate is required.
func (c CertificateManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return capp.Spec.RouteSpec.TlsEnabled && capp.Spec.RouteSpec.Hostname != ""
}

// Manage creates or updates a Certificate resource based on the provided Capp if it's required.
// If it's not, then it cleans up the resource if it exists.
func (c CertificateManager) Manage(ctx context.Context, capp cappv1alpha1.Capp) error {
	if c.IsRequired(capp) {
		return c.createOrUpdate(ctx, capp)
	}

	return c.CleanUp(ctx, capp)
}

// createOrUpdate creates or updates a Certificate resource.
func (c CertificateManager) createOrUpdate(ctx context.Context, capp cappv1alpha1.Capp) error {
	certificateFromCapp := c.prepareResource(capp)
	certificate := cmapi.Certificate{}

	if err := c.K8sClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: certificateFromCapp.Name}, &certificate); err != nil {
		if errors.IsNotFound(err) {
			return createManagedResource(ctx, c.K8sClient, c.CreateResource, c.EventRecorder, &capp, &certificateFromCapp,
				Certificate, eventCappCertificateCreated, eventCappCertificateCreationFailed)
		}
		return fmt.Errorf("failed to get Certificate %q: %w", certificateFromCapp.Name, err)
	}

	orig := certificate.DeepCopy()
	certificate.Spec = *certificateFromCapp.Spec.DeepCopy()
	if err := ensureOwnerReference(c.K8sClient, &capp, &certificate, Certificate); err != nil {
		return err
	}
	if err := updateManagedResourceIfNeeded(ctx, c.UpdateResource, &certificate, orig.Spec, certificate.Spec, orig.OwnerReferences,
		c.EventRecorder, &capp, Certificate, eventCappCertificateUpdateFailed); err != nil {
		return fmt.Errorf("update Certificate %q: %w", certificate.Name, err)
	}

	return nil
}

// getPreviousCertificates returns a list of all Certificate objects that are related to the given Capp.
func (c CertificateManager) getPreviousCertificates(ctx context.Context, capp cappv1alpha1.Capp) (cmapi.CertificateList, error) {
	certificates := cmapi.CertificateList{}
	if err := listManagedResources(ctx, c.K8sClient, capp, &certificates, Certificate, nil); err != nil {
		return certificates, err
	}
	return certificates, nil
}
