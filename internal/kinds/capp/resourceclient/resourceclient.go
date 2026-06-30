package resourceclient

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ResourceManagerClient struct {
	K8sClient client.Client
	Log       logr.Logger
}

// ObjectIdentityKeyVals returns key/value pairs identifying obj for structured logs.
func ObjectIdentityKeyVals(obj client.Object) []any {
	gvk := obj.GetObjectKind().GroupVersionKind()
	kind := gvk.Kind
	if kind == "" {
		kind = "Unknown"
	}
	return []any{
		"kind", kind,
		"group", gvk.Group,
		"version", gvk.Version,
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
		"resourceVersion", obj.GetResourceVersion(),
	}
}

// CreateResource creates a resource.
func (r ResourceManagerClient) CreateResource(ctx context.Context, resource client.Object) error {
	r.Log.Info("kubernetes API write create", ObjectIdentityKeyVals(resource)...)
	if err := r.K8sClient.Create(ctx, resource); err != nil {
		return fmt.Errorf("failed to create resource %s %s: %w", resource.GetObjectKind().GroupVersionKind().Kind, resource.GetName(), err)
	}
	return nil
}

// UpdateResource updates a resource.
func (r ResourceManagerClient) UpdateResource(ctx context.Context, resource client.Object) error {
	r.Log.Info("kubernetes API write update", ObjectIdentityKeyVals(resource)...)
	if err := r.K8sClient.Update(ctx, resource); err != nil {
		return fmt.Errorf("failed to update %s %s: %w", resource.GetObjectKind().GroupVersionKind().Kind, resource.GetName(), err)
	}
	return nil
}

// DeleteResource deletes a resource.
func (r ResourceManagerClient) DeleteResource(ctx context.Context, resource client.Object) error {
	r.Log.Info("kubernetes API write delete", ObjectIdentityKeyVals(resource)...)
	if err := r.K8sClient.Delete(ctx, resource); err != nil {
		return fmt.Errorf("failed to delete %s %s: %w", resource.GetObjectKind().GroupVersionKind().Kind, resource.GetName(), err)
	}
	return nil
}
