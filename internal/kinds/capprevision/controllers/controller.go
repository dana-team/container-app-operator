package controllers

import (
	"context"
	"fmt"
	"time"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capprevision/actionmanagers"
	"github.com/dana-team/container-app-operator/internal/kinds/capprevision/adapters"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	cappRevisionControllerName = "CappRevisionController"
	RequeueTime                = 5 * time.Second
)

// CappRevisionReconciler reconciles a Capp object
type CappRevisionReconciler struct {
	Log logr.Logger
	client.Client
	Scheme        *runtime.Scheme
	EventRecorder record.EventRecorder
}

// +kubebuilder:rbac:groups=rcs.dana.io,resources=capps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rcs.dana.io,resources=capps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rcs.dana.io,resources=capps/finalizers,verbs=update
// +kubebuilder:rbac:groups=rcs.dana.io,resources=capprevisions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rcs.dana.io,resources=capprevisions/status,verbs=get;update;patch

// SetupWithManager sets up the controller with the Manager.
func (r *CappRevisionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cappv1alpha1.Capp{}).
		Named(cappRevisionControllerName).
		WithEventFilter(NewCappPredicates()).
		Complete(r)
}

func (r *CappRevisionReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("CappName", req.Name, "CappNamespace", req.Namespace)
	logger.Info("Starting Reconcile")
	capp := cappv1alpha1.Capp{}
	if err := r.Get(ctx, req.NamespacedName, &capp); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Capp does not exist")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get Capp: %s", err.Error())
	}
	// There's no need to do anything when a Capp is deleted because we set owner reference in the CappRevison
	if !capp.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}
	if err := syncCappRevision(ctx, r.Client, capp, logger); err != nil {
		if errors.IsConflict(err) || errors.IsAlreadyExists(err) {
			logger.Info(fmt.Sprintf("Conflict detected requeuing: %s", err.Error()))
			return ctrl.Result{RequeueAfter: RequeueTime}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to sync Capp: %s", err.Error())
	}
	return ctrl.Result{}, nil
}

// syncCappRevision manages the lifecycle of CappRevisions based on the state of a Capp, handling creation, update, or deletion.
func syncCappRevision(ctx context.Context, k8sClient client.Client, capp cappv1alpha1.Capp, logger logr.Logger) error {
	cappRevisions, err := adapters.GetCappRevisions(ctx, k8sClient, capp)
	if err != nil {
		logger.Error(err, "could not fetch cappRevisions")
		return err
	}

	if len(cappRevisions) == 0 {
		return actionmanagers.HandleCappCreation(ctx, k8sClient, capp, logger)
	}

	return actionmanagers.HandleCappUpdate(ctx, k8sClient, capp, logger, cappRevisions)
}
