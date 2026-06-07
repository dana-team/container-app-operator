package finalizer

import (
	"context"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

const CappCleanupFinalizer = "dana.io/capp-cleanup"

// HandleResourceDeletion manages the Capp deletion.
func HandleResourceDeletion(ctx context.Context, capp cappv1alpha1.Capp, r client.Client, resourceManagers map[string]rmanagers.ResourceManager) (error, bool) {
	if capp.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(&capp, CappCleanupFinalizer) {
			if err := finalizeCapp(ctx, capp, resourceManagers); err != nil {
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
	ctrllog.FromContext(ctx).Info("kubernetes API write update", rclient.ObjectIdentityKeyVals(&capp)...)
	if err := r.Update(ctx, &capp); err != nil {
		return err
	}
	return nil
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
func EnsureFinalizer(ctx context.Context, service cappv1alpha1.Capp, r client.Client) error {
	if !controllerutil.ContainsFinalizer(&service, CappCleanupFinalizer) {
		controllerutil.AddFinalizer(&service, CappCleanupFinalizer)
		ctrllog.FromContext(ctx).Info("kubernetes API write update", rclient.ObjectIdentityKeyVals(&service)...)
		if err := r.Update(ctx, &service); err != nil {
			return err
		}
	}
	return nil
}
