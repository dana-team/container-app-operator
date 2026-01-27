package resourcemanagers

import (
	"context"
	"fmt"
	"reflect"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DomainMapping                        = "domainMapping"
	eventCappDomainMappingCreationFailed = "DomainMappingCreationFailed"
	eventCappDomainMappingCreated        = "DomainMappingCreated"
	referenceKind                        = "Service"
)

type KnativeDomainMappingManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder record.EventRecorder
}

// PrepareKnativeDomainMapping creates a new DomainMapping for a Knative service.
func (k KnativeDomainMappingManager) prepareResource(capp cappv1alpha1.Capp) (knativev1beta1.DomainMapping, error) {
	dnsConfig, err := utils.GetDNSConfig(k.Ctx, k.K8sclient)
	if err != nil {
		return knativev1beta1.DomainMapping{}, err
	}

	zone, err := utils.GetZoneFromConfig(dnsConfig)
	if err != nil {
		return knativev1beta1.DomainMapping{}, err
	}

	resourceName := utils.GenerateResourceName(capp.Spec.RouteSpec.Hostname, zone)
	secretName := utils.GenerateSecretName(resourceName)

	knativeDomainMapping := &knativev1beta1.DomainMapping{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: capp.Namespace,
			Labels: map[string]string{
				utils.CappResourceKey:   capp.Name,
				utils.ManagedByLabelKey: utils.CappKey,
			},
		},
		Spec: knativev1beta1.DomainMappingSpec{
			Ref: duckv1.KReference{
				APIVersion: knativev1.SchemeGroupVersion.String(),
				Name:       capp.Name,
				Kind:       referenceKind,
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
	resourceManager := rclient.ResourceManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}

	if err := resourceManager.K8sclient.Get(resourceManager.Ctx, types.NamespacedName{Name: secretName, Namespace: secretNamespace}, &tlsSecret); err != nil {
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
	resourceManager := rclient.ResourceManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}

	domainMappings, err := k.getPreviousDomainMappings(capp)
	if err != nil {
		return err
	}

	for _, domainMapping := range domainMappings.Items {
		dm := rclient.GetBareDomainMapping(domainMapping.Name, domainMapping.Namespace)

		if err := resourceManager.DeleteResource(&dm); err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return err
		}

		if err := deleteTLSSecret(resourceManager.Ctx, resourceManager.K8sclient, utils.GenerateSecretName(domainMapping.Name), domainMapping.Namespace); err != nil {
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
	resourceManager := rclient.ResourceManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}

	if err := k.K8sclient.Get(k.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: domainMappingFromCapp.Name}, &domainMapping); err != nil {
		if errors.IsNotFound(err) {
			return k.createDomainMapping(capp, domainMappingFromCapp, resourceManager)
		} else {
			return fmt.Errorf("failed to get DomainMapping %q: %w", domainMappingFromCapp.Name, err)
		}
	}

	if capp.Status.RouteStatus.DomainMappingObjectStatus.URL != nil {
		if err := k.handlePreviousDomainMappings(capp, resourceManager, domainMappingFromCapp.Name); err != nil {
			return fmt.Errorf("failed to delete previous DomainMappings: %w", err)
		}
	}

	return k.updateDomainMapping(domainMapping, domainMappingFromCapp, resourceManager)
}

// createDomainMapping creates a new DomainMapping and emits an event.
func (k KnativeDomainMappingManager) createDomainMapping(capp cappv1alpha1.Capp, domainMappingFromCapp knativev1beta1.DomainMapping, resourceManager rclient.ResourceManagerClient) error {
	if err := resourceManager.CreateResource(&domainMappingFromCapp); err != nil {
		k.EventRecorder.Event(&capp, corev1.EventTypeWarning, eventCappDomainMappingCreationFailed,
			fmt.Sprintf("Failed to create DomainMapping %s", domainMappingFromCapp.Name))

		return err
	}

	k.EventRecorder.Event(&capp, corev1.EventTypeNormal, eventCappDomainMappingCreated,
		fmt.Sprintf("Created DomainMapping %s", domainMappingFromCapp.Name))

	return nil
}

// updateDomainMapping checks if an update to the DomainMapping is necessary and performs the update to match desired state.
func (k KnativeDomainMappingManager) updateDomainMapping(knativeDomainMapping, domainMappingFromCapp knativev1beta1.DomainMapping, resourceManager rclient.ResourceManagerClient) error {
	if !reflect.DeepEqual(knativeDomainMapping.Spec, domainMappingFromCapp.Spec) {
		knativeDomainMapping.Spec = domainMappingFromCapp.Spec
		return resourceManager.UpdateResource(&knativeDomainMapping)
	}

	return nil
}

// handlePreviousDomainMappings takes care of removing unneeded DomainMapping objects. If the DNSRecord
// which corresponds to the latest DomainMapping object is not yet available then return early
// and do not delete the previous DomainMappings.
func (k KnativeDomainMappingManager) handlePreviousDomainMappings(capp cappv1alpha1.Capp, resourceManager rclient.ResourceManagerClient, name string) error {
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

	return k.deletePreviousDomainMappings(domainMappings, resourceManager, capp.Spec.RouteSpec.Hostname)
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
func (k KnativeDomainMappingManager) deletePreviousDomainMappings(knativeDomainMappings knativev1beta1.DomainMappingList, resourceManager rclient.ResourceManagerClient, hostname string) error {
	for _, domainMapping := range knativeDomainMappings.Items {
		if domainMapping.Name != hostname {
			dm := rclient.GetBareDomainMapping(domainMapping.Name, domainMapping.Namespace)
			if err := resourceManager.DeleteResource(&dm); err != nil {
				return err
			}
		}
		if err := deleteTLSSecret(resourceManager.Ctx, resourceManager.K8sclient, utils.GenerateSecretName(domainMapping.Name), domainMapping.Namespace); err != nil {
			return err
		}
	}
	return nil
}

// deleteTLSSecret deletes the tls secret associated with the DomainMapping.
func deleteTLSSecret(ctx context.Context, client client.Client, secretName string, namespace string) error {
	secret := corev1.Secret{}
	if err := client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, &secret); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if err := client.Delete(ctx, &secret); err != nil {
		return err
	}

	return nil
}
