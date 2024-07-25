package resourcemanagers

import (
	"context"
	"fmt"

	certv1alpha1 "github.com/dana-team/certificate-operator/api/v1alpha1"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	Certificate                        = "certificate"
	eventCappCertificateCreationFailed = "CertificateCreationFailed"
	eventCappCertificateCreated        = "CertificateCreated"
	certificateForm                    = "pfx"
	certificateConfig                  = "certificateconfig-capp"
)

type CertificateManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder record.EventRecorder
}

// prepareResource prepares a Certificate resource based on the provided Capp.
func (c CertificateManager) prepareResource(capp cappv1alpha1.Capp) (certv1alpha1.Certificate, error) {
	dnsConfig, err := utils.GetDNSConfig(c.Ctx, c.K8sclient)
	if err != nil {
		return certv1alpha1.Certificate{}, err
	}

	zone, err := utils.GetZoneFromConfig(dnsConfig)
	if err != nil {
		return certv1alpha1.Certificate{}, err
	}

	resourceName := utils.GenerateResourceName(capp.Spec.RouteSpec.Hostname, zone)
	secretName := utils.GenerateSecretName(capp)

	certificate := certv1alpha1.Certificate{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: capp.Namespace,
			Labels: map[string]string{
				CappResourceKey: capp.Name,
			},
		},
		Spec: certv1alpha1.CertificateSpec{
			CertificateData: certv1alpha1.CertificateData{
				Subject: certv1alpha1.Subject{
					CommonName: resourceName,
				},
				San: certv1alpha1.San{
					DNS: []string{resourceName},
				},
				Form: certificateForm,
			},
			SecretName: secretName,
			ConfigRef: certv1alpha1.ConfigReference{
				Name: certificateConfig,
			},
		},
	}

	return certificate, nil
}

// CleanUp attempts to delete the associated Certificate for a given Capp resource.
func (c CertificateManager) CleanUp(capp cappv1alpha1.Capp) error {
	resourceManager := rclient.ResourceManagerClient{Ctx: c.Ctx, K8sclient: c.K8sclient, Log: c.Log}

	if capp.Status.RouteStatus.DomainMappingObjectStatus.URL != nil {
		certificate := rclient.GetBareCertificate(capp.Status.RouteStatus.DomainMappingObjectStatus.URL.Host, capp.Namespace)
		if err := resourceManager.DeleteResource(&certificate); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}
	}
	return nil
}

// IsRequired is responsible to determine if resource Certificate is required.
func (c CertificateManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return capp.Spec.RouteSpec.Hostname != "" && capp.Spec.RouteSpec.TlsEnabled
}

// Manage creates or updates a Certificate resource based on the provided Capp if it's required.
// If it's not, then it cleans up the resource if it exists.
func (c CertificateManager) Manage(capp cappv1alpha1.Capp) error {
	if c.IsRequired(capp) {
		return c.create(capp)
	}

	return c.CleanUp(capp)
}

// create creates a Certificate resource.
func (c CertificateManager) create(capp cappv1alpha1.Capp) error {
	certificateFromCapp, err := c.prepareResource(capp)
	if err != nil {
		return fmt.Errorf("failed to prepare Certificate: %w", err)
	}

	certificate := certv1alpha1.Certificate{}
	resourceManager := rclient.ResourceManagerClient{Ctx: c.Ctx, K8sclient: c.K8sclient, Log: c.Log}

	if err := c.K8sclient.Get(c.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: certificateFromCapp.Name}, &certificate); err != nil {
		if errors.IsNotFound(err) {
			if err := c.createCertificate(capp, certificateFromCapp, resourceManager); err != nil {
				return err
			}
			if capp.Status.RouteStatus.DomainMappingObjectStatus.URL != nil {
				if err := c.handlePreviousCertificates(capp, resourceManager, certificateFromCapp.Name); err != nil {
					return fmt.Errorf("failed to handle previous Certificates: %w", err)
				}
			}
			return c.createCertificate(capp, certificateFromCapp, resourceManager)
		} else {
			return fmt.Errorf("failed to get Certificate %q: %w", certificateFromCapp.Name, err)
		}
	}

	return nil
}

// createCertificate creates a new Certificate and emits an event.
func (c CertificateManager) createCertificate(capp cappv1alpha1.Capp, certificateFromCapp certv1alpha1.Certificate, resourceManager rclient.ResourceManagerClient) error {
	if err := resourceManager.CreateResource(&certificateFromCapp); err != nil {
		c.EventRecorder.Event(&capp, corev1.EventTypeWarning, eventCappCertificateCreationFailed,
			fmt.Sprintf("Failed to create Certificate %s", certificateFromCapp.Name))

		return err
	}

	c.EventRecorder.Event(&capp, corev1.EventTypeNormal, eventCappCertificateCreated,
		fmt.Sprintf("Created Certificate %s", certificateFromCapp.Name))

	return nil
}

// handlePreviousCertificates takes care of removing unneeded Certificate objects. If the DNSRecord
// which corresponds to the latest Certificate object is not yet available then return early
// and do not delete the previous Certificates.
func (c CertificateManager) handlePreviousCertificates(capp cappv1alpha1.Capp, resourceManager rclient.ResourceManagerClient, name string) error {
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

	return c.deletePreviousCertificates(certificates, resourceManager, capp.Spec.RouteSpec.Hostname)
}

// getPreviousCertificates returns a list of all Certificate objects that are related to the given Capp.
func (c CertificateManager) getPreviousCertificates(capp cappv1alpha1.Capp) (certv1alpha1.CertificateList, error) {
	certificates := certv1alpha1.CertificateList{}

	set := labels.Set{
		CappResourceKey: capp.Name,
	}
	listOptions := utils.GetListOptions(set)

	if err := c.K8sclient.List(c.Ctx, &certificates, &listOptions); err != nil {
		return certificates, fmt.Errorf("unable to list Certificates of Capp %q: %w", capp.Name, err)
	}

	return certificates, nil
}

// deletePreviousCertificates deletes all previous Certificates associated with a Capp.
func (c CertificateManager) deletePreviousCertificates(certificates certv1alpha1.CertificateList, resourceManager rclient.ResourceManagerClient, hostname string) error {
	for _, certificate := range certificates.Items {
		if certificate.Name != hostname {
			cert := rclient.GetBareCertificate(certificate.Name, certificate.Namespace)
			if err := resourceManager.DeleteResource(&cert); err != nil {
				return err
			}
		}
	}
	return nil
}
