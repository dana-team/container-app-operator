package resourceclient

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ResourceManagerClient struct {
	Ctx       context.Context
	K8sclient client.Client
	Log       logr.Logger
}

// CreateResource creates a resource.
func (r ResourceManagerClient) CreateResource(resource client.Object) error {
	if err := r.K8sclient.Create(r.Ctx, resource); err != nil {
		return fmt.Errorf("failed to create resource %s %s: %w", resource.GetObjectKind().GroupVersionKind().Kind, resource.GetName(), err)
	}

	r.Log.Info(fmt.Sprintf("successfully created %s %s", resource.GetObjectKind().GroupVersionKind().Kind, resource.GetName()))
	return nil
}

// UpdateResource updates a resource.
func (r ResourceManagerClient) UpdateResource(resource client.Object) error {
	if err := r.K8sclient.Update(r.Ctx, resource); err != nil {
		return fmt.Errorf("failed to update %s %s: %w", resource.GetObjectKind().GroupVersionKind().Kind, resource.GetName(), err)
	}

	r.Log.Info(fmt.Sprintf("successfully updated %s %s", resource.GetObjectKind().GroupVersionKind().Kind, resource.GetName()))
	return nil
}

// DeleteResource deletes a resource.
func (r ResourceManagerClient) DeleteResource(resource client.Object) error {
	if err := r.K8sclient.Delete(r.Ctx, resource); err != nil {
		return fmt.Errorf("failed to delete %s %s: %w", resource.GetObjectKind().GroupVersionKind().Kind, resource.GetName(), err)
	}

	r.Log.Info(fmt.Sprintf("successfully deleted %s %s", resource.GetObjectKind().GroupVersionKind().Kind, resource.GetName()))
	return nil
}
