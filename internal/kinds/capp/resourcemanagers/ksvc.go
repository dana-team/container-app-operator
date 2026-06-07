package resourcemanagers

import (
	"context"
	"fmt"
	"maps"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/autoscale"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	cappDisabledState                     = "disabled"
	cappEnabledState                      = "enabled"
	KnativeServing                        = "knativeServing"
	eventCappKnativeServiceCreationFailed = "KnativeServiceCreationFailed"
	eventCappKnativeServiceCreated        = "KnativeServiceCreated"
	eventCappDisabled                     = "CappDisabled"
	eventCappEnabled                      = "CappEnabled"
	knativeServiceKind                    = "Service"

	kubectlKubernetesIOAnnotationPrefix = "kubectl.kubernetes.io/"
)

type KnativeServiceManager struct {
	rclient.ResourceManagerClient
	EventRecorder events.EventRecorder
}

// prepareResource generates a Knative Service definition from a given Capp resource.
func (k KnativeServiceManager) prepareResource(capp cappv1alpha1.Capp, ctx context.Context) knativev1.Service {
	knativeServiceAnnotations := utils.ExcludeKeysWithPrefix(
		utils.ExcludeKeysWithPrefix(capp.Annotations, utils.CappAPIGroup),
		kubectlKubernetesIOAnnotationPrefix,
	)
	knativeServiceLabels := map[string]string{}

	if capp.Labels != nil {
		maps.Copy(knativeServiceLabels, capp.Labels)

	}
	knativeServiceLabels[utils.CappResourceKey] = capp.Name

	knativeService := knativev1.Service{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:        capp.Name,
			Namespace:   capp.Namespace,
			Labels:      utils.ManagedResourceLabels(capp.Name),
			Annotations: knativeServiceAnnotations,
		},
		Spec: knativev1.ServiceSpec{
			ConfigurationSpec: *capp.Spec.ConfigurationSpec.DeepCopy(),
		},
	}

	// set defaults
	knativeService.Spec.Template.Spec.EnableServiceLinks = new(bool)
	knativeService.Spec.ConfigurationSpec.SetDefaults(ctx)
	knativeService.Spec.RouteSpec.SetDefaults(ctx)
	knativeService.Spec.Template.Spec.SetDefaults(ctx)
	knativeService.Spec.Template.Spec.TimeoutSeconds = capp.Spec.RouteSpec.RouteTimeoutSeconds

	for _, nfsVolume := range capp.Spec.VolumesSpec.NFSVolumes {
		knativeService.Spec.Template.Spec.Volumes = append(knativeService.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: nfsVolume.Name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: nfsVolume.Name,
				},
			},
		})
	}

	cappConfig, err := utils.GetCappConfig(ctx, k.K8sclient)
	if err != nil {
		k.Log.Error(err, fmt.Sprintf("could not fetch cappConfig from namespace %q", utils.CappNS))
	}

	knativeService.Spec.Template.Annotations = utils.MergeMaps(knativeServiceAnnotations, autoscale.SetAutoScaler(capp, cappConfig.Spec.AutoscaleConfig))
	knativeService.Spec.Template.Labels = knativeServiceLabels

	return knativeService
}

// CleanUp ensures the Knative Service is not left behind when it is no longer required for this Capp.
func (k KnativeServiceManager) CleanUp(ctx context.Context, capp cappv1alpha1.Capp) error {
	var ksvc knativev1.Service
	if err := k.K8sclient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, &ksvc); err != nil {
		return client.IgnoreNotFound(err)
	}
	if capp.DeletionTimestamp != nil {
		if ok, err := controllerutil.HasOwnerReference(ksvc.OwnerReferences, &capp, k.K8sclient.Scheme()); err != nil || ok {
			return err
		}
	}
	return client.IgnoreNotFound(k.DeleteResource(ctx, &ksvc))
}

// IsRequired determines if a Knative service (ksvc) is required based on the Capp's spec.
func (k KnativeServiceManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return capp.Spec.State == cappEnabledState
}

// Manage creates or updates a KnativeService resource based on the provided Capp if it's required.
// If it's not, then it cleans up the resource if it exists.
func (k KnativeServiceManager) Manage(ctx context.Context, capp cappv1alpha1.Capp) error {
	if k.IsRequired(capp) {
		return k.createOrUpdate(ctx, capp)
	}

	k.Log.Info("Attempting to disable Capp")
	if err := k.CleanUp(ctx, capp); err != nil {
		return err
	}

	k.EventRecorder.Eventf(&capp, nil, corev1.EventTypeNormal, eventCappDisabled, eventCappDisabled, fmt.Sprintf("Capp %s state changed to disabled", capp.Name))
	k.Log.Info("Successfully disabled Capp")

	return nil
}

// createOrUpdate creates or updates a KSVC resource.
func (k KnativeServiceManager) createOrUpdate(ctx context.Context, capp cappv1alpha1.Capp) error {
	knativeServiceFromCapp := k.prepareResource(capp, ctx)
	knativeService := knativev1.Service{}

	if err := k.K8sclient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, &knativeService); err != nil {
		if errors.IsNotFound(err) {
			if err := createManagedResource(ctx, k.K8sclient, k.CreateResource, k.EventRecorder, &capp, &knativeServiceFromCapp,
				"KnativeService", eventCappKnativeServiceCreated, eventCappKnativeServiceCreationFailed); err != nil {
				return err
			}

			if capp.Status.StateStatus.State == cappDisabledState && k.IsRequired(capp) &&
				!capp.Status.StateStatus.LastChange.IsZero() {
				k.Log.Info("Capp resumed to enabled state")
				k.EventRecorder.Eventf(&capp, nil, corev1.EventTypeNormal, eventCappEnabled, eventCappEnabled, fmt.Sprintf("Capp %q state changed to enabled", capp.Name))
			}
			return nil
		}
		return fmt.Errorf("failed to get KnativeService %q: %w", knativeService.Name, err)
	}

	orig := knativeService.DeepCopy()
	knativeService.Spec = knativeServiceFromCapp.Spec
	if err := ensureOwnerReference(k.K8sclient, &capp, &knativeService, "Knative Service"); err != nil {
		return err
	}
	if managedResourceNeedsUpdate(orig.Spec, knativeService.Spec, orig.OwnerReferences, knativeService.OwnerReferences) {
		k.Log.V(1).Info("KnativeService spec or owner metadata differs from desired; applying update",
			"knativeService", knativeService.Name,
			"namespace", knativeService.Namespace,
			"resourceVersion", knativeService.ResourceVersion,
			"generation", knativeService.Generation)
	}
	return updateManagedResourceIfNeeded(ctx, k.UpdateResource, &knativeService, orig.Spec, knativeService.Spec, orig.OwnerReferences)
}
