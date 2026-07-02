package resourcemanagers

import (
	"context"
	"fmt"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	DomainMapping                        = "domainMapping"
	eventCappDomainMappingCreationFailed = "DomainMappingCreationFailed"
	eventCappDomainMappingCreated        = "DomainMappingCreated"
)

type DomainMappingManager struct {
	rclient.ResourceManagerClient
	EventRecorder events.EventRecorder
}

// prepareResource creates a new DomainMapping for a Knative service.
func (k DomainMappingManager) prepareResource(ctx context.Context, capp cappv1alpha1.Capp) (knativev1beta1.DomainMapping, error) {
	dnsConfig, err := utils.GetDNSConfig(ctx, k.K8sClient)
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
		tlsSecret := corev1.Secret{}
		if err := k.K8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: capp.Namespace}, &tlsSecret); err != nil {
			if !errors.IsNotFound(err) {
				return *knativeDomainMapping, fmt.Errorf("failed to get tlsSecret %s for DomainMapping: %w", secretName, err)
			}
			k.Log.Info("tlsSecret does not yet exist", "secretName", secretName)
		} else {
			knativeDomainMapping.Spec.TLS = &knativev1beta1.SecretTLS{
				SecretName: secretName,
			}
		}
	}

	return *knativeDomainMapping, nil
}

// CleanUp attempts to delete the associated DomainMappings and tls secrets for a given Capp resource.
func (k DomainMappingManager) CleanUp(ctx context.Context, capp cappv1alpha1.Capp) error {
	domainMappings, err := k.getPreviousDomainMappings(ctx, capp)
	if err != nil {
		return err
	}

	for _, item := range domainMappings.Items {
		ownedByCapp := false
		if capp.DeletionTimestamp != nil {
			ok, err := controllerutil.HasOwnerReference(item.OwnerReferences, &capp, k.K8sClient.Scheme())
			if err != nil {
				return err
			}
			ownedByCapp = ok
		}

		if !ownedByCapp {
			dm := knativev1beta1.DomainMapping{ObjectMeta: metav1.ObjectMeta{Name: item.Name, Namespace: item.Namespace}}
			if err := k.DeleteResource(ctx, &dm); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}

		secretName := utils.GenerateSecretName(item.Name)
		secret := corev1.Secret{}
		if err := k.K8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: item.Namespace}, &secret); err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
		} else if err := k.DeleteResource(ctx, &secret); err != nil {
			return err
		}
	}

	return nil
}

// IsRequired is responsible to determine if resource DomainMapping is required.
func (k DomainMappingManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return capp.Spec.RouteSpec.Hostname != ""
}

// Manage creates or updates a DomainMapping resource based on the provided Capp if it's required.
// If it's not, then it cleans up the resource if it exists.
func (k DomainMappingManager) Manage(ctx context.Context, capp cappv1alpha1.Capp) error {
	if k.IsRequired(capp) {
		return k.createOrUpdate(ctx, capp)
	}

	return k.CleanUp(ctx, capp)
}

// createOrUpdate creates or updates a DomainMapping resource.
func (k DomainMappingManager) createOrUpdate(ctx context.Context, capp cappv1alpha1.Capp) error {
	domainMappingFromCapp, err := k.prepareResource(ctx, capp)
	if err != nil {
		return fmt.Errorf("failed to prepare DomainMapping: %w", err)
	}

	domainMapping := knativev1beta1.DomainMapping{}

	if err := k.K8sClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: domainMappingFromCapp.Name}, &domainMapping); err != nil {
		if errors.IsNotFound(err) {
			return createManagedResource(ctx, k.K8sClient, k.CreateResource, k.EventRecorder, &capp, &domainMappingFromCapp,
				"DomainMapping", eventCappDomainMappingCreated, eventCappDomainMappingCreationFailed)
		}
		return fmt.Errorf("failed to get DomainMapping %q: %w", domainMappingFromCapp.Name, err)
	}

	orig := domainMapping.DeepCopy()
	domainMapping.Spec = domainMappingFromCapp.Spec
	if err := ensureOwnerReference(k.K8sClient, &capp, &domainMapping, "DomainMapping"); err != nil {
		return err
	}
	return updateManagedResourceIfNeeded(ctx, k.UpdateResource, &domainMapping, orig.Spec, domainMapping.Spec, orig.OwnerReferences)
}

// getPreviousDomainMappings returns a list of all DomainMapping objects that are related to the given Capp.
func (k DomainMappingManager) getPreviousDomainMappings(ctx context.Context, capp cappv1alpha1.Capp) (knativev1beta1.DomainMappingList, error) {
	knativeDomainMappings := knativev1beta1.DomainMappingList{}
	if err := listManagedResources(ctx, k.K8sClient, capp, &knativeDomainMappings, "DomainMapping", nil); err != nil {
		return knativeDomainMappings, err
	}
	return knativeDomainMappings, nil
}
