package controllers

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strings"
	"time"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/rollout"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	knativeapis "knative.dev/pkg/apis"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const actionRollout = "Rollout"

// errRolloutAnnotationChanged reports that the rollout annotation changed after the reconcile processed it.
// The reconcile is requeued so that the new value is processed.
var errRolloutAnnotationChanged = stderrors.New("rollout annotation changed during reconcile")

// rolloutAnnotationPath is the JSON pointer to the rollout annotation, with "~" and "/" escaped.
var rolloutAnnotationPath = "/metadata/annotations/" +
	strings.NewReplacer("~", "~0", "/", "~1").Replace(cappmeta.RolloutAnnotationKey)

// runRollout advances the rollout state machine of the Capp, stores the result in capp.Status.Rollout
// (persisted later by SyncStatus), emits the resulting events and consumes the rollout action annotation.
// The action annotation is left in place; SyncApplication removes it with removeRolloutAnnotation once the
// status was persisted, so a failed write does not lose the user's action.
// It returns how long to wait before the next timed rollout step, or 0 if none is needed.
func (r *CappReconciler) runRollout(ctx context.Context, capp *cappv1alpha1.Capp) (time.Duration, error) {
	ksvc := &knativev1.Service{}
	latestCreated, latestReady := "", ""
	var applied, served []cappv1alpha1.RevisionTraffic
	err := r.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, ksvc)
	switch {
	case err == nil:
		latestCreated = ksvc.Status.LatestCreatedRevisionName
		latestReady = ksvc.Status.LatestReadyRevisionName
		applied = appliedTraffic(ksvc)
		served = reportedTraffic(ksvc)
	case !errors.IsNotFound(err):
		return 0, fmt.Errorf("failed to get Knative Service: %w", err)
	}

	candidateReady, candidateCreated, err := r.revisionReady(ctx, capp.Namespace, latestCreated)
	if err != nil {
		return 0, err
	}

	annotation := capp.Annotations[cappmeta.RolloutAnnotationKey]
	action, actionRevision, valid := rollout.ParseAction(annotation)
	if !valid {
		r.EventRecorder.Eventf(capp, nil, corev1.EventTypeWarning, rollout.ReasonInvalidAction, actionRollout,
			"Invalid value %q of annotation %s; expected %s:<revision>, %s:<revision> or %s:<revision>",
			annotation, cappmeta.RolloutAnnotationKey, cappmeta.RolloutActionPromote, cappmeta.RolloutActionAbort, cappmeta.RolloutActionRetry)
	}

	in := rollout.Input{
		Strategy:         capp.Spec.RouteSpec.RolloutStrategy,
		LatestCreated:    latestCreated,
		LatestReady:      latestReady,
		CandidateReady:   candidateReady,
		CandidateCreated: candidateCreated,
		AppliedTraffic:   applied,
		ServedTraffic:    served,
		Action:           action,
		ActionRevision:   actionRevision,
		Now:              time.Now(),
	}
	if capp.Status.Rollout != nil {
		in.Previous = *capp.Status.Rollout
	}
	out := rollout.Step(in)

	if equality.Semantic.DeepEqual(out.Status, cappv1alpha1.RolloutStatus{}) {
		capp.Status.Rollout = nil
	} else {
		capp.Status.Rollout = &out.Status
	}

	for _, e := range out.Events {
		r.EventRecorder.Eventf(capp, nil, e.Type, e.Reason, actionRollout, "%s", e.Message)
	}

	return out.RequeueAfter, nil
}

// appliedTraffic returns the traffic Knative confirms is serving. It returns nil when the Knative Service status
// does not confirm it yet, because the status is stale or the routes are not ready.
func appliedTraffic(ksvc *knativev1.Service) []cappv1alpha1.RevisionTraffic {
	if ksvc.Status.ObservedGeneration != ksvc.Generation {
		return nil
	}
	routes := ksvc.Status.GetCondition(knativev1.ServiceConditionRoutesReady)
	if routes == nil || routes.Status != corev1.ConditionTrue {
		return nil
	}
	return reportedTraffic(ksvc)
}

// reportedTraffic returns the traffic the Knative Service status reports, whether or not Knative has applied it yet.
func reportedTraffic(ksvc *knativev1.Service) []cappv1alpha1.RevisionTraffic {
	traffic := make([]cappv1alpha1.RevisionTraffic, 0, len(ksvc.Status.Traffic))
	for _, t := range ksvc.Status.Traffic {
		var percent int64
		if t.Percent != nil {
			percent = *t.Percent
		}
		traffic = append(traffic, cappv1alpha1.RevisionTraffic{RevisionName: t.RevisionName, Percent: percent, Tag: t.Tag})
	}
	return traffic
}

// removeRolloutAnnotation removes the rollout action annotation that the reconcile processed. A JSON patch first
// tests that the annotation still has the processed value. If the user changed it meanwhile, the patch is rejected,
// the new value is kept and errRolloutAnnotationChanged is returned. It overwrites capp with the server response.
func (r *CappReconciler) removeRolloutAnnotation(ctx context.Context, capp *cappv1alpha1.Capp) error {
	processed, ok := capp.Annotations[cappmeta.RolloutAnnotationKey]
	if !ok {
		return nil
	}
	patch, err := json.Marshal([]map[string]any{
		{"op": "test", "path": rolloutAnnotationPath, "value": processed},
		{"op": "remove", "path": rolloutAnnotationPath},
	})
	if err != nil {
		return fmt.Errorf("failed to build annotation patch: %w", err)
	}
	if err := r.Patch(ctx, capp, client.RawPatch(types.JSONPatchType, patch)); err != nil {
		if errors.IsInvalid(err) {
			return errRolloutAnnotationChanged
		}
		return fmt.Errorf("failed to remove annotation %s: %w", cappmeta.RolloutAnnotationKey, err)
	}
	return nil
}

// revisionReady returns the Ready condition status and the creation time of the named Revision.
// The status is Unknown and the time is zero if the Revision does not exist.
func (r *CappReconciler) revisionReady(ctx context.Context, namespace, name string) (metav1.ConditionStatus, time.Time, error) {
	if name == "" {
		return metav1.ConditionUnknown, time.Time{}, nil
	}
	revision := &knativev1.Revision{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, revision); err != nil {
		if errors.IsNotFound(err) {
			return metav1.ConditionUnknown, time.Time{}, nil
		}
		return metav1.ConditionUnknown, time.Time{}, fmt.Errorf("failed to get Revision %s: %w", name, err)
	}
	created := revision.CreationTimestamp.Time
	if cond := revision.Status.GetCondition(knativeapis.ConditionReady); cond != nil {
		return metav1.ConditionStatus(cond.Status), created, nil
	}
	return metav1.ConditionUnknown, created, nil
}
