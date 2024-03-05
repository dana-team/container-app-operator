package resourceprepares

import (
	"context"
	"fmt"
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internals/utils"
	autoscaleutils "github.com/dana-team/container-app-operator/internals/utils/autoscale"
	rclient "github.com/dana-team/container-app-operator/internals/wrappers"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	danaAnnotationsPrefix = "rcs.dana.io"
	cappDisabledState     = "disabled"
	cappEnabledState      = "enabled"
	DefaultAutoScaleCM    = "autoscale-defaults"
	CappNS                = "capp-operator-system"
)

type KnativeServiceManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder record.EventRecorder
}

// prepareResource generates a Knative Service definition from a given Capp resource.
func (k KnativeServiceManager) prepareResource(capp rcsv1alpha1.Capp, ctx context.Context) knativev1.Service {
	knativeServiceAnnotations := utils.FilterKeysWithoutPrefix(capp.Annotations, danaAnnotationsPrefix)
	knativeServiceLabels := utils.FilterKeysWithoutPrefix(capp.Labels, danaAnnotationsPrefix)
	knativeServiceLabels[CappResourceKey] = capp.Name

	knativeService := knativev1.Service{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      capp.Name,
			Namespace: capp.Namespace,
			Labels: map[string]string{
				CappResourceKey: capp.Name,
			},
			Annotations: knativeServiceAnnotations,
		},
		Spec: knativev1.ServiceSpec{
			ConfigurationSpec: capp.Spec.ConfigurationSpec,
		},
	}

	// Set defaults
	knativeService.Spec.Template.Spec.EnableServiceLinks = new(bool)
	knativeService.Spec.ConfigurationSpec.SetDefaults(ctx)
	knativeService.Spec.RouteSpec.SetDefaults(ctx)
	knativeService.Spec.Template.Spec.SetDefaults(ctx)

	knativeService.Spec.ConfigurationSpec.Template.Spec.TimeoutSeconds = capp.Spec.RouteSpec.RouteTimeoutSeconds

	defaulCM := corev1.ConfigMap{}
	if err := k.K8sclient.Get(k.Ctx, types.NamespacedName{Namespace: CappNS, Name: DefaultAutoScaleCM}, &defaulCM); err != nil {
		k.Log.Error(err, fmt.Sprintf("could not fetch configMap: %q", CappNS))
	}
	knativeService.Spec.Template.ObjectMeta.Annotations = utils.MergeMaps(knativeServiceAnnotations,
		autoscaleutils.SetAutoScaler(capp, defaulCM.Data))
	knativeService.Spec.Template.ObjectMeta.Labels = knativeServiceLabels
	return knativeService
}

// CleanUp attempts to delete the associated KnativeService for a given Capp resource.
// If the KnativeService is not found, the function completes without error.
// If any other errors occur during the deletion process,
// an error detailing the issue is returned.
func (k KnativeServiceManager) CleanUp(capp rcsv1alpha1.Capp) error {
	resourceManager := rclient.ResourceBaseManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}
	kservice := knativev1.Service{}
	if err := resourceManager.DeleteResource(&kservice, capp.Name, capp.Namespace); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("unable to delete KnativeService of Capp: %s", err.Error())
	}
	return nil
}

// isRequired determines if a Knative service (ksvc) is required based on the Capp's spec.
func (k KnativeServiceManager) isRequired(capp rcsv1alpha1.Capp) bool {
	return capp.Spec.State == cappEnabledState
}

// isResumed checks whether the state changed from disabled to enabled.
func (k KnativeServiceManager) isResumed(capp rcsv1alpha1.Capp) bool {
	return capp.Status.StateStatus.State == cappDisabledState && k.isRequired(capp) &&
		!capp.Status.StateStatus.LastChange.IsZero()
}

// CreateOrUpdateObject ensures a KnativeService resource exists based on the provided Capp.
// If the Capp doesn't require a KnativeService, it triggers a cleanup.
// Otherwise, it either creates a new KnativeService or updates an existing one based on the Capp's specifications.
func (k KnativeServiceManager) CreateOrUpdateObject(capp rcsv1alpha1.Capp) error {
	knativeServiceFromCapp := k.prepareResource(capp, k.Ctx)
	knativeService := knativev1.Service{}
	resourceManager := rclient.ResourceBaseManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}
	if !k.isRequired(capp) {
		k.Log.Info("halting Capp")
		k.EventRecorder.Event(&capp, corev1.EventTypeNormal, eventCappDisabled,
			fmt.Sprintf("Capp %s state changed to disabled", capp.Name))
		return k.CleanUp(capp)
	} else {
		if err := k.K8sclient.Get(k.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name},
			&knativeService); err != nil {
			if errors.IsNotFound(err) {
				if err := resourceManager.CreateResource(&knativeServiceFromCapp); err != nil {
					k.EventRecorder.Event(&capp, corev1.EventTypeWarning, eventCappKnativeServiceCreationFailed,
						fmt.Sprintf("Failed to create KnativeService %s for Capp %s",
							knativeService.Name, capp.Name))
					return fmt.Errorf("unable to create KnativeService for Capp: %s", err.Error())
				}
				if k.isResumed(capp) {
					k.Log.Info("Capp resumed")
					k.EventRecorder.Event(&capp, corev1.EventTypeNormal, eventCappEnabled,
						fmt.Sprintf("Capp %s state changed to enabled", capp.Name))
				}
			} else {
				return err
			}
			return nil
		}
		if !reflect.DeepEqual(knativeService.Spec, knativeServiceFromCapp.Spec) {
			knativeService.Spec = knativeServiceFromCapp.Spec
			if err := resourceManager.UpdateResource(&knativeService); err != nil {
				return fmt.Errorf("unable to update KnativeService of Capp: %s", err.Error())
			}
		}

	}

	return nil
}
