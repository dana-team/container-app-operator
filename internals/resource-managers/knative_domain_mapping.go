package resourceprepares

import (
	"context"
	"fmt"
	"reflect"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	secureutils "github.com/dana-team/container-app-operator/internals/utils/secure"
	rclient "github.com/dana-team/container-app-operator/internals/wrappers"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/tools/record"
)

const CappResourceKey = "rcs.dana.io/parent-capp"

type KnativeDomainMappingManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder record.EventRecorder
}

// PrepareKnativeDomainMapping creates a new DomainMapping for a Knative service.
// Takes a context.Context object, and a rcsv1alpha1.Capp object as input.
// Returns a knativev1alphav1.DomainMapping object.
func (k KnativeDomainMappingManager) prepareResource(capp rcsv1alpha1.Capp) knativev1beta1.DomainMapping {
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
				Kind:       "Service",
			},
		},
	}
	resourceManager := rclient.ResourceBaseManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}
	secureutils.SetHttpsKnativeDomainMapping(capp, knativeDomainMapping, resourceManager, k.EventRecorder)
	return *knativeDomainMapping
}

func (k KnativeDomainMappingManager) CleanUp(capp rcsv1alpha1.Capp) error {
	if capp.Spec.RouteSpec.Hostname == "" {
		return nil
	}
	resourceManager := rclient.ResourceBaseManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}
	DomainMapping := knativev1beta1.DomainMapping{}
	if err := resourceManager.DeleteResource(&DomainMapping, capp.Spec.RouteSpec.Hostname, capp.Namespace); err != nil {
		return fmt.Errorf("unable to delete DomainMapping %s: %s", capp.Spec.RouteSpec.Hostname, err.Error())
	}
	return nil
}

// responsible to determine if resource knative domain mapping is required.
func (k KnativeDomainMappingManager) isRequired(capp rcsv1alpha1.Capp) bool {
	return capp.Spec.RouteSpec.Hostname != ""
}

func (k KnativeDomainMappingManager) CreateOrUpdateObject(capp rcsv1alpha1.Capp) error {
	if err := k.HandleIrrelevantDomainMapping(capp); err != nil {
		k.Log.Error(err, fmt.Sprintf("failed to handle irrelevant DomainMappings"))
		return err
	}
	if k.isRequired(capp) {
		cappDomainMapping := k.prepareResource(capp)
		knativeDomainMapping := knativev1beta1.DomainMapping{}
		resourceManager := rclient.ResourceBaseManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}
		if err := k.K8sclient.Get(k.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Spec.RouteSpec.Hostname}, &knativeDomainMapping); err != nil {
			if errors.IsNotFound(err) {
				if err := resourceManager.CreateResource(&cappDomainMapping); err != nil {
					k.EventRecorder.Event(&capp, eventTypeError, eventCappDomainMappingCreationFailed, fmt.Sprintf("Failed to create DomainMapping %s for Capp %s", capp.Spec.RouteSpec.Hostname, capp.Name))
					return fmt.Errorf("unable to create DomainMapping: %s", err.Error())
				}
			} else {
				return err
			}
			return nil
		}
		if !reflect.DeepEqual(knativeDomainMapping.Spec, cappDomainMapping.Spec) {
			knativeDomainMapping.Spec = cappDomainMapping.Spec
			if err := resourceManager.UpdateResource(&knativeDomainMapping); err != nil {
				return fmt.Errorf("unable to update DomainMapping: %s", err.Error())
			}
		}
	}

	return nil
}

func (k KnativeDomainMappingManager) HandleIrrelevantDomainMapping(capp rcsv1alpha1.Capp) error {
	logger := k.Log
	requirement, err := labels.NewRequirement(CappResourceKey, selection.Equals, []string{capp.Name})
	if err != nil {
		return fmt.Errorf("unable to create label requirement for Capp %s: %s", capp.Name, err.Error())
	}
	labelSelector := labels.NewSelector().Add(*requirement)
	listOptions := client.ListOptions{
		LabelSelector: labelSelector,
	}
	knativeDomainMappings := knativev1beta1.DomainMappingList{}
	if err := k.K8sclient.List(k.Ctx, &knativeDomainMappings, &listOptions); err != nil {
		return fmt.Errorf("unable to list DomainMappings of Capp %s: %s", capp.Name, err.Error())
	}
	resourceManager := rclient.ResourceBaseManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}
	for _, domainMapping := range knativeDomainMappings.Items {
		if domainMapping.Name != capp.Spec.RouteSpec.Hostname {
			DomainMapping := knativev1beta1.DomainMapping{}
			if err := resourceManager.DeleteResource(&DomainMapping, domainMapping.Name, capp.Namespace); err != nil {
				logger.Error(err, fmt.Sprintf("unable to delete irrelevant DomainMapping %s of Capp %s", domainMapping.Name, capp.Name))
				return err
			}
		}
	}
	return nil
}
