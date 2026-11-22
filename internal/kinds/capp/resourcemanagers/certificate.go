package resourcemanagers

import (
	"context"
	"fmt"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	Certificate                        = "Certificate"
	eventCappCertificateCreationFailed = "CertificateCreationFailed"
	eventCappCertificateCreated        = "CertificateCreated"
	PrivateKeySize                     = 4096
	maxCommonNameLength                = 64
	certificateUIDSecretLabelKey       = "networking.internal.knative.dev/certificate-uid"
)

type CertificateManager struct {
	rclient.ResourceManagerClient
	EventRecorder events.EventRecorder
	CappConfig    *cappv1alpha1.CappConfig
	// APIReader reads the hostname claim from the API server. When nil, K8sClient is used.
	APIReader client.Reader
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
				// The Capp labels let the operator watch the Secret, so the listener that serves it is set up as
				// soon as cert-manager writes it.
				Labels: cappmeta.MergeMaps(cappmeta.ManagedResourceLabels(capp.Name), map[string]string{
					// Add knative label to the secret so that kourier can fetch it.
					// See: https://docs.redhat.com/en/documentation/red_hat_openshift_serverless/1.33/html/serving/configuring-custom-domains-for-knative-services#serverless-ossm-secret-filtering-net-kourier_domain-mapping-custom-tls-cert
					certificateUIDSecretLabelKey: "",
				}),
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
// With Gateway API routing in the Gateway TLS mode, a listener of the Gateway serves the hostname with its own
// certificate, so no per-Capp Certificate is issued.
func (c CertificateManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return certificateRequired(c.CappConfig, capp)
}

func certificateRequired(cappConfig *cappv1alpha1.CappConfig, capp cappv1alpha1.Capp) bool {
	if !capp.Spec.RouteSpec.TlsEnabled || capp.Spec.RouteSpec.Hostname == "" {
		return false
	}
	return !IsGatewayAPIRouting(cappConfig) || UsesListenerSetTLS(cappConfig, capp)
}

// Manage creates or updates a Certificate resource based on the provided Capp if it's required.
// If it's not, then it cleans up the resource if it exists.
// With Gateway API routing, the hostname claim is checked first: the issuer would otherwise sign a certificate, and
// hand its private key to the Capp's namespace, for a hostname that another namespace holds.
func (c CertificateManager) Manage(ctx context.Context, capp cappv1alpha1.Capp) error {
	if !c.IsRequired(capp) {
		return c.CleanUp(ctx, capp)
	}

	if IsGatewayAPIRouting(c.CappConfig) {
		hostname := cappHostname(c.CappConfig, capp)
		if err := requireHostnameClaim(ctx, readerOrClient(c.APIReader, c.K8sClient), hostname, capp); err != nil {
			if IsHostnameClaimedError(err) {
				if cleanupErr := c.CleanUp(ctx, capp); cleanupErr != nil {
					return cleanupErr
				}
				if cleanupErr := deleteIssuedTLSSecret(ctx, c.ResourceManagerClient, capp, hostname); cleanupErr != nil {
					return cleanupErr
				}
			}
			return err
		}
	}

	return c.createOrUpdate(ctx, capp)
}

// deleteIssuedTLSSecret deletes the TLS Secret that cert-manager issued for the Capp's Certificate of hostname.
// A Secret of that name that cert-manager did not issue for this Certificate, such as one a user created, is left
// alone. When the Capp is being deleted, the Certificate is deleted first: otherwise cert-manager would issue the
// Secret again before garbage collection removes the Certificate, and that Secret would never be deleted.
func deleteIssuedTLSSecret(ctx context.Context, rm rclient.ResourceManagerClient, capp cappv1alpha1.Capp, hostname string) error {
	secret := corev1.Secret{}
	if err := rm.K8sClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: generateTLSSecretName(hostname)}, &secret); err != nil {
		return client.IgnoreNotFound(err)
	}
	if secret.Annotations[cmapi.CertificateNameKey] != hostname {
		return nil
	}

	if capp.DeletionTimestamp != nil {
		certificate := cmapi.Certificate{}
		err := rm.K8sClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: hostname}, &certificate)
		switch {
		case err == nil && isOwnedBy(&certificate, capp):
			if err := client.IgnoreNotFound(rm.K8sClient.Delete(ctx, &certificate)); err != nil {
				return fmt.Errorf("failed to delete Certificate %q: %w", hostname, err)
			}
		case err != nil && !errors.IsNotFound(err):
			return fmt.Errorf("failed to get Certificate %q: %w", hostname, err)
		}
	}

	if err := client.IgnoreNotFound(rm.K8sClient.Delete(ctx, &secret)); err != nil {
		return fmt.Errorf("failed to delete tlsSecret %s: %w", secret.Name, err)
	}
	return nil
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
	if err := updateManagedResourceIfNeeded(ctx, c.UpdateResource, &certificate, orig.Spec, certificate.Spec, orig.OwnerReferences); err != nil {
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
