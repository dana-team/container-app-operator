package resourceclient

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	utils "github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
)

type ResourceManagerClient struct {
	K8sClient client.Client
	Log       logr.Logger
}

// CreateResource creates a resource.
func (r ResourceManagerClient) CreateResource(ctx context.Context, resource client.Object) error {
	r.Log.Info("kubernetes API write create", utils.ObjectIdentityKeyVals(resource)...)
	if err := r.K8sClient.Create(ctx, resource); err != nil {
		return fmt.Errorf("failed to create resource %s %s: %w", resource.GetObjectKind().GroupVersionKind().Kind, resource.GetName(), err)
	}
	return nil
}

// UpdateResource updates a resource.
func (r ResourceManagerClient) UpdateResource(ctx context.Context, resource client.Object) error {
	r.Log.Info("kubernetes API write update", utils.ObjectIdentityKeyVals(resource)...)
	if err := r.K8sClient.Update(ctx, resource); err != nil {
		return fmt.Errorf("failed to update %s %s: %w", resource.GetObjectKind().GroupVersionKind().Kind, resource.GetName(), err)
	}
	return nil
}

// DeleteResource deletes a resource.
func (r ResourceManagerClient) DeleteResource(ctx context.Context, resource client.Object) error {
	r.Log.Info("kubernetes API write delete", utils.ObjectIdentityKeyVals(resource)...)
	if err := r.K8sClient.Delete(ctx, resource); err != nil {
		return fmt.Errorf("failed to delete %s %s: %w", resource.GetObjectKind().GroupVersionKind().Kind, resource.GetName(), err)
	}
	return nil
}
