package finalizer

import (
	"context"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rmanagers "github.com/dana-team/container-app-operator/internals/resource-managers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const FinalizerCleanupCapp = "dana.io/capp-cleanup"

func HandleResourceDeletion(ctx context.Context, capp rcsv1alpha1.Capp, r client.Client, resource_managers []rmanagers.ResourceManager) (error, bool) {
	if capp.ObjectMeta.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(&capp, FinalizerCleanupCapp) {
			if err := finalizeService(capp, resource_managers); err != nil {
				return err, false
			}
			return RemoveFinalizer(ctx, capp, r), true
		}
	}
	return nil, false
}

func RemoveFinalizer(ctx context.Context, capp rcsv1alpha1.Capp, r client.Client) error {
	controllerutil.RemoveFinalizer(&capp, FinalizerCleanupCapp)
	if err := r.Update(ctx, &capp); err != nil {
		return err
	}
	return nil
}

func finalizeService(capp rcsv1alpha1.Capp, resource_managers []rmanagers.ResourceManager) error {
	for _, manager := range resource_managers {
		if err := manager.CleanUp(capp); err != nil {
			return err
		}
	}
	return nil
}

// ensureFinalizer ensures the service has the finalizer
func EnsureFinalizer(ctx context.Context, service rcsv1alpha1.Capp, r client.Client) error {
	if !controllerutil.ContainsFinalizer(&service, FinalizerCleanupCapp) {
		controllerutil.AddFinalizer(&service, FinalizerCleanupCapp)
		if err := r.Update(ctx, &service); err != nil {
			return err
		}
	}
	return nil
}
