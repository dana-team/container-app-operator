package controllers

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
)

const cappBuildControllerName = "CappBuildController"

// CappBuildReconciler reconciles a CappBuild object.
type CappBuildReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	EventRecorder record.EventRecorder
}

// +kubebuilder:rbac:groups=rcs.dana.io,resources=cappbuilds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rcs.dana.io,resources=cappbuilds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rcs.dana.io,resources=cappbuilds/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;patch;update
// +kubebuilder:rbac:groups="events.k8s.io",resources=events,verbs=get;list;watch;create;patch;update

func (r *CappBuildReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cappv1alpha1.CappBuild{}).
		Named(cappBuildControllerName).
		Complete(r)
}

func (r *CappBuildReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("CappBuildName", req.Name, "CappBuildNamespace", req.Namespace)
	logger.Info("Starting Reconcile")

	cb := &cappv1alpha1.CappBuild{}
	if err := r.Get(ctx, req.NamespacedName, cb); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get CappBuild: %w", err)
	}

	if cb.Status.ObservedGeneration != cb.Generation {
		orig := cb.DeepCopy()
		cb.Status.ObservedGeneration = cb.Generation
		if err := r.Status().Patch(ctx, cb, client.MergeFrom(orig)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch CappBuild status: %w", err)
		}
	}

	return ctrl.Result{}, nil
}
