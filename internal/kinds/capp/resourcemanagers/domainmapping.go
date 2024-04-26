package resourcemanagers

import (
	"context"
	"fmt"
	"reflect"

	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
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
	CappResourceKey                      = "rcs.dana.io/parent-capp"
	eventCappSecretNotFound              = "SecretNotFound"
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
	knativeDomainMapping := &knativev1beta1.DomainMapping{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      capp.Spec.RouteSpec.Hostname,
			Namespace: capp.Namespace,
			Labels: map[string]string{
				CappResourceKey: capp.Name,
			},
			Annotations: map[string]string{
				CappResourceKey: capp.Name,
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
	resourceManager := rclient.ResourceManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}

	if tlsEnabled := capp.Spec.RouteSpec.TlsEnabled; tlsEnabled {
		if err := k.setHTTPSKnativeDomainMapping(capp, knativeDomainMapping, resourceManager); err != nil {
			if !errors.IsNotFound(err) {
				return *knativeDomainMapping, err
			}
		}
	}

	return *knativeDomainMapping, nil
}

// setHTTPSKnativeDomainMapping sets the DomainMapping TLS based on Capp.
func (k KnativeDomainMappingManager) setHTTPSKnativeDomainMapping(capp cappv1alpha1.Capp, knativeDomainMapping *knativev1beta1.DomainMapping, resourceManager rclient.ResourceManagerClient) error {
	tlsSecret := corev1.Secret{}

	if err := resourceManager.K8sclient.Get(resourceManager.Ctx, types.NamespacedName{Name: capp.Spec.RouteSpec.TlsSecret, Namespace: capp.Namespace}, &tlsSecret); err != nil {
		if errors.IsNotFound(err) {
			k.EventRecorder.Event(&capp, corev1.EventTypeWarning, eventCappSecretNotFound,
				fmt.Sprintf("TlsSecret %s for DomainMapping %s does not exist", capp.Spec.RouteSpec.TlsSecret, knativeDomainMapping.Name))
			return fmt.Errorf("tlsSecret %s for DomainMapping does not exist: %w", capp.Spec.RouteSpec.TlsSecret, err)
		}
		return fmt.Errorf("failed to get TlsSecret %s for DomainMapping: %w", capp.Spec.RouteSpec.TlsSecret, err)
	}

	knativeDomainMapping.Spec.TLS = &knativev1beta1.SecretTLS{
		SecretName: capp.Spec.RouteSpec.TlsSecret,
	}

	return nil
}

// CleanUp attempts to delete the associated DomainMapping for a given Capp resource.
func (k KnativeDomainMappingManager) CleanUp(capp cappv1alpha1.Capp) error {
	resourceManager := rclient.ResourceManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}

	if capp.Status.RouteStatus.DomainMappingObjectStatus.URL != nil {
		domainMapping := rclient.PrepareDomainMapping(capp.Status.RouteStatus.DomainMappingObjectStatus.URL.Host, capp.Namespace)
		if err := resourceManager.DeleteResource(&domainMapping); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}
	}
	return nil
}

// IsRequired is responsible to determine if resource knative domain mapping is required.
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
		return fmt.Errorf("failed to prepare DomainMapping resource: %w", err)
	}

	domainMapping := knativev1beta1.DomainMapping{}
	resourceManager := rclient.ResourceManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}

	if err := k.deletePreviousDomainMappings(capp, resourceManager); err != nil {
		return fmt.Errorf("failed to delete previous DomainMappings: %w", err)
	}

	if err := k.K8sclient.Get(k.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: domainMappingFromCapp.Name}, &domainMapping); err != nil {
		if errors.IsNotFound(err) {
			return k.createDomainMapping(capp, domainMappingFromCapp, resourceManager)
		} else {
			return fmt.Errorf("failed to get DomainMapping %q: %w", domainMappingFromCapp.Name, err)
		}
	}

	return k.updateDomainMapping(domainMapping, domainMappingFromCapp, resourceManager)
}

// createKSVC creates a new DomainMapping and emits an event.
func (k KnativeDomainMappingManager) createDomainMapping(capp cappv1alpha1.Capp, domainMappingFromCapp knativev1beta1.DomainMapping, resourceManager rclient.ResourceManagerClient) error {
	if err := resourceManager.CreateResource(&domainMappingFromCapp); err != nil {
		k.EventRecorder.Event(&capp, corev1.EventTypeWarning, eventCappDomainMappingCreationFailed,
			fmt.Sprintf("Failed to create DomainMapping %s", domainMappingFromCapp.Name))

		return err
	}

	k.EventRecorder.Event(&capp, corev1.EventTypeWarning, eventCappDomainMappingCreated,
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

// deletePreviousDomainMappings deletes all previous DomainMappings associated with a Capp.
func (k KnativeDomainMappingManager) deletePreviousDomainMappings(capp cappv1alpha1.Capp, resourceManager rclient.ResourceManagerClient) error {
	requirement, err := labels.NewRequirement(CappResourceKey, selection.Equals, []string{capp.Name})
	if err != nil {
		return fmt.Errorf("unable to create label requirement for Capp: %w", err)
	}

	labelSelector := labels.NewSelector().Add(*requirement)
	listOptions := client.ListOptions{
		LabelSelector: labelSelector,
	}

	knativeDomainMappings := knativev1beta1.DomainMappingList{}
	if err := k.K8sclient.List(k.Ctx, &knativeDomainMappings, &listOptions); err != nil {
		return fmt.Errorf("unable to list DomainMappings of Capp %q: %w", capp.Name, err)
	}

	for _, domainMapping := range knativeDomainMappings.Items {
		if domainMapping.Name != capp.Spec.RouteSpec.Hostname {
			domainMapping := rclient.PrepareDomainMapping(domainMapping.Name, domainMapping.Namespace)
			if err := resourceManager.DeleteResource(&domainMapping); err != nil {
				return err
			}
		}
	}
	return nil
}
