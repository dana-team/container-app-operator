package controllers

import (
	"context"
	"time"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	onCommitDebounce    = 10 * time.Second
	onCommitMinInterval = 30 * time.Second
	onCommitLabelKey    = "rcs.dana.io/oncommit-enabled"
)

// ensureOnCommitLabel maintains the label required for the webhook handler to filter CappBuilds.
func (r *CappBuildReconciler) ensureOnCommitLabel(ctx context.Context, cb *rcs.CappBuild) error {
	desired := "false"
	if cb.Spec.Rebuild != nil && cb.Spec.Rebuild.Mode == rcs.CappBuildRebuildModeOnCommit {
		desired = "true"
	}

	if cb.Labels == nil {
		cb.Labels = map[string]string{}
	}
	if cb.Labels[onCommitLabelKey] == desired {
		return nil
	}

	orig := cb.DeepCopy()
	cb.Labels[onCommitLabelKey] = desired
	return r.Patch(ctx, cb, client.MergeFrom(orig))
}

// triggerBuildRun enforces debounce/rate-limit/one-active-build and creates a BuildRun
// when a pending trigger is ready.
//
// Returns:
// - selected BuildRun to use for status mapping (may be an existing active run)
// - optional requeueAfter for debounce/rate-limit timers
func (r *CappBuildReconciler) triggerBuildRun(
	ctx context.Context,
	cb *rcs.CappBuild,
) (*shipwright.BuildRun, *time.Duration, error) {
	if cb.Spec.Rebuild == nil || cb.Spec.Rebuild.Mode != rcs.CappBuildRebuildModeOnCommit {
		return nil, nil, nil
	}

	if cb.Status.OnCommit == nil || cb.Status.OnCommit.Pending == nil {
		return nil, nil, nil
	}

	if requeueAfter := requeueAfter(cb); requeueAfter != nil {
		return nil, requeueAfter, nil
	}

	if cb.Status.LastBuildRunRef != "" {
		active := &shipwright.BuildRun{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: cb.Namespace, Name: cb.Status.LastBuildRunRef}, active); err == nil {
			if metav1.IsControlledBy(active, cb) {
				cond := active.Status.GetCondition(shipwright.Succeeded)
				// If the build is still running (not True and not False), return it as active.
				if cond == nil || (cond.GetStatus() != corev1.ConditionTrue && cond.GetStatus() != corev1.ConditionFalse) {
					return active, nil, nil
				}
			}
		} else if client.IgnoreNotFound(err) != nil {
			return nil, nil, err
		}
	}

	counter := nextTrigger(cb)
	br, err := r.ensureBuildRunOnCommit(ctx, cb, counter)
	if err != nil {
		return nil, nil, err
	}

	if err := r.markTriggered(ctx, cb, br, counter); err != nil {
		return nil, nil, err
	}

	return br, nil, nil
}

func requeueAfter(cb *rcs.CappBuild) *time.Duration {
	receivedAt := cb.Status.OnCommit.Pending.ReceivedAt.Time
	if !receivedAt.IsZero() {
		if remaining := time.Until(receivedAt.Add(onCommitDebounce)); remaining > 0 {
			return &remaining
		}
	}

	if cb.Status.OnCommit.LastTriggeredBuildRun != nil && !cb.Status.OnCommit.LastTriggeredBuildRun.TriggeredAt.IsZero() {
		last := cb.Status.OnCommit.LastTriggeredBuildRun.TriggeredAt.Time
		if remaining := time.Until(last.Add(onCommitMinInterval)); remaining > 0 {
			return &remaining
		}
	}

	return nil
}

func nextTrigger(cb *rcs.CappBuild) int64 {
	counter := cb.Status.OnCommit.TriggerCounter
	if counter < 0 {
		counter = 0
	}
	return counter + 1
}

func (r *CappBuildReconciler) markTriggered(ctx context.Context, cb *rcs.CappBuild, br *shipwright.BuildRun, triggerCounter int64) error {
	orig := cb.DeepCopy()
	cb.Status.OnCommit.TriggerCounter = triggerCounter
	cb.Status.OnCommit.LastTriggeredBuildRun = &rcs.CappBuildOnCommitLastTriggered{
		Name:        br.Name,
		TriggeredAt: metav1.Now(),
	}
	cb.Status.OnCommit.Pending = nil
	return r.Status().Patch(ctx, cb, client.MergeFrom(orig))
}
