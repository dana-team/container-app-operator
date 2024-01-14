package resourceprepares

import (
	"context"
	"fmt"
	"reflect"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internals/utils"
	autoscale_utils "github.com/dana-team/container-app-operator/internals/utils/autoscale"
	rclient "github.com/dana-team/container-app-operator/internals/wrappers"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	knativev1 "knative.dev/serving/pkg/apis/serving/v1"

	"k8s.io/client-go/tools/record"
)

const (
	danaAnnotationsPrefix = "rcs.dana.io"
	capHaltState          = "halted"
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
	knativeService.Spec.Template.ObjectMeta.Annotations = utils.MergeMaps(knativeServiceAnnotations,
		autoscale_utils.SetAutoScaler(capp))
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
	return !utils.DoesHaltAnnotationExist(capp.Annotations)
}

// isResumed determines if a given Capp resource has resumed its operation.
func isResumed(capp rcsv1alpha1.Capp) bool {
	return capp.Status.StateStatus.State == capHaltState && !utils.DoesHaltAnnotationExist(capp.Annotations) &&
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
		k.EventRecorder.Event(&capp, eventTypeNormal, eventCappHalted,
			fmt.Sprintf("Capp %s halted", capp.Name))
		return k.CleanUp(capp)
	} else {
		if err := k.K8sclient.Get(k.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name},
			&knativeService); err != nil {
			if errors.IsNotFound(err) {
				if err := resourceManager.CreateResource(&knativeServiceFromCapp); err != nil {
					k.EventRecorder.Event(&capp, eventTypeError, eventCappKnativeServiceCreationFailed,
						fmt.Sprintf("Failed to create KnativeService %s for Capp %s",
							knativeService.Name, capp.Name))
					return fmt.Errorf("unable to create KnativeService for Capp: %s", err.Error())
				}
				if isResumed(capp) {
					k.Log.Info("Capp resumed")
					k.EventRecorder.Event(&capp, eventTypeNormal, eventCappResumed,
						fmt.Sprintf("Capp %s is active", capp.Name))
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
