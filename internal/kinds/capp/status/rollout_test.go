package status

import (
	"context"
	"errors"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	kapis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	stableRevision    = "rev-1"
	candidateRevision = "rev-2"
)

// testTemplates checks the template of a Knative Service with the real manager, so that fixtures can hold the template
// the Capp produces.
var testTemplates templateApplier = rmanagers.KnativeServiceManager{CappConfig: newCappConfig()}

// newRolloutCapp returns newCapp() with a container, so that its desired template has a field that
// newStaleRolloutKsvc does not hold. A Capp without containers would have an empty desired template, which matches
// any Knative Service.
func newRolloutCapp() cappv1alpha1.Capp {
	capp := newCapp()
	capp.Spec.ConfigurationSpec.Template.Spec.Containers = []corev1.Container{{Name: "app", Image: "example.com/app:new"}}
	return capp
}

// desiredTemplate returns the template of the Knative Service that newRolloutCapp() must have.
func desiredTemplate() knativev1.RevisionTemplateSpec {
	return rmanagers.KnativeServiceManager{CappConfig: newCappConfig()}.DesiredTemplate(context.Background(), newRolloutCapp())
}

// newRolloutKsvc returns a Knative Service whose latest created revision is latest and whose routes have routesReady.
// Its configurations are ready and it holds the desired template.
func newRolloutKsvc(latest string, routesReady corev1.ConditionStatus, traffic ...knativev1.TrafficTarget) *knativev1.Service {
	return newRolloutKsvcWithConfigurations(latest, corev1.ConditionTrue, routesReady, traffic...)
}

// newRolloutKsvcWithConfigurations returns a Knative Service like newRolloutKsvc with the given ConfigurationsReady status.
func newRolloutKsvcWithConfigurations(latest string, configurationsReady, routesReady corev1.ConditionStatus, traffic ...knativev1.TrafficTarget) *knativev1.Service {
	ksvc := newKsvc(latest, latest,
		kapis.Condition{Type: kapis.ConditionReady, Status: corev1.ConditionTrue},
		kapis.Condition{Type: knativev1.ServiceConditionConfigurationsReady, Status: configurationsReady},
		kapis.Condition{Type: knativev1.ServiceConditionRoutesReady, Status: routesReady},
	)
	ksvc.Status.Traffic = traffic
	ksvc.Spec.Template = desiredTemplate()
	return ksvc
}

// newStaleRolloutKsvc returns a rollout Knative Service that still holds a template from before the latest Capp update.
func newStaleRolloutKsvc(latest string, routesReady corev1.ConditionStatus, traffic ...knativev1.TrafficTarget) *knativev1.Service {
	ksvc := newRolloutKsvc(latest, routesReady, traffic...)
	ksvc.Spec.Template.Spec.Containers = []corev1.Container{{Name: "app", Image: "example.com/app:old"}}
	return ksvc
}

func trafficTo(revision string, percent int64) knativev1.TrafficTarget {
	return knativev1.TrafficTarget{RevisionName: revision, Percent: ptr.To(percent)}
}

// newRevisionWithReady returns a Revision whose Ready condition has the given status.
func newRevisionWithReady(name string, ready corev1.ConditionStatus) *knativev1.Revision {
	rev := newRevision(name, metav1.Now())
	rev.Status.Conditions = duckv1.Conditions{{Type: kapis.ConditionReady, Status: ready}}
	return rev
}

func newRolloutClient(objects ...client.Object) client.Client {
	return fake.NewClientBuilder().WithScheme(newStatusScheme()).WithObjects(objects...).Build()
}

func TestRolloutSucceededCondition(t *testing.T) {
	const message = "rollout message"
	stable := func(phase cappv1alpha1.RolloutPhase) *cappv1alpha1.RolloutStatus {
		return &cappv1alpha1.RolloutStatus{StableRevision: stableRevision, Phase: phase, Message: message}
	}

	tests := []struct {
		name        string
		rollout     *cappv1alpha1.RolloutStatus
		objects     []client.Object
		wantStatus  metav1.ConditionStatus
		wantReason  string
		wantMessage string // checked instead of the rollout message when set
	}{
		{
			name:       "true when stable is the latest revision and serves 100%",
			rollout:    stable(cappv1alpha1.RolloutPhaseStable),
			objects:    []client.Object{newRolloutKsvc(stableRevision, corev1.ConditionTrue, trafficTo(stableRevision, 100))},
			wantStatus: metav1.ConditionTrue,
			wantReason: string(cappv1alpha1.RolloutPhaseStable),
		},
		{
			name:    "true when only the spec traffic differs from the desired template",
			rollout: stable(cappv1alpha1.RolloutPhaseStable),
			objects: []client.Object{func() client.Object {
				ksvc := newRolloutKsvc(stableRevision, corev1.ConditionTrue, trafficTo(stableRevision, 100))
				ksvc.Spec.Traffic = []knativev1.TrafficTarget{trafficTo(candidateRevision, 100)}
				return ksvc
			}()},
			wantStatus: metav1.ConditionTrue,
			wantReason: string(cappv1alpha1.RolloutPhaseStable),
		},
		{
			name:       "unknown when the Knative Service holds a stale template, which Knative still serves at 100%",
			rollout:    stable(cappv1alpha1.RolloutPhaseStable),
			objects:    []client.Object{newStaleRolloutKsvc(stableRevision, corev1.ConditionTrue, trafficTo(stableRevision, 100))},
			wantStatus: metav1.ConditionUnknown,
			wantReason: cappv1alpha1.CapabilityReasonPending,
		},
		{
			name:       "unknown when the configurations are not ready, although the routes are ready",
			rollout:    stable(cappv1alpha1.RolloutPhaseStable),
			objects:    []client.Object{newRolloutKsvcWithConfigurations(stableRevision, corev1.ConditionUnknown, corev1.ConditionTrue, trafficTo(stableRevision, 100))},
			wantStatus: metav1.ConditionUnknown,
			wantReason: cappv1alpha1.CapabilityReasonPending,
		},
		{
			name:        "false with RevisionFailed when the configurations fail and the stable revision failed",
			rollout:     stable(cappv1alpha1.RolloutPhaseStable),
			objects:     []client.Object{newRolloutKsvcWithConfigurations(stableRevision, corev1.ConditionFalse, corev1.ConditionTrue, trafficTo(stableRevision, 100)), newRevisionWithReady(stableRevision, corev1.ConditionFalse)},
			wantStatus:  metav1.ConditionFalse,
			wantReason:  revisionFailedReason,
			wantMessage: "revision " + stableRevision + " failed",
		},
		{
			name:       "unknown when the configurations are not ready and the stable revision is ready",
			rollout:    stable(cappv1alpha1.RolloutPhaseStable),
			objects:    []client.Object{newRolloutKsvcWithConfigurations(stableRevision, corev1.ConditionUnknown, corev1.ConditionTrue, trafficTo(stableRevision, 100)), newRevisionWithReady(stableRevision, corev1.ConditionTrue)},
			wantStatus: metav1.ConditionUnknown,
			wantReason: cappv1alpha1.CapabilityReasonPending,
		},
		{
			name:       "unknown when the configurations are not ready and the stable revision failed",
			rollout:    stable(cappv1alpha1.RolloutPhaseStable),
			objects:    []client.Object{newRolloutKsvcWithConfigurations(stableRevision, corev1.ConditionUnknown, corev1.ConditionTrue, trafficTo(stableRevision, 100)), newRevisionWithReady(stableRevision, corev1.ConditionFalse)},
			wantStatus: metav1.ConditionUnknown,
			wantReason: cappv1alpha1.CapabilityReasonPending,
		},
		{
			name:       "unknown when stable is confirmed but the split is still applied",
			rollout:    stable(cappv1alpha1.RolloutPhaseStable),
			objects:    []client.Object{newRolloutKsvc(stableRevision, corev1.ConditionTrue, trafficTo(stableRevision, 50), trafficTo(candidateRevision, 50))},
			wantStatus: metav1.ConditionUnknown,
			wantReason: cappv1alpha1.CapabilityReasonPending,
		},
		{
			name:       "unknown when routes are not ready",
			rollout:    stable(cappv1alpha1.RolloutPhaseStable),
			objects:    []client.Object{newRolloutKsvc(stableRevision, corev1.ConditionUnknown, trafficTo(stableRevision, 100))},
			wantStatus: metav1.ConditionUnknown,
			wantReason: cappv1alpha1.CapabilityReasonPending,
		},
		{
			name:       "unknown when a newer revision was created",
			rollout:    stable(cappv1alpha1.RolloutPhaseStable),
			objects:    []client.Object{newRolloutKsvc(candidateRevision, corev1.ConditionTrue, trafficTo(stableRevision, 100))},
			wantStatus: metav1.ConditionUnknown,
			wantReason: cappv1alpha1.CapabilityReasonPending,
		},
		{
			name:       "unknown when there is no Knative Service",
			rollout:    stable(cappv1alpha1.RolloutPhaseStable),
			wantStatus: metav1.ConditionUnknown,
			wantReason: cappv1alpha1.CapabilityReasonPending,
		},
		{
			name:       "unknown when progressing",
			rollout:    stable(cappv1alpha1.RolloutPhaseProgressing),
			wantStatus: metav1.ConditionUnknown,
			wantReason: string(cappv1alpha1.RolloutPhaseProgressing),
		},
		{
			name:       "unknown when holding",
			rollout:    stable(cappv1alpha1.RolloutPhaseHolding),
			wantStatus: metav1.ConditionUnknown,
			wantReason: string(cappv1alpha1.RolloutPhaseHolding),
		},
		{
			name:       "unknown when promoting",
			rollout:    stable(cappv1alpha1.RolloutPhasePromoting),
			wantStatus: metav1.ConditionUnknown,
			wantReason: string(cappv1alpha1.RolloutPhasePromoting),
		},
		{
			name:       "false when failed",
			rollout:    stable(cappv1alpha1.RolloutPhaseFailed),
			wantStatus: metav1.ConditionFalse,
			wantReason: string(cappv1alpha1.RolloutPhaseFailed),
		},
		{
			name:       "false when rolled back",
			rollout:    stable(cappv1alpha1.RolloutPhaseRolledBack),
			wantStatus: metav1.ConditionFalse,
			wantReason: string(cappv1alpha1.RolloutPhaseRolledBack),
		},
		{
			name:       "false when aborted",
			rollout:    stable(cappv1alpha1.RolloutPhaseAborted),
			wantStatus: metav1.ConditionFalse,
			wantReason: string(cappv1alpha1.RolloutPhaseAborted),
		},
		{
			name:        "false when the latest revision failed without a strategy",
			rollout:     stable(cappv1alpha1.RolloutPhaseStable),
			objects:     []client.Object{newRolloutKsvc(candidateRevision, corev1.ConditionTrue, trafficTo(stableRevision, 100)), newRevisionWithReady(candidateRevision, corev1.ConditionFalse)},
			wantStatus:  metav1.ConditionFalse,
			wantReason:  revisionFailedReason,
			wantMessage: "revision " + candidateRevision + " failed",
		},
		{
			name:       "unknown when the latest revision is still progressing without a strategy",
			rollout:    stable(cappv1alpha1.RolloutPhaseStable),
			objects:    []client.Object{newRolloutKsvc(candidateRevision, corev1.ConditionTrue, trafficTo(stableRevision, 100)), newRevisionWithReady(candidateRevision, corev1.ConditionUnknown)},
			wantStatus: metav1.ConditionUnknown,
			wantReason: cappv1alpha1.CapabilityReasonPending,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capp := newRolloutCapp()
			capp.Status.Rollout = tt.rollout

			cond, err := rolloutSucceededCondition(context.Background(), newRolloutClient(tt.objects...), capp, testTemplates)
			require.NoError(t, err)
			require.NotNil(t, cond)

			assert.Equal(t, tt.wantStatus, cond.Status)
			assert.Equal(t, tt.wantReason, cond.Reason)
			switch {
			case tt.wantMessage != "":
				assert.Contains(t, cond.Message, tt.wantMessage)
			case tt.wantStatus == metav1.ConditionFalse:
				assert.Equal(t, message, cond.Message)
			}
		})
	}
}

func TestRolloutSucceededConditionWithoutRollout(t *testing.T) {
	tests := []struct {
		name       string
		objects    []client.Object
		wantStatus metav1.ConditionStatus // empty when there is no condition
		wantReason string                 // checked when wantStatus is set
	}{
		{
			name:       "false when the latest revision failed",
			objects:    []client.Object{newRolloutKsvc(candidateRevision, corev1.ConditionTrue), newRevisionWithReady(candidateRevision, corev1.ConditionFalse)},
			wantStatus: metav1.ConditionFalse,
			wantReason: revisionFailedReason,
		},
		{
			name:       "unknown when the latest revision is ready but no rollout status exists yet",
			objects:    []client.Object{newRolloutKsvc(candidateRevision, corev1.ConditionTrue), newRevisionWithReady(candidateRevision, corev1.ConditionTrue)},
			wantStatus: metav1.ConditionUnknown,
			wantReason: cappv1alpha1.CapabilityReasonPending,
		},
		{
			name:       "unknown when the latest revision is still progressing",
			objects:    []client.Object{newRolloutKsvc(candidateRevision, corev1.ConditionTrue), newRevisionWithReady(candidateRevision, corev1.ConditionUnknown)},
			wantStatus: metav1.ConditionUnknown,
			wantReason: cappv1alpha1.CapabilityReasonPending,
		},
		{
			name:       "unknown when the latest revision is missing",
			objects:    []client.Object{newRolloutKsvc(candidateRevision, corev1.ConditionTrue)},
			wantStatus: metav1.ConditionUnknown,
			wantReason: cappv1alpha1.CapabilityReasonPending,
		},
		{
			name:    "no condition when there is no Knative Service",
			objects: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capp := newCapp()
			capp.Status.Rollout = nil

			cond, err := rolloutSucceededCondition(context.Background(), newRolloutClient(tt.objects...), capp, testTemplates)
			require.NoError(t, err)
			if tt.wantStatus == "" {
				assert.Nil(t, cond)
				return
			}
			require.NotNil(t, cond)
			assert.Equal(t, tt.wantStatus, cond.Status)
			assert.Equal(t, tt.wantReason, cond.Reason)
			if tt.wantStatus == metav1.ConditionFalse {
				assert.Contains(t, cond.Message, "revision "+candidateRevision+" failed")
			}
		})
	}
}

func TestRolloutSucceededConditionWithoutTemplateChecker(t *testing.T) {
	capp := newCapp()
	capp.Status.Rollout = &cappv1alpha1.RolloutStatus{StableRevision: stableRevision, Phase: cappv1alpha1.RolloutPhaseStable}
	k8sClient := newRolloutClient(newRolloutKsvc(stableRevision, corev1.ConditionTrue, trafficTo(stableRevision, 100)),
		newRevisionWithReady(stableRevision, corev1.ConditionTrue))

	// Without the Knative Service manager the template cannot be compared, so the rollout must not be confirmed.
	cond, err := rolloutSucceededCondition(context.Background(), k8sClient, capp, nil)
	require.NoError(t, err)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionUnknown, cond.Status)
	assert.Equal(t, cappv1alpha1.CapabilityReasonPending, cond.Reason)
}

func TestSetRolloutConditionsRevisionFailedWithoutStrategy(t *testing.T) {
	capp := newCapp()
	capp.Status.Rollout = &cappv1alpha1.RolloutStatus{StableRevision: stableRevision, Phase: cappv1alpha1.RolloutPhaseStable}
	k8sClient := newRolloutClient(
		newRolloutKsvc(candidateRevision, corev1.ConditionTrue, trafficTo(stableRevision, 100)),
		newRevisionWithReady(candidateRevision, corev1.ConditionFalse),
	)

	status := &cappv1alpha1.CappStatus{}
	require.NoError(t, setRolloutConditions(context.Background(), k8sClient, status, capp, nil, testTemplates))
	succeeded := meta.FindStatusCondition(status.Conditions, cappv1alpha1.CappConditionRolloutSucceeded)
	require.NotNil(t, succeeded)
	assert.Equal(t, metav1.ConditionFalse, succeeded.Status)
	assert.Equal(t, revisionFailedReason, succeeded.Reason)
}

// TestSetRolloutConditionsStaleTemplateAtNewGeneration covers a Capp update whose Knative Service still holds the old
// template at the old generation. This happens when the apply succeeded but SyncStatus reads a cached Knative Service
// older than the write, and when the update conflicted, which is not reported as a sync error. Both reach
// setRolloutConditions as a nil sync error, so RolloutSucceeded must not be True for the new generation.
func TestSetRolloutConditionsStaleTemplateAtNewGeneration(t *testing.T) {
	capp := newRolloutCapp()
	capp.Generation = 2
	capp.Status.Rollout = &cappv1alpha1.RolloutStatus{StableRevision: stableRevision, Phase: cappv1alpha1.RolloutPhaseStable}
	ksvc := newStaleRolloutKsvc(stableRevision, corev1.ConditionTrue, trafficTo(stableRevision, 100))
	ksvc.Generation = 1
	ksvc.Status.ObservedGeneration = 1
	k8sClient := newRolloutClient(ksvc, newRevisionWithReady(stableRevision, corev1.ConditionTrue))

	status := &cappv1alpha1.CappStatus{}
	require.NoError(t, setRolloutConditions(context.Background(), k8sClient, status, capp, nil, testTemplates))
	succeeded := meta.FindStatusCondition(status.Conditions, cappv1alpha1.CappConditionRolloutSucceeded)
	require.NotNil(t, succeeded)
	assert.Equal(t, metav1.ConditionUnknown, succeeded.Status)
	assert.Equal(t, int64(2), succeeded.ObservedGeneration)
}

func TestSetRolloutConditionsKnativeServiceSyncError(t *testing.T) {
	capp := newCapp()
	capp.Generation = 2
	capp.Status.Rollout = &cappv1alpha1.RolloutStatus{StableRevision: stableRevision, Phase: cappv1alpha1.RolloutPhaseStable}
	// The Knative Service still serves the stable revision at 100%, because applying the new generation failed.
	k8sClient := newRolloutClient(
		newRolloutKsvc(stableRevision, corev1.ConditionTrue, trafficTo(stableRevision, 100)),
		newRevisionWithReady(stableRevision, corev1.ConditionTrue),
	)
	applyErr := errors.New("admission webhook denied the request")

	status := &cappv1alpha1.CappStatus{}
	require.NoError(t, setRolloutConditions(context.Background(), k8sClient, status, capp, applyErr, testTemplates))
	succeeded := meta.FindStatusCondition(status.Conditions, cappv1alpha1.CappConditionRolloutSucceeded)
	require.NotNil(t, succeeded)
	assert.Equal(t, metav1.ConditionFalse, succeeded.Status)
	assert.Equal(t, cappv1alpha1.CapabilityReasonResourceSyncFailed, succeeded.Reason)
	assert.Equal(t, applyErr.Error(), succeeded.Message)
	assert.Equal(t, int64(2), succeeded.ObservedGeneration)
	available := meta.FindStatusCondition(status.Conditions, cappv1alpha1.CappConditionAvailable)
	require.NotNil(t, available)
	assert.Equal(t, metav1.ConditionTrue, available.Status)
}

func TestSetRolloutConditionsWithoutSyncErrorReportsStableRollout(t *testing.T) {
	capp := newRolloutCapp()
	capp.Generation = 2
	capp.Status.Rollout = &cappv1alpha1.RolloutStatus{StableRevision: stableRevision, Phase: cappv1alpha1.RolloutPhaseStable}
	k8sClient := newRolloutClient(
		newRolloutKsvc(stableRevision, corev1.ConditionTrue, trafficTo(stableRevision, 100)),
		newRevisionWithReady(stableRevision, corev1.ConditionTrue),
	)

	status := &cappv1alpha1.CappStatus{}
	require.NoError(t, setRolloutConditions(context.Background(), k8sClient, status, capp, nil, testTemplates))
	succeeded := meta.FindStatusCondition(status.Conditions, cappv1alpha1.CappConditionRolloutSucceeded)
	require.NotNil(t, succeeded)
	assert.Equal(t, metav1.ConditionTrue, succeeded.Status)
	assert.Equal(t, int64(2), succeeded.ObservedGeneration)
}

func TestCheckAvailable(t *testing.T) {
	readyRevisions := []client.Object{
		newRevisionWithReady(stableRevision, corev1.ConditionTrue),
	}
	tests := []struct {
		name        string
		objects     []client.Object
		wantStatus  metav1.ConditionStatus // empty when available
		wantMessage string
	}{
		{
			name:       "available when the serving revision is ready",
			objects:    append(readyRevisions, newRolloutKsvc(stableRevision, corev1.ConditionTrue, trafficTo(stableRevision, 100))),
			wantStatus: "",
		},
		{
			name: "ignores a 0% target that is not ready",
			objects: append(readyRevisions,
				newRevisionWithReady(candidateRevision, corev1.ConditionFalse),
				newRolloutKsvc(stableRevision, corev1.ConditionTrue, trafficTo(stableRevision, 100), trafficTo(candidateRevision, 0))),
			wantStatus: "",
		},
		{
			name: "false when a serving target is not ready",
			objects: append(readyRevisions,
				newRevisionWithReady(candidateRevision, corev1.ConditionFalse),
				newRolloutKsvc(stableRevision, corev1.ConditionTrue, trafficTo(stableRevision, 50), trafficTo(candidateRevision, 50))),
			wantStatus:  metav1.ConditionFalse,
			wantMessage: "Revision " + candidateRevision,
		},
		{
			name: "unknown when a serving target has no revision yet",
			objects: append(readyRevisions,
				newRolloutKsvc(stableRevision, corev1.ConditionTrue, trafficTo(stableRevision, 50), trafficTo(candidateRevision, 50))),
			wantStatus: metav1.ConditionUnknown,
		},
		{
			name:       "false when routes are not ready",
			objects:    append(readyRevisions, newRolloutKsvc(stableRevision, corev1.ConditionFalse, trafficTo(stableRevision, 100))),
			wantStatus: metav1.ConditionFalse,
		},
		{
			name: "unknown then false reports false",
			objects: append(
				[]client.Object{newRevisionWithReady(stableRevision, corev1.ConditionUnknown)},
				newRevisionWithReady(candidateRevision, corev1.ConditionFalse),
				newRolloutKsvc(stableRevision, corev1.ConditionTrue, trafficTo(stableRevision, 50), trafficTo(candidateRevision, 50))),
			wantStatus:  metav1.ConditionFalse,
			wantMessage: "Revision " + candidateRevision,
		},
		{
			name: "false then unknown reports false",
			objects: append(
				[]client.Object{newRevisionWithReady(candidateRevision, corev1.ConditionFalse), newRevisionWithReady(stableRevision, corev1.ConditionUnknown)},
				newRolloutKsvc(stableRevision, corev1.ConditionTrue, trafficTo(stableRevision, 50), trafficTo(candidateRevision, 50))),
			wantStatus:  metav1.ConditionFalse,
			wantMessage: "Revision " + candidateRevision,
		},
		{
			name: "unknown then ready reports unknown",
			objects: append(
				[]client.Object{newRevisionWithReady(stableRevision, corev1.ConditionUnknown), newRevisionWithReady(candidateRevision, corev1.ConditionTrue)},
				newRolloutKsvc(stableRevision, corev1.ConditionTrue, trafficTo(stableRevision, 50), trafficTo(candidateRevision, 50))),
			wantStatus: metav1.ConditionUnknown,
		},
		{
			name: "all serving revisions ready",
			objects: append(
				[]client.Object{newRevisionWithReady(stableRevision, corev1.ConditionTrue), newRevisionWithReady(candidateRevision, corev1.ConditionTrue)},
				newRolloutKsvc(stableRevision, corev1.ConditionTrue, trafficTo(stableRevision, 50), trafficTo(candidateRevision, 50))),
			wantStatus: "",
		},
		{
			name:       "unknown when routes are being updated",
			objects:    append(readyRevisions, newRolloutKsvc(stableRevision, corev1.ConditionUnknown, trafficTo(stableRevision, 100))),
			wantStatus: metav1.ConditionUnknown,
		},
		{
			name:       "unknown when the traffic is empty",
			objects:    append(readyRevisions, newRolloutKsvc(stableRevision, corev1.ConditionTrue)),
			wantStatus: metav1.ConditionUnknown,
		},
		{
			name:       "unknown when there is no Knative Service",
			wantStatus: metav1.ConditionUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := checkAvailable(context.Background(), newRolloutClient(tt.objects...), newCapp())
			require.NoError(t, err)

			if tt.wantStatus == "" {
				assert.Nil(t, cond)
				return
			}
			require.NotNil(t, cond)
			assert.Equal(t, tt.wantStatus, cond.Status)
			if tt.wantMessage != "" {
				assert.Contains(t, cond.Message, tt.wantMessage)
			}
		})
	}
}
