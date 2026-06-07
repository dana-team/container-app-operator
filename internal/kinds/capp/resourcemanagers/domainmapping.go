package resourcemanagers

import (
	"fmt"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	DomainMapping                        = "domainMapping"
	eventCappDomainMappingCreationFailed = "DomainMappingCreationFailed"
	eventCappDomainMappingCreated        = "DomainMappingCreated"
)

type KnativeDomainMappingManager struct {
	rclient.ResourceManagerClient
	EventRecorder events.EventRecorder
}

// PrepareKnativeDomainMapping creates a new DomainMapping for a Knative service.
func (k KnativeDomainMappingManager) prepareResource(capp cappv1alpha1.Capp) (knativev1beta1.DomainMapping, error) {
	dnsConfig, err := utils.GetDNSConfig(k.Ctx, k.K8sclient)
	if err != nil {
		return knativev1beta1.DomainMapping{}, err
	}

	resourceName := utils.GenerateResourceName(capp.Spec.RouteSpec.Hostname, dnsConfig.Zone)
	secretName := utils.GenerateSecretName(resourceName)

	knativeDomainMapping := &knativev1beta1.DomainMapping{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: capp.Namespace,
			Labels:    utils.ManagedResourceLabels(capp.Name),
		},
		Spec: knativev1beta1.DomainMappingSpec{
			Ref: duckv1.KReference{
				APIVersion: knativev1.SchemeGroupVersion.String(),
				Name:       capp.Name,
				Kind:       knativeServiceKind,
			},
		},
	}

	if tlsEnabled := capp.Spec.RouteSpec.TlsEnabled; tlsEnabled {
		if err := k.setHTTPSKnativeDomainMapping(secretName, capp.Namespace, knativeDomainMapping); err != nil {
			if !errors.IsNotFound(err) {
				return *knativeDomainMapping, err
			}
		}
	}

	return *knativeDomainMapping, nil
}

// setHTTPSKnativeDomainMapping sets the DomainMapping TLS based on Capp.
func (k KnativeDomainMappingManager) setHTTPSKnativeDomainMapping(secretName, secretNamespace string, knativeDomainMapping *knativev1beta1.DomainMapping) error {
	tlsSecret := corev1.Secret{}

	if err := k.K8sclient.Get(k.Ctx, types.NamespacedName{Name: secretName, Namespace: secretNamespace}, &tlsSecret); err != nil {
		if errors.IsNotFound(err) {
			k.Log.Info("tlsSecret does not yet exist", "secretName", secretName)
			return nil
		}
		return fmt.Errorf("failed to get tlsSecret %s for DomainMapping: %w", secretName, err)
	}

	knativeDomainMapping.Spec.TLS = &knativev1beta1.SecretTLS{
		SecretName: secretName,
	}

	return nil
}

// CleanUp attempts to delete the associated DomainMappings and tls secrets for a given Capp resource.
func (k KnativeDomainMappingManager) CleanUp(capp cappv1alpha1.Capp) error {
	domainMappings, err := k.getPreviousDomainMappings(capp)
	if err != nil {
		return err
	}

	for _, item := range domainMappings.Items {
		var dm knativev1beta1.DomainMapping
		if err := k.K8sclient.Get(k.Ctx, client.ObjectKeyFromObject(&item), &dm); err != nil {
			if err := client.IgnoreNotFound(err); err != nil {
				return err
			}
			continue
		}
		if capp.DeletionTimestamp != nil {
			ok, err := controllerutil.HasOwnerReference(dm.OwnerReferences, &capp, k.K8sclient.Scheme())
			if err != nil {
				return err
			}
			if ok {
				continue
			}
		}
		if err := k.DeleteResource(&dm); err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return err
		}
		if err := k.deleteTLSSecret(utils.GenerateSecretName(dm.Name), dm.Namespace); err != nil {
			return err
		}
	}

	return nil
}

// IsRequired is responsible to determine if resource DomainMapping is required.
func (k KnativeDomainMappingManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return capp.Spec.RouteSpec.Hostname != ""
}

// Manage creates or updates a DomainMapping resource based on the provided Capp if it's required.
// If it's not, then it cleans up the resource if it exists.
func (k KnativeDomainMappingManager) Manage(capp cappv1alpha1.Capp) error {
	if k.IsRequired(capp) {
		return k.createOrUpdate(capp)
	}

	return k.CleanUp(capp)
}

// createOrUpdate creates or updates a DomainMapping resource.
func (k KnativeDomainMappingManager) createOrUpdate(capp cappv1alpha1.Capp) error {
	domainMappingFromCapp, err := k.prepareResource(capp)
	if err != nil {
		return fmt.Errorf("failed to prepare DomainMapping: %w", err)
	}

	domainMapping := knativev1beta1.DomainMapping{}

	if err := k.K8sclient.Get(k.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: domainMappingFromCapp.Name}, &domainMapping); err != nil {
		if errors.IsNotFound(err) {
			return createManagedResource(k.K8sclient, k.CreateResource, k.EventRecorder, &capp, &domainMappingFromCapp,
				"DomainMapping", eventCappDomainMappingCreated, eventCappDomainMappingCreationFailed)
		}
		return fmt.Errorf("failed to get DomainMapping %q: %w", domainMappingFromCapp.Name, err)
	}

	if capp.Status.RouteStatus.DomainMappingObjectStatus.URL != nil {
		if err := k.handlePreviousDomainMappings(capp, domainMappingFromCapp.Name); err != nil {
			return fmt.Errorf("failed to delete previous DomainMappings: %w", err)
		}
	}

	orig := domainMapping.DeepCopy()
	domainMapping.Spec = domainMappingFromCapp.Spec
	if err := ensureOwnerReference(k.K8sclient, &capp, &domainMapping, "DomainMapping"); err != nil {
		return err
	}
	return updateManagedResourceIfNeeded(k.UpdateResource, &domainMapping, orig.Spec, domainMapping.Spec, orig.OwnerReferences)
}

// handlePreviousDomainMappings takes care of removing unneeded DomainMapping objects. If the DNSRecord
// which corresponds to the latest DomainMapping object is not yet available then return early
// and do not delete the previous DomainMappings.
func (k KnativeDomainMappingManager) handlePreviousDomainMappings(capp cappv1alpha1.Capp, name string) error {
	var available bool
	var err error

	available, err = utils.IsDNSRecordAvailable(k.Ctx, k.K8sclient, name, capp.Namespace)
	if err != nil {
		return err
	}

	if !available {
		return nil
	}

	domainMappings, err := k.getPreviousDomainMappings(capp)
	if err != nil {
		return err
	}

	return k.deletePreviousDomainMappings(domainMappings, capp.Spec.RouteSpec.Hostname)
}

// getPreviousDomainMappings returns a list of all DomainMapping objects that are related to the given Capp.
func (k KnativeDomainMappingManager) getPreviousDomainMappings(capp cappv1alpha1.Capp) (knativev1beta1.DomainMappingList, error) {
	knativeDomainMappings := knativev1beta1.DomainMappingList{}

	set := labels.Set{
		utils.CappResourceKey: capp.Name,
	}

	listOptions := utils.GetListOptions(set)
	if err := k.K8sclient.List(k.Ctx, &knativeDomainMappings, &listOptions); err != nil {
		return knativeDomainMappings, fmt.Errorf("unable to list DomainMappings of Capp %q: %w", capp.Name, err)
	}

	return knativeDomainMappings, nil
}

// deletePreviousDomainMappings deletes all previous DomainMappings associated with a Capp.
func (k KnativeDomainMappingManager) deletePreviousDomainMappings(knativeDomainMappings knativev1beta1.DomainMappingList, hostname string) error {
	for _, domainMapping := range knativeDomainMappings.Items {
		if domainMapping.Name != hostname {
			dm := rclient.GetBareDomainMapping(domainMapping.Name, domainMapping.Namespace)
			if err := k.DeleteResource(&dm); err != nil {
				return err
			}
			if err := k.deleteTLSSecret(utils.GenerateSecretName(domainMapping.Name), domainMapping.Namespace); err != nil {
				return err
			}
		}
	}
	return nil
}

// deleteTLSSecret deletes the tls secret associated with the DomainMapping.
func (k KnativeDomainMappingManager) deleteTLSSecret(secretName, namespace string) error {
	secret := corev1.Secret{}
	if err := k.K8sclient.Get(k.Ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, &secret); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return k.DeleteResource(&secret)
}
