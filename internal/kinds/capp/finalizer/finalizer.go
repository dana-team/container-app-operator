package finalizer

import (
	"context"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const CappCleanupFinalizer = "dana.io/capp-cleanup"

// HandleResourceDeletion manages the Capp deletion.
func HandleResourceDeletion(ctx context.Context, capp cappv1alpha1.Capp, r client.Client, resourceManagers map[string]rmanagers.ResourceManager) (error, bool) {
	if capp.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(&capp, CappCleanupFinalizer) {
			if err := finalizeCapp(capp, resourceManagers); err != nil {
				return err, false
			}
			return RemoveFinalizer(ctx, capp, r), true
		}
	}
	return nil, false
}

// RemoveFinalizer removes the finalizer from the Capp manifest.
func RemoveFinalizer(ctx context.Context, capp cappv1alpha1.Capp, r client.Client) error {
	controllerutil.RemoveFinalizer(&capp, CappCleanupFinalizer)
	if err := r.Update(ctx, &capp); err != nil {
		return err
	}
	return nil
}

// finalizeCapp runs the cleanup of all the resource managers.
func finalizeCapp(capp cappv1alpha1.Capp, resourceManagers map[string]rmanagers.ResourceManager) error {
	for _, manager := range resourceManagers {
		if err := manager.CleanUp(capp); err != nil {
			return err
		}
	}
	return nil
}

// EnsureFinalizer ensures the service has the finalizer.
func EnsureFinalizer(ctx context.Context, service cappv1alpha1.Capp, r client.Client) error {
	if !controllerutil.ContainsFinalizer(&service, CappCleanupFinalizer) {
		controllerutil.AddFinalizer(&service, CappCleanupFinalizer)
		if err := r.Update(ctx, &service); err != nil {
			return err
		}
	}
	return nil
}
