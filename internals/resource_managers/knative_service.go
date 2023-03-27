package resourcemanagers

import (
	"context"
	"fmt"
	"reflect"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internals/utils"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
)

type KnativeServiceManager struct {
	ctx       context.Context
	k8sclient client.Client
	log       logr.Logger
}

func (k KnativeServiceManager) PrepareResource(capp rcsv1alpha1.Capp) knativev1.Service {
	knativeService := knativev1.Service{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      capp.Name,
			Namespace: capp.Namespace,
			Labels: map[string]string{
				CappResourceKey: capp.Name,
			},
			Annotations: map[string]string{
				CappResourceKey: capp.Name,
			},
		},
		Spec: knativev1.ServiceSpec{
			ConfigurationSpec: capp.Spec.ConfigurationSpec,
		},
	}
	knativeService.Spec.Template.ObjectMeta.Annotations = utils.SetAutoScaler(capp, knativeService)
	return knativeService
}

func (k KnativeServiceManager) CreateOrUpdateResource(capp rcsv1alpha1.Capp) error {
	knativeServiceFromCapp := k.PrepareResource(capp)
	knativeService := knativev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, &knativeService); err != nil {
		if errors.IsNotFound(err) {
			if err := k.CreateResource(knativeServiceFromCapp); err != nil {
				return err
			}
		} else {
			return err
		}
		return nil
	}

	if err := k.UpdateResource(knativeServiceFromCapp, knativeService); err != nil {
		return err
	}
	return nil
}

func (k KnativeServiceManager) CreateResource(knativeService knativev1.Service) error {
	if err := k.k8sclient.Create(k.ctx, &knativeService); err != nil {
		if errors.IsAlreadyExists(err) {
			k.log.Error(err, fmt.Sprintf("unable to create %s %s ", knativeService.GetObjectKind().GroupVersionKind().Kind, knativeService.Name))
			return err
		}
	}
	return nil
}

func (k KnativeServiceManager) UpdateResource(knativeService knativev1.Service, oldKnativeService knativev1.Service) error {
	if reflect.DeepEqual(oldKnativeService.Spec, knativeService.Spec) {
		return nil
	}
	oldKnativeService.Spec = knativeService.Spec
	if err := k.k8sclient.Update(k.ctx, &oldKnativeService); err != nil {
		if errors.IsConflict(err) {
			k.log.Info(fmt.Sprintf("newer resource version exists for %s %s ", oldKnativeService.GetObjectKind().GroupVersionKind().Kind, oldKnativeService.Name))
			return nil
		}
		k.log.Error(err, fmt.Sprintf("unable to update %s %s ", knativeService.GetObjectKind().GroupVersionKind().Kind, knativeService.Name))
		return err
	}
	k.log.Info(fmt.Sprintf("%s %s updated", knativeService.GetObjectKind().GroupVersionKind().Kind, knativeService.Name))
	return nil
}

func (k KnativeServiceManager) DeleteResource(capp rcsv1alpha1.Capp) error {
	knativeService := &knativev1.Service{}
	if err := k.k8sclient.Get(k.ctx, types.NamespacedName{Name: capp.Name, Namespace: capp.Namespace}, knativeService); err != nil {
		if !errors.IsNotFound(err) {
			k.log.Error(err, "unable to get KnativeService")
			return err
		}
		return nil
	}
	if err := k.k8sclient.Delete(k.ctx, knativeService); err != nil {
		k.log.Error(err, "unable to delete KnativeService")
		return err
	}
	return nil
}
