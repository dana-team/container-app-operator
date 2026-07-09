package controllers

import (
	"context"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const cappCleanupFinalizer = "dana.io/capp-cleanup"

func handleResourceDeletion(ctx context.Context, capp cappv1alpha1.Capp, rmClient rclient.ResourceManagerClient, resourceManagers map[string]rmanagers.ResourceManager) (bool, error) {
	if capp.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(&capp, cappCleanupFinalizer) {
			if err := finalizeCapp(ctx, capp, resourceManagers); err != nil {
				return false, err
			}
			return true, removeFinalizer(ctx, capp, rmClient)
		}
	}
	return false, nil
}

func removeFinalizer(ctx context.Context, capp cappv1alpha1.Capp, rmClient rclient.ResourceManagerClient) error {
	controllerutil.RemoveFinalizer(&capp, cappCleanupFinalizer)
	return rmClient.UpdateResource(ctx, &capp)
}

func finalizeCapp(ctx context.Context, capp cappv1alpha1.Capp, resourceManagers map[string]rmanagers.ResourceManager) error {
	for _, manager := range resourceManagers {
		if err := manager.CleanUp(ctx, capp); err != nil {
			return err
		}
	}
	return nil
}

func ensureFinalizer(ctx context.Context, capp cappv1alpha1.Capp, rmClient rclient.ResourceManagerClient) error {
	if !controllerutil.ContainsFinalizer(&capp, cappCleanupFinalizer) {
		controllerutil.AddFinalizer(&capp, cappCleanupFinalizer)
		return rmClient.UpdateResource(ctx, &capp)
	}
	return nil
}
