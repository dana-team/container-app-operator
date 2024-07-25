package resourcemanagers

import (
	"context"
	"fmt"
	"reflect"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/autoscale"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	cappDisabledState                     = "disabled"
	cappEnabledState                      = "enabled"
	defaultAutoScaleCM                    = "autoscale-defaults"
	KnativeServing                        = "knativeServing"
	eventCappKnativeServiceCreationFailed = "KnativeServiceCreationFailed"
	eventCappKnativeServiceCreated        = "KnativeServiceCreated"
	eventCappDisabled                     = "CappDisabled"
	eventCappEnabled                      = "CappEnabled"
)

type KnativeServiceManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder record.EventRecorder
}

// prepareResource generates a Knative Service definition from a given Capp resource.
func (k KnativeServiceManager) prepareResource(capp cappv1alpha1.Capp, ctx context.Context) knativev1.Service {
	knativeServiceAnnotations := utils.FilterKeysWithoutPrefix(capp.Annotations, utils.CappAPIGroup)
	knativeServiceLabels := map[string]string{}

	if capp.Labels != nil {
		knativeServiceLabels = capp.Labels
	}
	knativeServiceLabels[utils.CappResourceKey] = capp.Name

	knativeService := knativev1.Service{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      capp.Name,
			Namespace: capp.Namespace,
			Labels: map[string]string{
				utils.CappResourceKey:   capp.Name,
				utils.ManagedByLabelKey: utils.CappKey,
			},
			Annotations: knativeServiceAnnotations,
		},
		Spec: knativev1.ServiceSpec{
			ConfigurationSpec: capp.Spec.ConfigurationSpec,
		},
	}

	// set defaults
	knativeService.Spec.Template.Spec.EnableServiceLinks = new(bool)
	knativeService.Spec.ConfigurationSpec.SetDefaults(ctx)
	knativeService.Spec.RouteSpec.SetDefaults(ctx)
	knativeService.Spec.Template.Spec.SetDefaults(ctx)
	knativeService.Spec.ConfigurationSpec.Template.Spec.TimeoutSeconds = capp.Spec.RouteSpec.RouteTimeoutSeconds

	volumes := k.prepareVolumes(capp)
	knativeService.Spec.Template.Spec.Volumes = append(knativeService.Spec.Template.Spec.Volumes, volumes...)

	defaultCM := corev1.ConfigMap{}
	if err := k.K8sclient.Get(k.Ctx, types.NamespacedName{Namespace: utils.CappNS, Name: defaultAutoScaleCM}, &defaultCM); err != nil {
		k.Log.Error(err, fmt.Sprintf("could not fetch configMap from namespace %q", utils.CappNS))
	}

	knativeService.Spec.Template.ObjectMeta.Annotations = utils.MergeMaps(knativeServiceAnnotations, autoscale.SetAutoScaler(capp, defaultCM.Data))
	knativeService.Spec.Template.ObjectMeta.Labels = knativeServiceLabels

	return knativeService
}

// prepareVolumes generates a list of volumes to be used in a Knative Service definition from a given Capp resource.
func (k KnativeServiceManager) prepareVolumes(capp cappv1alpha1.Capp) []corev1.Volume {
	var volumes []corev1.Volume
	for _, nfsVolume := range capp.Spec.VolumesSpec.NFSVolumes {
		volumes = append(volumes, corev1.Volume{
			Name: nfsVolume.Name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: nfsVolume.Name,
				},
			},
		})
	}
	return volumes
}

// CleanUp attempts to delete the associated KnativeService for a given Capp resource.
func (k KnativeServiceManager) CleanUp(capp cappv1alpha1.Capp) error {
	resourceManager := rclient.ResourceManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}
	ksvc := rclient.GetBareKSVC(capp.Name, capp.Namespace)

	if err := resourceManager.DeleteResource(&ksvc); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

// IsRequired determines if a Knative service (ksvc) is required based on the Capp's spec.
func (k KnativeServiceManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return capp.Spec.State == cappEnabledState
}

// isResumed checks whether the state changed from disabled to enabled.
func (k KnativeServiceManager) isResumed(capp cappv1alpha1.Capp) bool {
	return capp.Status.StateStatus.State == cappDisabledState && k.IsRequired(capp) &&
		!capp.Status.StateStatus.LastChange.IsZero()
}

// Manage creates or updates a KnativeService resource based on the provided Capp if it's required.
// If it's not, then it cleans up the resource if it exists.
func (k KnativeServiceManager) Manage(capp cappv1alpha1.Capp) error {
	if k.IsRequired(capp) {
		return k.createOrUpdate(capp)
	}

	k.Log.Info("Attempting to disable Capp")
	if err := k.CleanUp(capp); err != nil {
		return err
	}

	k.EventRecorder.Event(&capp, corev1.EventTypeNormal, eventCappDisabled, fmt.Sprintf("Capp %s state changed to disabled", capp.Name))
	k.Log.Info("Successfully disabled Capp")

	return nil
}

// createOrUpdate creates or updates a KSVC resource.
func (k KnativeServiceManager) createOrUpdate(capp cappv1alpha1.Capp) error {
	knativeServiceFromCapp := k.prepareResource(capp, k.Ctx)
	knativeService := knativev1.Service{}
	resourceManager := rclient.ResourceManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}

	if err := k.K8sclient.Get(k.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, &knativeService); err != nil {
		if errors.IsNotFound(err) {
			if err := k.createKSVC(&capp, &knativeServiceFromCapp, resourceManager); err != nil {
				return err
			}

			if k.isResumed(capp) {
				k.Log.Info("Capp resumed to enabled state")
				k.EventRecorder.Event(&capp, corev1.EventTypeNormal, eventCappEnabled, fmt.Sprintf("Capp %q state changed to enabled", capp.Name))
			}
			return nil
		} else {
			return fmt.Errorf("failed to get KnativeService %q: %w", knativeService.Name, err)
		}
	}

	return k.updateKSVC(&knativeService, &knativeServiceFromCapp, resourceManager)
}

// createKSVC creates a new knativeService and emits an event.
func (k KnativeServiceManager) createKSVC(capp *cappv1alpha1.Capp, knativeServiceFromCapp *knativev1.Service, resourceManager rclient.ResourceManagerClient) error {
	if err := resourceManager.CreateResource(knativeServiceFromCapp); err != nil {
		k.EventRecorder.Event(capp, corev1.EventTypeWarning, eventCappKnativeServiceCreationFailed,
			fmt.Sprintf("Failed to create KnativeService %s", knativeServiceFromCapp.Name))
		return err
	}

	k.EventRecorder.Event(capp, corev1.EventTypeNormal, eventCappKnativeServiceCreated,
		fmt.Sprintf("Created KnativeService %s", knativeServiceFromCapp.Name))

	return nil
}

// updateKSVC checks if an update to the KnativeService is necessary and performs the update to match desired state.
func (k KnativeServiceManager) updateKSVC(knativeService, knativeServiceFromCapp *knativev1.Service, resourceManager rclient.ResourceManagerClient) error {
	if !reflect.DeepEqual(knativeService.Spec, knativeServiceFromCapp.Spec) {
		knativeService.Spec = knativeServiceFromCapp.Spec
		return resourceManager.UpdateResource(knativeService)
	}

	return nil
}
