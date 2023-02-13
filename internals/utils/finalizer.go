package utils

import (
	"context"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
)

const FinalizerCleanupCapp = "dana.io/capp-cleanup"

func HandleResourceDeletion(ctx context.Context, capp rcsv1alpha1.Capp, log logr.Logger, r client.Client) (error, bool) {
	if capp.ObjectMeta.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(&capp, FinalizerCleanupCapp) {
			if err := finalizeService(ctx, capp, log, r); err != nil {
				return err, false
			}
			controllerutil.RemoveFinalizer(&capp, FinalizerCleanupCapp)
			if err := r.Update(ctx, &capp); err != nil {
				return err, false
			}
			return nil, true
		}
	}
	return nil, false
}

func finalizeService(ctx context.Context, capp rcsv1alpha1.Capp, log logr.Logger, r client.Client) error {
	knativeService := &knativev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Name: capp.Name, Namespace: capp.Namespace}, knativeService); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
	}
	if err := r.Delete(ctx, knativeService); err != nil {
		log.Error(err, "unable to delete KnativeService")
		return err
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
