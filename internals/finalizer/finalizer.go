package finalizer

import (
	"context"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rmanagers "github.com/dana-team/container-app-operator/internals/resource-managers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const FinalizerCleanupCapp = "dana.io/capp-cleanup"

// HandleResourceDeletion manages the Capp deletion.
func HandleResourceDeletion(ctx context.Context, capp rcsv1alpha1.Capp, r client.Client, resourceManagers map[string]rmanagers.ResourceManager) (error, bool) {
	if capp.ObjectMeta.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(&capp, FinalizerCleanupCapp) {
			if err := finalizeService(capp, resourceManagers); err != nil {
				return err, false
			}
			return RemoveFinalizer(ctx, capp, r), true
		}
	}
	return nil, false
}

// RemoveFinalizer removes the finalizer from the Capp manifest.
func RemoveFinalizer(ctx context.Context, capp rcsv1alpha1.Capp, r client.Client) error {
	controllerutil.RemoveFinalizer(&capp, FinalizerCleanupCapp)
	if err := r.Update(ctx, &capp); err != nil {
		return err
	}
	return nil
}

// fnializeService runs the cleanup of all the resource mangers.
func finalizeService(capp rcsv1alpha1.Capp, resourceManagers map[string]rmanagers.ResourceManager) error {
	for _, manager := range resourceManagers {
		if err := manager.CleanUp(capp); err != nil {
			return err
		}
	}
	return nil
}

// EnsureFinalizer ensures the service has the finalizer.
func EnsureFinalizer(ctx context.Context, service rcsv1alpha1.Capp, r client.Client) error {
	if !controllerutil.ContainsFinalizer(&service, FinalizerCleanupCapp) {
		controllerutil.AddFinalizer(&service, FinalizerCleanupCapp)
		if err := r.Update(ctx, &service); err != nil {
			return err
		}
	}
	return nil
}
