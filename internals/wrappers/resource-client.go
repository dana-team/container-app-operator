package resourceclient

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const CappResourceKey = "rcs.dana.io/parent-capp"

type ResourceManagerClient interface {
	CreateResource(resource client.Object) error
	UpdateResource(resource client.Object, oldResource client.Object) error
	DeleteResource(resource client.Object, name string, namespace string) error
}

type ResourceBaseManagerClient struct {
	Ctx       context.Context
	K8sclient client.Client
	Log       logr.Logger
}

func (r ResourceBaseManagerClient) CreateResource(resource client.Object) error {

	if err := r.K8sclient.Create(r.Ctx, resource); err != nil {
		r.Log.Error(err, fmt.Sprintf("unable to create %s %s ", resource.GetObjectKind().GroupVersionKind().Kind, resource.GetName()))
		return err
	}
	return nil
}

func (r ResourceBaseManagerClient) UpdateResource(resource client.Object) error {
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

func (r ResourceBaseManagerClient) DeleteResource(resource client.Object, name string, namespace string) error {
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
