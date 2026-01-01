package controllers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	capputils "github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
// +kubebuilder:rbac:groups=shipwright.io,resources=builds,verbs=get;list;watch;create;update;patch;delete

func (r *CappBuildReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rcs.CappBuild{}).
		Named(cappBuildControllerName).
		Complete(r)
}

func (r *CappBuildReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("CappBuildName", req.Name, "CappBuildNamespace", req.Namespace)
	logger.Info("Starting Reconcile")

	cb := &rcs.CappBuild{}
	if err := r.Get(ctx, req.NamespacedName, cb); err != nil {
		if apierrors.IsNotFound(err) {
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

	var alreadyOwned *controllerutil.AlreadyOwnedError

	cappConfig, err := capputils.GetCappConfig(r.Client)
	if err != nil || cappConfig.Spec.CappBuild == nil {
		_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonMissingPolicy, "CappConfig build policy is missing")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	present := cappConfig.Spec.CappBuild.ClusterBuildStrategy.BuildFile.Present
	absent := cappConfig.Spec.CappBuild.ClusterBuildStrategy.BuildFile.Absent

	selectedStrategyName := absent
	if cb.Spec.BuildFile.Mode == rcs.CappBuildFileModePresent {
		selectedStrategyName = present
	}

	if err := r.reconcileBuild(ctx, cb, selectedStrategyName); err != nil {
		if errors.As(err, &alreadyOwned) {
			_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonBuildConflict, err.Error())
			return ctrl.Result{}, nil
		}
		_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonBuildReconcileFailed, err.Error())
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	buildRef := cb.Namespace + "/" + buildNameFor(cb)
	if cb.Status.BuildRef != buildRef {
		orig := cb.DeepCopy()
		cb.Status.ObservedGeneration = cb.Generation
		cb.Status.BuildRef = buildRef
		if err := r.Status().Patch(ctx, cb, client.MergeFrom(orig)); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}
