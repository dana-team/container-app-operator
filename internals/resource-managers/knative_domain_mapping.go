package resourceprepares

import (
	"context"
	"reflect"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	secure_utils "github.com/dana-team/container-app-operator/internals/utils/secure"
	rclient "github.com/dana-team/container-app-operator/internals/wrappers"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1alphav1 "knative.dev/serving/pkg/apis/serving/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const CappResourceKey = "dana.io/parent-capp"

type KnativeDomainMappingManager struct {
	Ctx       context.Context
	K8sclient client.Client
	Log       logr.Logger
}

// PrepareKnativeDomainMapping creates a new DomainMapping for a Knative service.
// Takes a context.Context object, and a rcsv1alpha1.Capp object as input.
// Returns a knativev1alphav1.DomainMapping object.
func (k KnativeDomainMappingManager) prepareResource(capp rcsv1alpha1.Capp) knativev1alphav1.DomainMapping {
	knativeDomainMapping := &knativev1alphav1.DomainMapping{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      capp.Spec.RouteSpec.Hostname,
			Namespace: capp.Namespace,
			Annotations: map[string]string{
				CappResourceKey: capp.Name,
			},
		},
		Spec: knativev1alphav1.DomainMappingSpec{
			Ref: duckv1.KReference{
				APIVersion: knativev1.SchemeGroupVersion.String(),
				Name:       capp.Name,
				Kind:       "Service",
			},
		},
	}
	resourceManager := rclient.ResourceBaseManager{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}
	secure_utils.SetHttpsKnativeDomainMapping(capp, knativeDomainMapping, resourceManager)
	return *knativeDomainMapping
}

func (k KnativeDomainMappingManager) CleanUp(capp rcsv1alpha1.Capp) error {
	if capp.Spec.RouteSpec.Hostname == "" {
		return nil
	}
	resourceManager := rclient.ResourceBaseManager{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}
	DomainMapping := knativev1alphav1.DomainMapping{}
	if err := resourceManager.DeleteResource(&DomainMapping, capp.Spec.RouteSpec.Hostname, capp.Namespace); err != nil {
		return err
	}
	return nil
}

func (k KnativeDomainMappingManager) CreateOrUpdateObject(capp rcsv1alpha1.Capp) error {
	if capp.Spec.RouteSpec.Hostname == "" {
		return nil
	}
	knativeDomainMappingFromCapp := k.prepareResource(capp)
	knativeDomainMapping := knativev1alphav1.DomainMapping{}
	resourceManager := rclient.ResourceBaseManager{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}
	if err := k.K8sclient.Get(k.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Spec.RouteSpec.Hostname}, &knativeDomainMapping); err != nil {
		if errors.IsNotFound(err) {
			if err := resourceManager.CreateResource(&knativeDomainMappingFromCapp); err != nil {
				return err
			}
		} else {
			return err
		}
		return nil
	}
	if !reflect.DeepEqual(knativeDomainMapping.Spec, knativeDomainMappingFromCapp.Spec) {
		knativeDomainMapping.Spec = knativeDomainMappingFromCapp.Spec
		if err := resourceManager.UpdateResource(&knativeDomainMapping); err != nil {
			return err
		}
	}
	return nil
}
