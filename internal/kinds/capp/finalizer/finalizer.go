package finalizer

import (
	"context"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const CappCleanupFinalizer = "dana.io/capp-cleanup"

// HandleResourceDeletion manages the Capp deletion.
func HandleResourceDeletion(ctx context.Context, capp cappv1alpha1.Capp, rmClient rclient.ResourceManagerClient, resourceManagers map[string]rmanagers.ResourceManager) (error, bool) {
	if capp.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(&capp, CappCleanupFinalizer) {
			if err := finalizeCapp(ctx, capp, resourceManagers); err != nil {
				return err, false
			}
			return RemoveFinalizer(ctx, capp, rmClient), true
		}
	}
	return nil, false
}

// RemoveFinalizer removes the finalizer from the Capp manifest.
func RemoveFinalizer(ctx context.Context, capp cappv1alpha1.Capp, rmClient rclient.ResourceManagerClient) error {
	controllerutil.RemoveFinalizer(&capp, CappCleanupFinalizer)
	return rmClient.UpdateResource(ctx, &capp)
}

// finalizeCapp runs the cleanup of all the resource managers.
func finalizeCapp(ctx context.Context, capp cappv1alpha1.Capp, resourceManagers map[string]rmanagers.ResourceManager) error {
	for _, manager := range resourceManagers {
		if err := manager.CleanUp(ctx, capp); err != nil {
			return err
		}
	}
	return nil
}

// EnsureFinalizer ensures the service has the finalizer.
func EnsureFinalizer(ctx context.Context, service cappv1alpha1.Capp, rmClient rclient.ResourceManagerClient) error {
	if !controllerutil.ContainsFinalizer(&service, CappCleanupFinalizer) {
		controllerutil.AddFinalizer(&service, CappCleanupFinalizer)
		return rmClient.UpdateResource(ctx, &service)
	}
	return nil
}
