package resourcemanagers

import (
	"context"
	"fmt"
	"reflect"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1alphav1 "knative.dev/serving/pkg/apis/serving/v1alpha1"
)

const CappResourceKey = "dana.io/parent-capp"

type KnativeDomainMappingManager struct {
	ctx       context.Context
	k8sclient client.Client
	log       logr.Logger
}

// PrepareKnativeDomainMapping creates a new DomainMapping for a Knative service.
// Takes a context.Context object, and a rcsv1alpha1.Capp object as input.
// Returns a knativev1alphav1.DomainMapping object.
func (k KnativeDomainMappingManager) PrepareResource(capp rcsv1alpha1.Capp) knativev1alphav1.DomainMapping {
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
	return *knativeDomainMapping
}

// CreateOrUpdateKnativeDomainMapping creates or updates a DomainMapping object for a Knative service.
// Takes a context.Context object, a rcsv1alpha1.Capp object, a client.Client object, and a logr.Logger object as input.
// Returns an error if there is an issue creating or updating the DomainMapping.
func (k KnativeDomainMappingManager) CreateOrUpdateResource(capp rcsv1alpha1.Capp) error {
	if capp.Spec.RouteSpec.Hostname == "" {
		return nil
	}
	knativeDomainMappingFromCapp := k.PrepareResource(capp)
	knativeDomainMapping := knativev1alphav1.DomainMapping{}
	if err := k.k8sclient.Get(k.ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Spec.RouteSpec.Hostname}, &knativeDomainMapping); err != nil {
		if errors.IsNotFound(err) {
			if err := k.CreateResource(knativeDomainMappingFromCapp); err != nil {
				return err
			}
		} else {
			return err
		}
		return nil
	}
	if err := k.UpdateResource(knativeDomainMappingFromCapp, knativeDomainMapping); err != nil {
		return err
	}
	return nil
}

func (k KnativeDomainMappingManager) CreateResource(domainMapping knativev1alphav1.DomainMapping) error {
	if err := k.k8sclient.Create(k.ctx, &domainMapping); err != nil {
		k.log.Error(err, fmt.Sprintf("unable to create %s %s ", domainMapping.GetObjectKind().GroupVersionKind().Kind, domainMapping.Name))
		return err
	}
	return nil
}

func (k KnativeDomainMappingManager) UpdateResource(domainMapping knativev1alphav1.DomainMapping, oldDomainMapping knativev1alphav1.DomainMapping) error {
	if reflect.DeepEqual(oldDomainMapping.Spec, domainMapping.Spec) {
		return nil
	}
	oldDomainMapping.Spec = domainMapping.Spec
	if err := k.k8sclient.Update(k.ctx, &oldDomainMapping); err != nil {
		if errors.IsConflict(err) {
			k.log.Info(fmt.Sprintf("newer resource version exists for %s %s ", oldDomainMapping.GetObjectKind().GroupVersionKind().Kind, domainMapping.Name))
			return err
		}
		k.log.Error(err, fmt.Sprintf("unable to update %s %s ", oldDomainMapping.GetObjectKind().GroupVersionKind().Kind, oldDomainMapping.Name))
		return err
	}
	k.log.Info(fmt.Sprintf("%s %s updated", oldDomainMapping.GetObjectKind().GroupVersionKind().Kind, oldDomainMapping.Name))
	return nil
}

func (k KnativeDomainMappingManager) DeleteResource(capp rcsv1alpha1.Capp) error {
	knativeDomainMapping := &knativev1alphav1.DomainMapping{}
	if capp.Spec.RouteSpec.Hostname == "" {
		return nil
	}
	if err := k.k8sclient.Get(k.ctx, types.NamespacedName{Name: capp.Spec.RouteSpec.Hostname, Namespace: capp.Namespace}, knativeDomainMapping); err != nil {
		if !errors.IsNotFound(err) {
			k.log.Error(err, "unable to get domainMapping")
			return err
		}
		return nil
	}
	if err := k.k8sclient.Delete(k.ctx, knativeDomainMapping); err != nil {
		k.log.Error(err, "unable to delete Knative domainMapping")
		return err
	}
	return nil
}
