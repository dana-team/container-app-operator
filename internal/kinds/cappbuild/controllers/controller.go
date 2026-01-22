package controllers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	capputils "github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
)

const cappBuildControllerName = "CappBuildController"

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
// +kubebuilder:rbac:groups=shipwright.io,resources=clusterbuildstrategies,verbs=get;list;watch
// +kubebuilder:rbac:groups=shipwright.io,resources=buildruns,verbs=get;list;watch;create

func (r *CappBuildReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rcs.CappBuild{}).
		Owns(&shipwright.Build{}).
		Owns(&shipwright.BuildRun{}).
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

	if err := r.ensureOnCommitLabel(ctx, cb); err != nil {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
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
		if errors.Is(err, ErrBuildStrategyNotFound) {
			_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonBuildStrategyNotFound, err.Error())
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		if errors.As(err, &alreadyOwned) {
			_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonBuildConflict, err.Error())
			return ctrl.Result{}, nil
		}
		_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonBuildReconcileFailed, err.Error())
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	buildRef := buildNameFor(cb)
	if cb.Status.BuildRef != buildRef || cb.Status.ObservedGeneration != cb.Generation {
		orig := cb.DeepCopy()
		cb.Status.ObservedGeneration = cb.Generation
		cb.Status.BuildRef = buildRef
		if err := r.Status().Patch(ctx, cb, client.MergeFrom(orig)); err != nil {
			return ctrl.Result{}, err
		}
	}

	var buildRun *shipwright.BuildRun

	if br, requeueAfter, err := r.triggerBuildRun(ctx, cb); err != nil {
		_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonBuildRunReconcileFailed, err.Error())
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	} else if requeueAfter != nil {
		return ctrl.Result{RequeueAfter: *requeueAfter}, nil
	} else if br != nil {
		buildRun = br
	}

	if buildRun == nil {
		br, result, err := r.ensureBuildRun(ctx, cb)
		if err != nil || result != nil {
			return *result, err
		}
		buildRun = br
	}

	if buildRun != nil {
		if err := r.patchBuildSucceededCondition(ctx, cb, buildRun); err != nil {
			return ctrl.Result{}, err
		}
	}

	ready := meta.FindStatusCondition(cb.Status.Conditions, TypeReady)
	if ready == nil ||
		ready.Status != metav1.ConditionTrue ||
		ready.ObservedGeneration != cb.Generation ||
		ready.Reason != ReasonReconciled {
		if err := r.patchReadyCondition(ctx, cb, metav1.ConditionTrue, ReasonReconciled, "CappBuild is reconciled"); err != nil {
			return ctrl.Result{}, err
		}
	}

	if buildRun.IsSuccessful() {
		if err := r.patchLatestImage(ctx, cb, computeLatestImage(cb, buildRun)); err != nil {
			return ctrl.Result{}, err
		}
	}

	cond := meta.FindStatusCondition(cb.Status.Conditions, TypeBuildSucceeded)
	if cond != nil && cond.Status == metav1.ConditionUnknown {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

func (r *CappBuildReconciler) ensureBuildRun(
	ctx context.Context,
	cb *rcs.CappBuild,
) (*shipwright.BuildRun, *ctrl.Result, error) {
	var alreadyOwned *controllerutil.AlreadyOwnedError

	if r.isNewBuildRequired(ctx, cb) {
		br, err := r.reconcileBuildRun(ctx, cb)
		if err != nil {
			if errors.As(err, &alreadyOwned) {
				_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonBuildRunConflict, err.Error())
				return nil, &ctrl.Result{}, nil
			}
			_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonBuildRunReconcileFailed, err.Error())
			return nil, &ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		if err := r.recordBuildSpec(cb); err != nil {
			return nil, &ctrl.Result{}, err
		}
		if err := r.Update(ctx, cb); err != nil {
			return nil, &ctrl.Result{}, err
		}

		return br, nil, nil
	}

	if cb.Status.LastBuildRunRef != "" {
		existingBR := &shipwright.BuildRun{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: cb.Namespace, Name: cb.Status.LastBuildRunRef}, existingBR); err != nil {
			if !apierrors.IsNotFound(err) {
				log.FromContext(ctx).Error(err, "Failed to fetch last BuildRun", "BuildRun", cb.Status.LastBuildRunRef)
			}
		} else {
			return existingBR, nil, nil
		}
	}

	return nil, nil, nil
}
