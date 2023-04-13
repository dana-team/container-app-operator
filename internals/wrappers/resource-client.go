package resourceclient

import (
	"context"
	"fmt"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const CappResourceKey = "dana.io/parent-capp"

type ResourceManager interface {
	CreateResource(resource client.Object) error
	UpdateResource(resource client.Object, oldResource client.Object) error
	DeleteResource(capp rcsv1alpha1.Capp) error
}

type ResourceBaseManager struct {
	Ctx       context.Context
	K8sclient client.Client
	Log       logr.Logger
}

func (r ResourceBaseManager) CreateResource(resource client.Object) error {

	if err := r.K8sclient.Create(r.Ctx, resource); err != nil {
		r.Log.Error(err, fmt.Sprintf("unable to create %s %s ", resource.GetObjectKind().GroupVersionKind().Kind, resource.GetName()))
		return err
	}
	return nil
}

func (r ResourceBaseManager) UpdateResource(resource client.Object) error {
	if err := r.K8sclient.Update(r.Ctx, resource); err != nil {
		if errors.IsConflict(err) {
			r.Log.Info(fmt.Sprintf("newer resource version exists for %s %s ", resource.GetObjectKind().GroupVersionKind().Kind, resource.GetName()))
			return err
		}
		r.Log.Error(err, fmt.Sprintf("unable to update %s %s ", resource.GetObjectKind().GroupVersionKind().Kind, resource.GetName()))
		return err
	}
	r.Log.Info(fmt.Sprintf("%s %s updated", resource.GetObjectKind().GroupVersionKind().Kind, resource.GetName()))
	return nil
}

func (r ResourceBaseManager) DeleteResource(resource client.Object, name string, namespace string) error {
	if err := r.K8sclient.Get(r.Ctx, types.NamespacedName{Name: name, Namespace: namespace}, resource); err != nil {
		if !errors.IsNotFound(err) {
			r.Log.Error(err, "unable to get resource")
			return err
		}
		return nil
	}
	if err := r.K8sclient.Delete(r.Ctx, resource); err != nil {
		r.Log.Error(err, "unable to delete resource")
		return err
	}
	return nil
}
