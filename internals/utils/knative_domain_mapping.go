package utils

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

func PrepareKnativeDomainMapping(ctx context.Context, capp rcsv1alpha1.Capp) knativev1alphav1.DomainMapping {
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

func CreateOrUpdateKnativeDomainMapping(ctx context.Context, capp rcsv1alpha1.Capp, r client.Client, log logr.Logger) error {
	knativeDomainMappingFromCapp := PrepareKnativeDomainMapping(ctx, capp)
	knativeDomainMapping := knativev1alphav1.DomainMapping{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Spec.RouteSpec.Hostname}, &knativeDomainMapping); err != nil {
		if errors.IsNotFound(err) {
			if err := CreateKnativeDomainMapping(ctx, knativeDomainMappingFromCapp, r, log); err != nil {
				return err
			}
		} else {
			return err
		}
		return nil
	}
	if err := UpdateKnativeDomainMapping(ctx, knativeDomainMappingFromCapp, knativeDomainMapping, r, log); err != nil {
		return err
	}
	return nil
}

func CreateKnativeDomainMapping(ctx context.Context, domainMapping knativev1alphav1.DomainMapping, r client.Client, log logr.Logger) error {
	if err := r.Create(ctx, &domainMapping); err != nil {
		log.Error(err, fmt.Sprintf("unable to create %s %s ", domainMapping.GetObjectKind().GroupVersionKind().Kind, domainMapping.Name))
		return err
	}
	return nil
}

func UpdateKnativeDomainMapping(ctx context.Context, domainMapping knativev1alphav1.DomainMapping, oldDomainMapping knativev1alphav1.DomainMapping, r client.Client, log logr.Logger) error {
	if reflect.DeepEqual(oldDomainMapping.Spec, domainMapping.Spec) {
		return nil
	}
	oldDomainMapping.Spec = domainMapping.Spec
	if err := r.Update(ctx, &oldDomainMapping); err != nil {
		if errors.IsConflict(err) {
			log.Info(fmt.Sprintf("newer resource version exists for %s %s ", oldDomainMapping.GetObjectKind().GroupVersionKind().Kind, domainMapping.Name))
			return err
		}
		log.Error(err, fmt.Sprintf("unable to update %s %s ", oldDomainMapping.GetObjectKind().GroupVersionKind().Kind, oldDomainMapping.Name))
		return err
	}
	log.Info(fmt.Sprintf("%s %s updated", oldDomainMapping.GetObjectKind().GroupVersionKind().Kind, oldDomainMapping.Name))
	return nil
}
