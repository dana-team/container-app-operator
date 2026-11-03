package status

import (
	"context"
	"fmt"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kapis "knative.dev/pkg/apis"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// revisionFailedReason is the reason of a RolloutSucceeded condition that is False because the latest revision failed.
const revisionFailedReason = "RevisionFailed"

// templateApplier reports whether a Knative Service holds the revision template that its Capp produces.
type templateApplier interface {
	TemplateApplied(ctx context.Context, capp cappv1alpha1.Capp, ksvc *knativev1.Service) bool
}

// setRolloutConditions sets the Available and RolloutSucceeded conditions. Neither feeds into the Ready summary.
// ksvcSyncErr is the error of applying the Knative Service, or nil. A failed apply leaves the previous revision serving,
// so the status of the rollout does not describe the new generation and RolloutSucceeded is False.
// RolloutSucceeded is removed when there is nothing to report about the rollout.
func setRolloutConditions(ctx context.Context, k8sClient client.Client, status *cappv1alpha1.CappStatus, capp cappv1alpha1.Capp, ksvcSyncErr error, templates templateApplier) error {
	available, err := checkAvailable(ctx, k8sClient, capp)
	if err != nil {
		return err
	}
	availableCond := conditionFromCheck(cappv1alpha1.CappConditionAvailable, available)
	availableCond.ObservedGeneration = capp.Generation
	meta.SetStatusCondition(&status.Conditions, availableCond)

	var succeeded *metav1.Condition
	if ksvcSyncErr != nil {
		succeeded = &metav1.Condition{
			Status:  metav1.ConditionFalse,
			Reason:  cappv1alpha1.CapabilityReasonResourceSyncFailed,
			Message: truncateConditionMessage(ksvcSyncErr.Error()),
		}
	} else {
		succeeded, err = rolloutSucceededCondition(ctx, k8sClient, capp, templates)
		if err != nil {
			return err
		}
	}
	if succeeded == nil {
		meta.RemoveStatusCondition(&status.Conditions, cappv1alpha1.CappConditionRolloutSucceeded)
		return nil
	}
	succeeded.Type = cappv1alpha1.CappConditionRolloutSucceeded
	succeeded.ObservedGeneration = capp.Generation
	meta.SetStatusCondition(&status.Conditions, *succeeded)
	return nil
}

// checkAvailable returns why the revisions serving traffic cannot serve it, or nil when they can. Every revision that
// gets traffic is checked, and a False reason is reported over an Unknown one.
// Revisions that get no traffic are ignored, so a failed candidate that was rolled back does not make the Capp unavailable.
func checkAvailable(ctx context.Context, k8sClient client.Client, capp cappv1alpha1.Capp) (*metav1.Condition, error) {
	ksvc := &knativev1.Service{}
	key := types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}
	if cond, err := getChildResource(ctx, k8sClient, rmanagers.KnativeService, key, ksvc); cond != nil || err != nil {
		return cond, err
	}
	if ksvc.Status.ObservedGeneration < ksvc.Generation {
		return newPendingCondition(rmanagers.KnativeService, ksvc.Name), nil
	}

	routes := ksvc.Status.GetCondition(knativev1.ServiceConditionRoutesReady)
	if routes == nil {
		return newPendingCondition(rmanagers.KnativeService, ksvc.Name), nil
	}
	if cond := formatChildCondition(rmanagers.KnativeService, ksvc.Name, metav1.ConditionStatus(routes.Status), routes.Reason, routes.Message); cond != nil {
		return cond, nil
	}

	if len(ksvc.Status.Traffic) == 0 {
		return &metav1.Condition{
			Status:  metav1.ConditionUnknown,
			Reason:  cappv1alpha1.CapabilityReasonPending,
			Message: fmt.Sprintf("Knative Service %s has no traffic targets yet", ksvc.Name),
		}, nil
	}
	var unknown *metav1.Condition
	for _, target := range ksvc.Status.Traffic {
		if target.Percent == nil || *target.Percent == 0 {
			continue
		}
		cond, err := checkRevisionReady(ctx, k8sClient, capp.Namespace, target.RevisionName)
		if err != nil {
			return nil, err
		}
		switch {
		case cond == nil:
		case cond.Status == metav1.ConditionFalse:
			return cond, nil
		case unknown == nil:
			unknown = cond
		}
	}
	return unknown, nil
}

// checkRevisionReady returns why the revision is not Ready, or nil when its Ready condition is True.
func checkRevisionReady(ctx context.Context, k8sClient client.Client, namespace, name string) (*metav1.Condition, error) {
	rev := &knativev1.Revision{}
	if cond, err := getChildResource(ctx, k8sClient, kindRevision, types.NamespacedName{Namespace: namespace, Name: name}, rev); cond != nil || err != nil {
		return cond, err
	}
	if rev.Status.ObservedGeneration < rev.Generation {
		return newPendingCondition(kindRevision, rev.Name), nil
	}

	ready := rev.Status.GetCondition(kapis.ConditionReady)
	if ready == nil {
		return newPendingCondition(kindRevision, rev.Name), nil
	}
	return formatChildCondition(kindRevision, rev.Name, metav1.ConditionStatus(ready.Status), ready.Reason, ready.Message), nil
}

// rolloutSucceededCondition returns whether the latest revision has fully rolled out. It is True only for a Stable
// rollout whose stable revision is the latest revision and that Knative confirms serves 100% of traffic. It is False
// when the latest revision failed. It is Unknown while the rollout is in progress or the first revision is not ready
// yet. It returns nil when there is nothing to report, which is a Capp without a Knative Service or created revision.
func rolloutSucceededCondition(ctx context.Context, k8sClient client.Client, capp cappv1alpha1.Capp, templates templateApplier) (*metav1.Condition, error) {
	rollout := capp.Status.Rollout
	if rollout == nil {
		return failedLatestRevisionCondition(ctx, k8sClient, capp)
	}
	switch rollout.Phase {
	case cappv1alpha1.RolloutPhaseFailed, cappv1alpha1.RolloutPhaseRolledBack, cappv1alpha1.RolloutPhaseAborted:
		return &metav1.Condition{
			Status:  metav1.ConditionFalse,
			Reason:  string(rollout.Phase),
			Message: truncateConditionMessage(rollout.Message),
		}, nil
	case cappv1alpha1.RolloutPhaseProgressing, cappv1alpha1.RolloutPhaseHolding, cappv1alpha1.RolloutPhasePromoting:
		return &metav1.Condition{
			Status:  metav1.ConditionUnknown,
			Reason:  string(rollout.Phase),
			Message: truncateConditionMessage(rollout.Message),
		}, nil
	case cappv1alpha1.RolloutPhaseStable:
		return checkStableConfirmed(ctx, k8sClient, capp, templates)
	default:
		return &metav1.Condition{
			Status:  metav1.ConditionUnknown,
			Reason:  cappv1alpha1.CapabilityReasonPending,
			Message: "rollout phase is not set yet",
		}, nil
	}
}

// failedLatestRevisionCondition returns the RolloutSucceeded condition of a Capp without a rollout status. It is False
// when its latest created revision failed, and Unknown otherwise until the rollout status is recorded, which is the case on
// the first deploy before any stable revision exists. It returns nil when there is no Knative Service or no created
// revision.
func failedLatestRevisionCondition(ctx context.Context, k8sClient client.Client, capp cappv1alpha1.Capp) (*metav1.Condition, error) {
	ksvc := &knativev1.Service{}
	key := types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}
	if cond, err := getChildResource(ctx, k8sClient, rmanagers.KnativeService, key, ksvc); cond != nil || err != nil {
		return nil, err
	}
	latest := ksvc.Status.LatestCreatedRevisionName
	if latest == "" {
		return nil, nil
	}
	failed, err := revisionFailedCondition(ctx, k8sClient, capp.Namespace, latest)
	if failed != nil || err != nil {
		return failed, err
	}
	return &metav1.Condition{
		Status:  metav1.ConditionUnknown,
		Reason:  cappv1alpha1.CapabilityReasonPending,
		Message: fmt.Sprintf("rollout status for latest revision %s is not recorded yet", latest),
	}, nil
}

// revisionFailedCondition returns the RolloutSucceeded condition that is False because the named revision is not Ready.
// It returns nil when the revision is Ready, still progressing, missing or unnamed.
func revisionFailedCondition(ctx context.Context, k8sClient client.Client, namespace, name string) (*metav1.Condition, error) {
	if name == "" {
		return nil, nil
	}
	rev := &knativev1.Revision{}
	if cond, err := getChildResource(ctx, k8sClient, kindRevision, types.NamespacedName{Namespace: namespace, Name: name}, rev); cond != nil || err != nil {
		return nil, err
	}
	ready := rev.Status.GetCondition(kapis.ConditionReady)
	if ready == nil || ready.Status != corev1.ConditionFalse {
		return nil, nil
	}
	message := fmt.Sprintf("revision %s failed", name)
	if ready.Message != "" {
		message += ": " + ready.Message
	}
	return &metav1.Condition{
		Status:  metav1.ConditionFalse,
		Reason:  revisionFailedReason,
		Message: truncateConditionMessage(message),
	}, nil
}

// checkStableConfirmed returns the RolloutSucceeded condition of a Stable rollout. It is True once the Knative Service
// holds every field set in the desired template, its configurations are ready, and Knative confirms that the stable
// revision, which must be the latest revision, serves 100% of traffic.
func checkStableConfirmed(ctx context.Context, k8sClient client.Client, capp cappv1alpha1.Capp, templates templateApplier) (*metav1.Condition, error) {
	ksvc := &knativev1.Service{}
	key := types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}
	if cond, err := getChildResource(ctx, k8sClient, rmanagers.KnativeService, key, ksvc); err != nil {
		return nil, err
	} else if cond != nil {
		return cond, nil
	}

	stable := capp.Status.Rollout.StableRevision
	pending := func(message string) *metav1.Condition {
		return &metav1.Condition{
			Status:  metav1.ConditionUnknown,
			Reason:  cappv1alpha1.CapabilityReasonPending,
			Message: message,
		}
	}

	if latest := ksvc.Status.LatestCreatedRevisionName; latest != stable {
		failed, err := revisionFailedCondition(ctx, k8sClient, capp.Namespace, latest)
		if failed != nil || err != nil {
			return failed, err
		}
		return pending(fmt.Sprintf("latest revision %s is not stable yet", latest)), nil
	}
	if ksvc.Status.ObservedGeneration < ksvc.Generation {
		return pending(fmt.Sprintf("Knative Service %s not observed yet", ksvc.Name)), nil
	}
	// A stale template means the apply of the latest Capp update is not visible in the Knative Service yet, even though
	// the old revision still serves 100% of traffic.
	if templates == nil {
		return pending(fmt.Sprintf("the template of Knative Service %s cannot be verified", ksvc.Name)), nil
	}
	if !templates.TemplateApplied(ctx, capp, ksvc) {
		return pending(fmt.Sprintf("Knative Service %s does not hold the desired template yet", ksvc.Name)), nil
	}
	// Knative marks the configurations as not ready while it has not reconciled the Configuration of the new generation,
	// and the routes can still be ready with the old revision.
	configurations := ksvc.Status.GetCondition(knativev1.ServiceConditionConfigurationsReady)
	if configurations == nil || !configurations.IsTrue() {
		if configurations != nil && configurations.IsFalse() {
			if failed, err := revisionFailedCondition(ctx, k8sClient, capp.Namespace, stable); failed != nil || err != nil {
				return failed, err
			}
		}
		return pending(fmt.Sprintf("configurations of Knative Service %s are not ready", ksvc.Name)), nil
	}
	routes := ksvc.Status.GetCondition(knativev1.ServiceConditionRoutesReady)
	if routes == nil || !routes.IsTrue() {
		return pending(fmt.Sprintf("routes of Knative Service %s are not ready", ksvc.Name)), nil
	}
	if !servesAll(ksvc.Status.Traffic, stable) {
		return pending(fmt.Sprintf("Knative does not yet serve 100%% of traffic to revision %s", stable)), nil
	}

	return &metav1.Condition{
		Status:  metav1.ConditionTrue,
		Reason:  string(cappv1alpha1.RolloutPhaseStable),
		Message: fmt.Sprintf("revision %s serves 100%% of traffic", stable),
	}, nil
}

// servesAll reports whether the traffic gives revision 100%.
func servesAll(traffic []knativev1.TrafficTarget, revision string) bool {
	for _, target := range traffic {
		if target.RevisionName == revision && target.Percent != nil && *target.Percent == 100 {
			return true
		}
	}
	return false
}

// conditionFromCheck returns the condition of a capability check. A nil check result means the check passed.
func conditionFromCheck(conditionType string, cond *metav1.Condition) metav1.Condition {
	if cond == nil {
		return metav1.Condition{Type: conditionType, Status: metav1.ConditionTrue, Reason: cappv1alpha1.CapabilityReasonReady}
	}
	result := *cond
	result.Type = conditionType
	return result
}
