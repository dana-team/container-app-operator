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
	"sigs.k8s.io/controller-runtime/pkg/client"

	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
)

func prepareKnativeService(ctx context.Context, capp rcsv1alpha1.Capp) knativev1.Service {
	knativeService := knativev1.Service{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      capp.Name,
			Namespace: capp.Namespace,
			Annotations: map[string]string{
				CappResourceKey: capp.Name,
			},
		},
		Spec: knativev1.ServiceSpec{
			ConfigurationSpec: capp.Spec.ConfigurationSpec,
		},
	}
	knativeService.Spec.Template.ObjectMeta.Annotations = SetAutoScaler(capp, knativeService)
	return knativeService
}

func CreateOrUpdateKnativeService(ctx context.Context, capp rcsv1alpha1.Capp, r client.Client, log logr.Logger) error {
	knativeServiceFromCapp := prepareKnativeService(ctx, capp)
	knativeService := knativev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, &knativeService); err != nil {
		if errors.IsNotFound(err) {
			if err := CreateKnativeService(ctx, knativeServiceFromCapp, r, log); err != nil {
				return err
			}
		} else {
			return err
		}
		return nil
	}

	if err := UpdateKnativeService(ctx, knativeServiceFromCapp, knativeService, r, log); err != nil {
		return err
	}
	return nil
}

func CreateKnativeService(ctx context.Context, knativeService knativev1.Service, r client.Client, log logr.Logger) error {
	if err := r.Create(ctx, &knativeService); err != nil {
		if errors.IsAlreadyExists(err) {
			log.Error(err, fmt.Sprintf("unable to create %s %s ", knativeService.GetObjectKind().GroupVersionKind().Kind, knativeService.Name))
			return err
		}
	}
	return nil
}

func UpdateKnativeService(ctx context.Context, knativeService knativev1.Service, oldKnativeService knativev1.Service, r client.Client, log logr.Logger) error {
	if reflect.DeepEqual(oldKnativeService.Spec, knativeService.Spec) {
		return nil
	}
	oldKnativeService.Spec = knativeService.Spec
	if err := r.Update(ctx, &oldKnativeService); err != nil {
		if errors.IsConflict(err) {
			log.Info(fmt.Sprintf("newer resource version exists for %s %s ", oldKnativeService.GetObjectKind().GroupVersionKind().Kind, oldKnativeService.Name))
			return nil
		}
		log.Error(err, fmt.Sprintf("unable to update %s %s ", knativeService.GetObjectKind().GroupVersionKind().Kind, knativeService.Name))
		return err
	}
	log.Info(fmt.Sprintf("%s %s updated", knativeService.GetObjectKind().GroupVersionKind().Kind, knativeService.Name))
	return nil
}
