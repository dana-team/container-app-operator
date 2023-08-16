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
	danaAnnotationsPrefix = "dana.io"
)

type KnativeServiceManager struct {
	Ctx       context.Context
	K8sclient client.Client
	Log       logr.Logger
	EventRecorder record.EventRecorder
}

func (k KnativeServiceManager) prepareResource(capp rcsv1alpha1.Capp, ctx context.Context) knativev1.Service {
	knativeServiceAnnotations := utils.FilterKeysWithoutPrefix(capp.Annotations, danaAnnotationsPrefix)
	knativeServiceLabels := utils.FilterKeysWithoutPrefix(capp.Labels, danaAnnotationsPrefix)
	knativeServiceLabels[CappResourceKey] = capp.Name

	knativeService := knativev1.Service{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:        capp.Name,
			Namespace:   capp.Namespace,
			Labels:      knativeServiceLabels,
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
	knativeService.Spec.Template.ObjectMeta.Annotations = autoscale_utils.SetAutoScaler(capp)
	return knativeService
}

func (k KnativeServiceManager) CleanUp(capp rcsv1alpha1.Capp) error {
	resourceManager := rclient.ResourceBaseManager{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}
	kservice := knativev1.Service{}
	if err := resourceManager.DeleteResource(&kservice, capp.Name, capp.Namespace); err != nil {
		return fmt.Errorf("unable to delete KnativeService of Capp: %s", err.Error())
	}
	return nil
}

func (k KnativeServiceManager) CreateOrUpdateObject(capp rcsv1alpha1.Capp) error {
	knativeServiceFromCapp := k.prepareResource(capp, k.Ctx)
	knativeService := knativev1.Service{}
	resourceManager := rclient.ResourceBaseManager{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}
	if err := k.K8sclient.Get(k.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, &knativeService); err != nil {
		if errors.IsNotFound(err) {
			if err := resourceManager.CreateResource(&knativeServiceFromCapp); err != nil {
				k.EventRecorder.Event(&capp, eventTypeError, eventCappKnativeServiceCreationFailed, fmt.Sprintf("Failed to create KnativeService %s for Capp %s", knativeService.Name, capp.Name))
				return fmt.Errorf("unable to create KnativeService for Capp: %s", err.Error())
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

	return nil
}
