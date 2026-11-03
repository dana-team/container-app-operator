package rollout

import (
	"fmt"
	"testing"
	"time"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const rolledBackMessage = "rolled back"

var now = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

func dur(d time.Duration) *metav1.Duration { return &metav1.Duration{Duration: d} }

func ago(d time.Duration) *metav1.Time {
	t := metav1.NewTime(now.Add(-d))
	return &t
}

func nowTime() *metav1.Time {
	t := metav1.NewTime(now)
	return &t
}

func canary(autoRollback bool) *cappv1alpha1.RolloutStrategy {
	return &cappv1alpha1.RolloutStrategy{
		AutoRollback: autoRollback,
		Stages: []cappv1alpha1.RolloutStage{
			{Percent: 10, Duration: dur(5 * time.Minute)},
			{Percent: 50, Duration: dur(10 * time.Minute)},
			{Percent: 100},
		},
	}
}

func split(stablePercent int64, candidatePercent int64) []cappv1alpha1.RevisionTraffic {
	return []cappv1alpha1.RevisionTraffic{
		{RevisionName: "r1", Percent: stablePercent, Tag: TagStable},
		{RevisionName: "r2", Percent: candidatePercent, Tag: TagCandidate},
	}
}

func stableOnly(stable string) []cappv1alpha1.RevisionTraffic {
	return []cappv1alpha1.RevisionTraffic{{RevisionName: stable, Percent: 100, Tag: TagStable}}
}

// progressing returns a status of a rollout from r1 to r2 at the given stage.
func progressing(stageIndex int32, stageStart *metav1.Time, percent int64) cappv1alpha1.RolloutStatus {
	return cappv1alpha1.RolloutStatus{
		StableRevision:     "r1",
		CandidateRevision:  "r2",
		Phase:              cappv1alpha1.RolloutPhaseProgressing,
		StageIndex:         stageIndex,
		StageStartTime:     stageStart,
		CandidateSince:     ago(30 * time.Minute),
		CandidateReadyOnce: stageStart != nil,
		Traffic:            split(100-percent, percent),
	}
}

// awaitingSplit returns a status of a ready candidate at the given stage whose split Knative has not confirmed yet.
func awaitingSplit(stageIndex int32, percent int64) cappv1alpha1.RolloutStatus {
	status := progressing(stageIndex, nil, percent)
	status.CandidateReadyOnce = true
	return status
}

// stepWithApplied steps a ready candidate r2 that Knative reports as serving the applied traffic.
func stepWithApplied(prev cappv1alpha1.RolloutStatus, applied []cappv1alpha1.RevisionTraffic) Output {
	return Step(Input{
		Strategy:       canary(true),
		Previous:       prev,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		AppliedTraffic: applied,
		Now:            now,
	})
}

func requireEvent(t *testing.T, events []Event, eventType, reason string) Event {
	t.Helper()
	for _, e := range events {
		if e.Reason == reason {
			require.Equal(t, eventType, e.Type)
			require.NotEmpty(t, e.Message)
			return e
		}
	}
	require.Failf(t, "event not found", "no event with reason %q in %v", reason, events)
	return Event{}
}

func TestStep_NoKsvcResets(t *testing.T) {
	out := Step(Input{
		Strategy: canary(true),
		Previous: progressing(1, ago(time.Minute), 50),
		Now:      now,
	})
	require.Equal(t, cappv1alpha1.RolloutStatus{}, out.Status)
	require.Zero(t, out.RequeueAfter)
	require.Empty(t, out.Events)
}

func TestStep_NoStrategyTracksStable(t *testing.T) {
	out := Step(Input{
		LatestCreated: "r1",
		LatestReady:   "r1",
		Now:           now,
	})
	require.Equal(t, "r1", out.Status.StableRevision)
	require.Equal(t, cappv1alpha1.RolloutPhaseStable, out.Status.Phase)
	require.Empty(t, out.Status.Traffic)
	require.Empty(t, out.Status.CandidateRevision)
	require.Zero(t, out.RequeueAfter)
}

func TestStep_FirstRevision(t *testing.T) {
	out := Step(Input{
		Strategy:       canary(true),
		LatestCreated:  "r1",
		LatestReady:    "r1",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	require.Equal(t, "r1", out.Status.StableRevision)
	require.Empty(t, out.Status.CandidateRevision)
	require.Equal(t, stableOnly("r1"), out.Status.Traffic)
	require.Equal(t, cappv1alpha1.RolloutPhaseStable, out.Status.Phase)
	require.Zero(t, out.RequeueAfter)
}

func TestStep_FirstRevisionNotReady(t *testing.T) {
	out := Step(Input{
		Strategy:       canary(true),
		LatestCreated:  "r1",
		CandidateReady: metav1.ConditionUnknown,
		Now:            now,
	})
	require.Empty(t, out.Status.Traffic)
	require.Empty(t, out.Status.StableRevision)
	require.Empty(t, out.Status.CandidateRevision)
}

func TestStep_NewRevisionStartsRollout(t *testing.T) {
	out := Step(Input{
		Strategy:       canary(true),
		Previous:       cappv1alpha1.RolloutStatus{StableRevision: "r1", Phase: cappv1alpha1.RolloutPhaseStable, Traffic: stableOnly("r1")},
		LatestCreated:  "r2",
		LatestReady:    "r1",
		CandidateReady: metav1.ConditionUnknown,
		Now:            now,
	})
	require.Equal(t, "r1", out.Status.StableRevision)
	require.Equal(t, "r2", out.Status.CandidateRevision)
	require.Equal(t, int32(0), out.Status.StageIndex)
	require.Nil(t, out.Status.StageStartTime)
	require.Equal(t, nowTime(), out.Status.CandidateSince)
	require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, out.Status.Phase)
	require.Equal(t, stableOnly("r1"), out.Status.Traffic)
	require.Equal(t, DefaultProgressDeadline, out.RequeueAfter)
}

func TestStep_CandidateReadyStartsStage(t *testing.T) {
	out := Step(Input{
		Strategy:       canary(true),
		Previous:       progressing(0, nil, 0),
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, out.Status.Phase)
	require.Equal(t, int32(0), out.Status.StageIndex)
	require.True(t, out.Status.CandidateReadyOnce)
	require.Nil(t, out.Status.StageStartTime)
	require.Equal(t, split(90, 10), out.Status.Traffic)
	require.Equal(t, splitConfirmationRequeue, out.RequeueAfter)
	require.Contains(t, out.Status.Message, "waiting for Knative to apply 10% traffic split")
}

func TestStep_StageTimerWaitsForAppliedTraffic(t *testing.T) {
	prev := awaitingSplit(0, 10)

	unconfirmed := stepWithApplied(prev, nil)
	require.Nil(t, unconfirmed.Status.StageStartTime)
	require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, unconfirmed.Status.Phase)
	require.Equal(t, int32(0), unconfirmed.Status.StageIndex)
	require.Equal(t, splitConfirmationRequeue, unconfirmed.RequeueAfter)

	confirmed := stepWithApplied(prev, split(90, 10))
	require.Equal(t, nowTime(), confirmed.Status.StageStartTime)
	require.Equal(t, split(90, 10), confirmed.Status.Traffic)
	require.Equal(t, 5*time.Minute, confirmed.RequeueAfter)
	require.Empty(t, confirmed.Events)
}

func TestStep_StageTimerIgnoresMismatchedTraffic(t *testing.T) {
	prev := awaitingSplit(0, 10)
	tests := []struct {
		name    string
		applied []cappv1alpha1.RevisionTraffic
	}{
		{name: "previous split", applied: split(100, 0)},
		{name: "candidate missing counts as zero", applied: stableOnly("r1")},
		{name: "candidate percent differs", applied: split(50, 50)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := stepWithApplied(prev, tc.applied)
			require.Nil(t, out.Status.StageStartTime)
			require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, out.Status.Phase)
			require.Equal(t, splitConfirmationRequeue, out.RequeueAfter)
		})
	}
}

func TestStep_ZeroPercentSplitConfirmedWithoutCandidate(t *testing.T) {
	strategy := &cappv1alpha1.RolloutStrategy{Stages: []cappv1alpha1.RolloutStage{{Percent: 0}}}
	out := Step(Input{
		Strategy:       strategy,
		Previous:       awaitingSplit(0, 0),
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		AppliedTraffic: stableOnly("r1"),
		Now:            now,
	})
	require.Equal(t, nowTime(), out.Status.StageStartTime)
	require.Equal(t, cappv1alpha1.RolloutPhaseHolding, out.Status.Phase)
	require.Zero(t, out.RequeueAfter)
}

func TestStep_StageNotElapsedRequeuesRemaining(t *testing.T) {
	prev := progressing(0, ago(2*time.Minute), 10)
	out := Step(Input{
		Strategy:       canary(true),
		Previous:       prev,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	require.Equal(t, int32(0), out.Status.StageIndex)
	require.Equal(t, prev.StageStartTime, out.Status.StageStartTime)
	require.Equal(t, split(90, 10), out.Status.Traffic)
	require.Equal(t, 3*time.Minute, out.RequeueAfter)
	require.Empty(t, out.Events)
}

func TestStep_ElapsedStageAdvancesOneStage(t *testing.T) {
	out := stepWithApplied(progressing(0, ago(20*time.Minute), 10), split(90, 10))
	require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, out.Status.Phase)
	require.Equal(t, int32(1), out.Status.StageIndex)
	require.Nil(t, out.Status.StageStartTime)
	require.Equal(t, split(50, 50), out.Status.Traffic)
	require.Equal(t, splitConfirmationRequeue, out.RequeueAfter)

	// The next stage's timer starts once Knative serves its split.
	confirmed := stepWithApplied(out.Status, split(50, 50))
	require.Equal(t, int32(1), confirmed.Status.StageIndex)
	require.Equal(t, nowTime(), confirmed.Status.StageStartTime)
	require.Equal(t, 10*time.Minute, confirmed.RequeueAfter)
}

func TestStep_FinalHoldBelowHundred(t *testing.T) {
	strategy := &cappv1alpha1.RolloutStrategy{
		Stages: []cappv1alpha1.RolloutStage{
			{Percent: 10, Duration: dur(5 * time.Minute)},
			{Percent: 50},
		},
	}
	stepped := Step(Input{
		Strategy:       strategy,
		Previous:       progressing(0, ago(6*time.Minute), 10),
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, stepped.Status.Phase)
	require.Equal(t, int32(1), stepped.Status.StageIndex)
	require.Nil(t, stepped.Status.StageStartTime)
	require.Equal(t, split(50, 50), stepped.Status.Traffic)

	out := Step(Input{
		Strategy:       strategy,
		Previous:       stepped.Status,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		AppliedTraffic: split(50, 50),
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseHolding, out.Status.Phase)
	require.Equal(t, int32(1), out.Status.StageIndex)
	require.Equal(t, split(50, 50), out.Status.Traffic)
	require.Zero(t, out.RequeueAfter)

	// Holding stays put on the next step.
	again := Step(Input{
		Strategy:       strategy,
		Previous:       out.Status,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Now:            now.Add(time.Hour),
	})
	require.Equal(t, out.Status, again.Status)
	require.Zero(t, again.RequeueAfter)
	require.Empty(t, again.Events)
}

func TestStep_NewRevisionDuringRolloutRestarts(t *testing.T) {
	out := Step(Input{
		Strategy:       canary(true),
		Previous:       progressing(1, ago(time.Minute), 50),
		LatestCreated:  "r3",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionUnknown,
		Now:            now,
	})
	require.Equal(t, "r1", out.Status.StableRevision)
	require.Equal(t, "r3", out.Status.CandidateRevision)
	require.Equal(t, int32(0), out.Status.StageIndex)
	require.Nil(t, out.Status.StageStartTime)
	require.Equal(t, nowTime(), out.Status.CandidateSince)
	require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, out.Status.Phase)
	require.Equal(t, stableOnly("r1"), out.Status.Traffic)
	require.NotEmpty(t, out.Status.Message)
	e := requireEvent(t, out.Events, corev1.EventTypeWarning, ReasonRestarted)
	require.Contains(t, e.Message, "r2")
	require.Contains(t, e.Message, "r3")
}

func TestStep_DeadlineAutoRollback(t *testing.T) {
	prev := progressing(0, nil, 0)
	prev.CandidateSince = ago(11 * time.Minute)
	out := Step(Input{
		Strategy:       canary(true),
		Previous:       prev,
		LatestCreated:  "r2",
		LatestReady:    "r1",
		CandidateReady: metav1.ConditionUnknown,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseRolledBack, out.Status.Phase)
	require.Equal(t, "r1", out.Status.StableRevision)
	require.Equal(t, "r2", out.Status.CandidateRevision)
	require.Equal(t, stableOnly("r1"), out.Status.Traffic)
	require.NotEmpty(t, out.Status.Message)
	require.Zero(t, out.RequeueAfter)
	requireEvent(t, out.Events, corev1.EventTypeWarning, ReasonRolledBack)
}

func TestStep_ReadyFalseAutoRollback(t *testing.T) {
	out := Step(Input{
		Strategy:       canary(true),
		Previous:       progressing(1, ago(time.Minute), 50),
		LatestCreated:  "r2",
		LatestReady:    "r1",
		CandidateReady: metav1.ConditionFalse,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseRolledBack, out.Status.Phase)
	require.Equal(t, stableOnly("r1"), out.Status.Traffic)
	require.NotEmpty(t, out.Status.Message)
	require.Zero(t, out.RequeueAfter)
	requireEvent(t, out.Events, corev1.EventTypeWarning, ReasonRolledBack)
}

func TestStep_DeadlineNoAutoRollback(t *testing.T) {
	prev := progressing(0, nil, 0)
	prev.CandidateSince = ago(11 * time.Minute)
	out := Step(Input{
		Strategy:       canary(false),
		Previous:       prev,
		LatestCreated:  "r2",
		LatestReady:    "r1",
		CandidateReady: metav1.ConditionUnknown,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseFailed, out.Status.Phase)
	require.Equal(t, stableOnly("r1"), out.Status.Traffic)
	require.Equal(t, "r2", out.Status.CandidateRevision)
	require.Contains(t, out.Status.Message, "abort the rollout, or wait for the candidate to become Ready to promote")
	require.Zero(t, out.RequeueAfter)
	requireEvent(t, out.Events, corev1.EventTypeWarning, ReasonFailed)

	// A failed rollout does not repeat the event.
	again := Step(Input{
		Strategy:       canary(false),
		Previous:       out.Status,
		LatestCreated:  "r2",
		LatestReady:    "r1",
		CandidateReady: metav1.ConditionUnknown,
		Now:            now.Add(time.Minute),
	})
	require.Equal(t, out.Status, again.Status)
	require.Empty(t, again.Events)
}

func TestStep_FailedCandidateRecovers(t *testing.T) {
	prev := progressing(1, ago(time.Minute), 50)
	prev.Phase = cappv1alpha1.RolloutPhaseFailed
	prev.Message = "failed"
	out := Step(Input{
		Strategy:       canary(false),
		Previous:       prev,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, out.Status.Phase)
	require.Equal(t, int32(1), out.Status.StageIndex)
	require.Nil(t, out.Status.StageStartTime)
	require.Equal(t, split(50, 50), out.Status.Traffic)
	require.Equal(t, splitConfirmationRequeue, out.RequeueAfter)

	confirmed := stepWithApplied(out.Status, split(50, 50))
	require.Equal(t, nowTime(), confirmed.Status.StageStartTime)
	require.Equal(t, 10*time.Minute, confirmed.RequeueAfter)
}

func rolledBack(phase cappv1alpha1.RolloutPhase) cappv1alpha1.RolloutStatus {
	return cappv1alpha1.RolloutStatus{
		StableRevision:    "r1",
		CandidateRevision: "r2",
		Phase:             phase,
		StageIndex:        1,
		StageStartTime:    ago(5 * time.Minute),
		CandidateSince:    ago(30 * time.Minute),
		Traffic:           stableOnly("r1"),
		Message:           rolledBackMessage,
	}
}

func TestStep_RolledBackDoesNotRetrySameRevision(t *testing.T) {
	prev := rolledBack(cappv1alpha1.RolloutPhaseRolledBack)
	out := Step(Input{
		Strategy:       canary(true),
		Previous:       prev,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	require.Equal(t, prev, out.Status)
	require.Empty(t, out.Events)
	require.Zero(t, out.RequeueAfter)
}

func TestStep_AbortedDoesNotRetrySameRevision(t *testing.T) {
	prev := rolledBack(cappv1alpha1.RolloutPhaseAborted)
	out := Step(Input{
		Strategy:       canary(true),
		Previous:       prev,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	require.Equal(t, prev, out.Status)
	require.Empty(t, out.Events)
	require.Zero(t, out.RequeueAfter)

	// Only a new revision starts a fresh rollout.
	fresh := Step(Input{
		Strategy:       canary(true),
		Previous:       prev,
		LatestCreated:  "r3",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionUnknown,
		Now:            now,
	})
	require.Equal(t, "r3", fresh.Status.CandidateRevision)
	require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, fresh.Status.Phase)
	require.Equal(t, stableOnly("r1"), fresh.Status.Traffic)
}

func TestStep_PromoteRejected(t *testing.T) {
	notReady := progressing(0, nil, 0)
	notReady.CandidateSince = ago(time.Minute)
	notReady.Traffic = stableOnly("r1")

	tests := []struct {
		name  string
		prev  cappv1alpha1.RolloutStatus
		ready metav1.ConditionStatus
	}{
		{name: "candidate not ready", prev: notReady, ready: metav1.ConditionUnknown},
		{name: rolledBackMessage, prev: rolledBack(cappv1alpha1.RolloutPhaseRolledBack), ready: metav1.ConditionTrue},
		{name: "aborted", prev: rolledBack(cappv1alpha1.RolloutPhaseAborted), ready: metav1.ConditionTrue},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := Step(Input{
				Strategy:       canary(true),
				Previous:       tc.prev,
				LatestCreated:  "r2",
				LatestReady:    "r1",
				CandidateReady: tc.ready,
				Action:         ActionPromote,
				ActionRevision: "r2",
				Now:            now,
			})
			require.Equal(t, tc.prev, out.Status)
			require.Len(t, out.Events, 1)
			requireEvent(t, out.Events, corev1.EventTypeWarning, ReasonPromoteRejected)
		})
	}
}

func TestStep_Abort(t *testing.T) {
	out := Step(Input{
		Strategy:       canary(true),
		Previous:       progressing(1, ago(time.Minute), 50),
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Action:         ActionAbort,
		ActionRevision: "r2",
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseAborted, out.Status.Phase)
	require.Equal(t, "r1", out.Status.StableRevision)
	require.Equal(t, "r2", out.Status.CandidateRevision)
	require.Equal(t, stableOnly("r1"), out.Status.Traffic)
	require.NotEmpty(t, out.Status.Message)
	require.Zero(t, out.RequeueAfter)
	requireEvent(t, out.Events, corev1.EventTypeNormal, ReasonAborted)
}

func TestParseAction(t *testing.T) {
	tests := []struct {
		value        string
		want         Action
		wantRevision string
		wantOK       bool
	}{
		{value: "promote:r2", want: ActionPromote, wantRevision: "r2", wantOK: true},
		{value: "abort:r2", want: ActionAbort, wantRevision: "r2", wantOK: true},
		{value: "", want: ActionNone, wantOK: true},
		{value: "promote", want: ActionNone, wantOK: false},
		{value: cappmeta.RolloutActionAbort, want: ActionNone, wantOK: false},
		{value: "promote:", want: ActionNone, wantOK: false},
		{value: "promot:r2", want: ActionNone, wantOK: false},
		{value: "retry:r2", want: ActionRetry, wantRevision: "r2", wantOK: true},
		{value: cappmeta.RolloutActionRetry, want: ActionNone, wantOK: false},
		{value: "retry:", want: ActionNone, wantOK: false},
	}
	for _, tc := range tests {
		got, revision, ok := ParseAction(tc.value)
		require.Equal(t, tc.want, got, tc.value)
		require.Equal(t, tc.wantRevision, revision, tc.value)
		require.Equal(t, tc.wantOK, ok, tc.value)
	}
}

func TestStep_StaleActionRejected(t *testing.T) {
	prev := progressing(1, ago(time.Minute), 50)
	tests := []struct {
		name   string
		action Action
		reason string
	}{
		{name: "promote", action: ActionPromote, reason: ReasonPromoteRejected},
		{name: cappmeta.RolloutActionAbort, action: ActionAbort, reason: ReasonAbortRejected},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := Step(Input{
				Strategy:       canary(true),
				Previous:       prev,
				LatestCreated:  "r2",
				LatestReady:    "r2",
				CandidateReady: metav1.ConditionTrue,
				Action:         tc.action,
				ActionRevision: "r3",
				Now:            now,
			})
			require.Equal(t, prev, out.Status)
			require.Len(t, out.Events, 1)
			e := requireEvent(t, out.Events, corev1.EventTypeWarning, tc.reason)
			require.Contains(t, e.Message, "r3")
			require.Contains(t, e.Message, "r2")
		})
	}
}

func TestStep_CandidateSinceFromCreation(t *testing.T) {
	created := now.Add(-3 * time.Minute)
	out := Step(Input{
		Strategy:         canary(true),
		Previous:         cappv1alpha1.RolloutStatus{StableRevision: "r1", Phase: cappv1alpha1.RolloutPhaseStable},
		LatestCreated:    "r2",
		LatestReady:      "r1",
		CandidateReady:   metav1.ConditionUnknown,
		CandidateCreated: created,
		Now:              now,
	})
	require.Equal(t, metav1.NewTime(created), *out.Status.CandidateSince)
	require.False(t, out.Status.CandidateSinceFallback)
	require.Equal(t, DefaultProgressDeadline-3*time.Minute, out.RequeueAfter)
}

func TestStep_DeadlineCountsFromCreation(t *testing.T) {
	// The candidate was created before the operator restarted and is already past its deadline.
	out := Step(Input{
		Strategy:         canary(true),
		Previous:         cappv1alpha1.RolloutStatus{StableRevision: "r1", Phase: cappv1alpha1.RolloutPhaseStable},
		LatestCreated:    "r2",
		LatestReady:      "r1",
		CandidateReady:   metav1.ConditionUnknown,
		CandidateCreated: now.Add(-11 * time.Minute),
		Now:              now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseRolledBack, out.Status.Phase)
	require.Equal(t, metav1.NewTime(now.Add(-11*time.Minute)), *out.Status.CandidateSince)
	requireEvent(t, out.Events, corev1.EventTypeWarning, ReasonRolledBack)
}

func TestStep_CandidateSinceReplacedByCreation(t *testing.T) {
	// CandidateSince was set from the time the operator first saw the candidate; the Revision is older.
	prev := progressing(0, nil, 0)
	prev.CandidateSince = nowTime()
	prev.CandidateSinceFallback = true
	created := now.Add(-11 * time.Minute)
	out := Step(Input{
		Strategy:         canary(true),
		Previous:         prev,
		LatestCreated:    "r2",
		LatestReady:      "r1",
		CandidateReady:   metav1.ConditionUnknown,
		CandidateCreated: created,
		Now:              now,
	})
	require.Equal(t, metav1.NewTime(created), *out.Status.CandidateSince)
	require.Equal(t, cappv1alpha1.RolloutPhaseRolledBack, out.Status.Phase)
}

func TestStep_CandidateSinceFallsBackToNow(t *testing.T) {
	out := Step(Input{
		Strategy:       canary(true),
		Previous:       cappv1alpha1.RolloutStatus{StableRevision: "r1", Phase: cappv1alpha1.RolloutPhaseStable},
		LatestCreated:  "r2",
		LatestReady:    "r1",
		CandidateReady: metav1.ConditionUnknown,
		Now:            now,
	})
	require.Equal(t, nowTime(), out.Status.CandidateSince)
	require.True(t, out.Status.CandidateSinceFallback)
	require.Equal(t, DefaultProgressDeadline, out.RequeueAfter)
}

func TestStep_DeadlineOnlyAppliesBeforeFirstReady(t *testing.T) {
	prev := progressing(1, ago(time.Minute), 50)
	out := Step(Input{
		Strategy:       canary(true),
		Previous:       prev,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionUnknown,
		Now:            now,
	})
	require.Equal(t, prev, out.Status)
	require.Empty(t, out.Events)
	require.Zero(t, out.RequeueAfter)
}

func heldAt() cappv1alpha1.RolloutStatus {
	status := awaitingSplit(0, 10)
	status.Phase = cappv1alpha1.RolloutPhaseHolding
	status.HeldPercent = ptr.To(int64(10))
	return status
}

// holdingAtFinal returns a status of a rollout that holds at the final stage of canary, below 100%.
func holdingAtFinal() cappv1alpha1.RolloutStatus {
	status := progressing(1, ago(time.Minute), 50)
	status.Phase = cappv1alpha1.RolloutPhaseHolding
	return status
}

func promotingOf(prev cappv1alpha1.RolloutStatus) cappv1alpha1.RolloutStatus {
	return Step(Input{
		Strategy:       canary(true),
		Previous:       prev,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Action:         ActionPromote,
		ActionRevision: "r2",
		Now:            now,
	}).Status
}

func TestStep_FinalHundredPromotes(t *testing.T) {
	out := Step(Input{
		Strategy:       canary(true),
		Previous:       progressing(1, ago(11*time.Minute), 50),
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	require.Equal(t, "r1", out.Status.StableRevision)
	require.Equal(t, "r2", out.Status.CandidateRevision)
	require.Equal(t, int32(2), out.Status.StageIndex)
	require.Equal(t, cappv1alpha1.RolloutPhasePromoting, out.Status.Phase)
	require.Equal(t, split(0, 100), out.Status.Traffic)
	require.Equal(t, splitConfirmationRequeue, out.RequeueAfter)
	require.Empty(t, out.Events)

	// The candidate becomes stable once Knative serves it all.
	confirmed := stepWithApplied(out.Status, split(0, 100))
	require.Equal(t, "r2", confirmed.Status.StableRevision)
	require.Empty(t, confirmed.Status.CandidateRevision)
	require.Nil(t, confirmed.Status.StageStartTime)
	require.Nil(t, confirmed.Status.CandidateSince)
	require.Equal(t, int32(0), confirmed.Status.StageIndex)
	require.Equal(t, stableOnly("r2"), confirmed.Status.Traffic)
	require.Equal(t, cappv1alpha1.RolloutPhaseStable, confirmed.Status.Phase)
	require.Zero(t, confirmed.RequeueAfter)
	requireEvent(t, confirmed.Events, corev1.EventTypeNormal, ReasonCompleted)
}

func TestStep_PromoteReady(t *testing.T) {
	promoted := Step(Input{
		Strategy:       canary(true),
		Previous:       progressing(0, ago(time.Minute), 10),
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Action:         ActionPromote,
		ActionRevision: "r2",
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhasePromoting, promoted.Status.Phase)
	require.Equal(t, "r1", promoted.Status.StableRevision)
	require.Equal(t, "r2", promoted.Status.CandidateRevision)
	require.Equal(t, split(0, 100), promoted.Status.Traffic)
	require.Equal(t, splitConfirmationRequeue, promoted.RequeueAfter)
	require.Empty(t, promoted.Events)
}

func TestStep_PromotingWaitsForAppliedTraffic(t *testing.T) {
	promoting := promotingOf(awaitingSplit(0, 10))

	waiting := stepWithApplied(promoting, split(90, 10))
	require.Equal(t, cappv1alpha1.RolloutPhasePromoting, waiting.Status.Phase)
	require.Equal(t, "r1", waiting.Status.StableRevision)
	require.Equal(t, "r2", waiting.Status.CandidateRevision)
	require.Equal(t, splitConfirmationRequeue, waiting.RequeueAfter)
	require.Empty(t, waiting.Events)

	done := stepWithApplied(promoting, split(0, 100))
	require.Equal(t, cappv1alpha1.RolloutPhaseStable, done.Status.Phase)
	require.Equal(t, "r2", done.Status.StableRevision)
	require.Empty(t, done.Status.CandidateRevision)
	requireEvent(t, done.Events, corev1.EventTypeNormal, ReasonCompleted)
}

func TestStep_PromotingRollsBackWhenCandidateUnhealthy(t *testing.T) {
	promoting := promotingOf(awaitingSplit(0, 10))
	out := Step(Input{
		Strategy:       canary(true),
		Previous:       promoting,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionFalse,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseRolledBack, out.Status.Phase)
	require.Equal(t, stableOnly("r1"), out.Status.Traffic)
	require.Zero(t, out.RequeueAfter)
	requireEvent(t, out.Events, corev1.EventTypeWarning, ReasonRolledBack)
}

func TestStep_StrategyRemovedPromotes(t *testing.T) {
	out := Step(Input{
		Previous:       progressing(1, ago(time.Minute), 50),
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	require.Equal(t, "r1", out.Status.StableRevision)
	require.Equal(t, "r2", out.Status.CandidateRevision)
	require.Equal(t, cappv1alpha1.RolloutPhasePromoting, out.Status.Phase)
	require.Equal(t, defaultRouteTraffic("r2"), out.Status.Traffic)
	require.Equal(t, splitConfirmationRequeue, out.RequeueAfter)
	require.Empty(t, out.Events)

	// Without a strategy, Knative's default routing sends everything to the candidate.
	applied := []cappv1alpha1.RevisionTraffic{{RevisionName: "r2", Percent: 100}}
	confirmed := Step(Input{
		Previous:       out.Status,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		AppliedTraffic: applied,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutStatus{
		StableRevision: "r2",
		Phase:          cappv1alpha1.RolloutPhaseStable,
		Message:        confirmed.Status.Message,
	}, confirmed.Status)
	require.Zero(t, confirmed.RequeueAfter)
	requireEvent(t, confirmed.Events, corev1.EventTypeNormal, ReasonCompleted)
}

func TestStep_StrategyRemovedPinsRejectedCandidate(t *testing.T) {
	tests := []struct {
		name      string
		prev      cappv1alpha1.RolloutStatus
		ready     metav1.ConditionStatus
		wantPhase cappv1alpha1.RolloutPhase
	}{
		{name: rolledBackMessage, prev: rolledBack(cappv1alpha1.RolloutPhaseRolledBack), ready: metav1.ConditionUnknown, wantPhase: cappv1alpha1.RolloutPhaseRolledBack},
		{name: "aborted", prev: rolledBack(cappv1alpha1.RolloutPhaseAborted), ready: metav1.ConditionTrue, wantPhase: cappv1alpha1.RolloutPhaseAborted},
		{name: "failed", prev: rolledBack(cappv1alpha1.RolloutPhaseFailed), ready: metav1.ConditionTrue, wantPhase: cappv1alpha1.RolloutPhaseFailed},
		{name: "progressing candidate not ready", prev: progressing(0, nil, 0), ready: metav1.ConditionUnknown, wantPhase: cappv1alpha1.RolloutPhaseFailed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := Step(Input{
				Previous:       tc.prev,
				LatestCreated:  "r2",
				LatestReady:    "r1",
				CandidateReady: tc.ready,
				Now:            now,
			})
			require.Equal(t, "r1", out.Status.StableRevision)
			require.Equal(t, "r1", out.Status.PinnedRevision)
			require.Equal(t, "r2", out.Status.CandidateRevision)
			// The ksvc is pinned to the stable revision, so the status shows that traffic.
			require.Equal(t, stableOnly("r1"), out.Status.Traffic)
			require.Equal(t, tc.wantPhase, out.Status.Phase)
			require.Zero(t, out.RequeueAfter)
			requireEvent(t, out.Events, corev1.EventTypeWarning, ReasonPinned)
		})
	}
}

func TestStep_PinHeldUntilNewerRevisionReady(t *testing.T) {
	pinned := Step(Input{
		Previous:       rolledBack(cappv1alpha1.RolloutPhaseRolledBack),
		LatestCreated:  "r2",
		LatestReady:    "r1",
		CandidateReady: metav1.ConditionUnknown,
		Now:            now,
	}).Status
	require.Equal(t, "r1", pinned.PinnedRevision)

	// rev3 is created but not Ready. LatestReady is still the rejected rev2, which must not be used.
	created := Step(Input{
		Previous:       pinned,
		LatestCreated:  "r3",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionUnknown,
		Now:            now,
	})
	require.Equal(t, pinned, created.Status)
	require.Empty(t, created.Events)

	// The rejected revision being Ready again does not release the pin either.
	rejectedReady := Step(Input{
		Previous:       pinned,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	require.Equal(t, pinned, rejectedReady.Status)

	// Once the newer revision is Ready, the Capp goes back to the default routing.
	released := Step(Input{
		Previous:       pinned,
		LatestCreated:  "r3",
		LatestReady:    "r3",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutStatus{StableRevision: "r3", Phase: cappv1alpha1.RolloutPhaseStable}, released.Status)
	require.Empty(t, released.Events)
}

func TestStep_StrategyAddedClearsPin(t *testing.T) {
	pinned := Step(Input{
		Previous:       rolledBack(cappv1alpha1.RolloutPhaseRolledBack),
		LatestCreated:  "r2",
		LatestReady:    "r1",
		CandidateReady: metav1.ConditionUnknown,
		Now:            now,
	}).Status

	out := Step(Input{
		Strategy:       canary(true),
		Previous:       pinned,
		LatestCreated:  "r2",
		LatestReady:    "r1",
		CandidateReady: metav1.ConditionUnknown,
		Now:            now,
	})
	require.Empty(t, out.Status.PinnedRevision)
	require.Equal(t, "r1", out.Status.StableRevision)
	require.Equal(t, cappv1alpha1.RolloutPhaseRolledBack, out.Status.Phase)
}

func TestStep_StrategyChangeMovesToLowerStage(t *testing.T) {
	// The 50% stage is gone. The candidate serves 50%, so the rollout moves to the 10% stage, never to 100%.
	out := Step(Input{
		Strategy:       &cappv1alpha1.RolloutStrategy{Stages: []cappv1alpha1.RolloutStage{{Percent: 10, Duration: dur(5 * time.Minute)}, {Percent: 100}}},
		Previous:       progressing(1, ago(time.Minute), 50),
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		ServedTraffic:  split(50, 50),
		Now:            now,
	})
	require.Equal(t, int32(0), out.Status.StageIndex)
	require.Nil(t, out.Status.StageStartTime)
	require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, out.Status.Phase)
	require.Equal(t, split(90, 10), out.Status.Traffic)
	require.Equal(t, splitConfirmationRequeue, out.RequeueAfter)
	requireEvent(t, out.Events, corev1.EventTypeNormal, ReasonStagesChanged)
}

func TestStep_StrategyChangeKeepsStageForDurationChange(t *testing.T) {
	prev := progressing(0, ago(2*time.Minute), 10)
	out := Step(Input{
		Strategy: &cappv1alpha1.RolloutStrategy{Stages: []cappv1alpha1.RolloutStage{
			{Percent: 10, Duration: dur(10 * time.Minute)},
			{Percent: 50, Duration: dur(10 * time.Minute)},
			{Percent: 100},
		}},
		Previous:       prev,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	require.Equal(t, int32(0), out.Status.StageIndex)
	require.Equal(t, prev.StageStartTime, out.Status.StageStartTime)
	require.Equal(t, split(90, 10), out.Status.Traffic)
	require.Equal(t, 8*time.Minute, out.RequeueAfter)
	require.Empty(t, out.Events)
}

func TestStep_StrategyChangeKeepsStageWhenPercentUnchanged(t *testing.T) {
	prev := progressing(1, ago(time.Minute), 50)
	out := Step(Input{
		Strategy: &cappv1alpha1.RolloutStrategy{Stages: []cappv1alpha1.RolloutStage{
			{Percent: 10, Duration: dur(5 * time.Minute)},
			{Percent: 50, Duration: dur(10 * time.Minute)},
		}},
		Previous:       prev,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	require.Equal(t, int32(1), out.Status.StageIndex)
	require.Equal(t, prev.StageStartTime, out.Status.StageStartTime)
	require.Equal(t, split(50, 50), out.Status.Traffic)
	require.Equal(t, 9*time.Minute, out.RequeueAfter)
	require.Empty(t, out.Events)
}

func TestStep_StrategyChangeHoldsWhenNoStageIsBelowServed(t *testing.T) {
	tests := []struct {
		name   string
		stages []cappv1alpha1.RolloutStage
	}{
		{name: "every stage above", stages: []cappv1alpha1.RolloutStage{{Percent: 50, Duration: dur(5 * time.Minute)}, {Percent: 100}}},
		{name: "only final stage", stages: []cappv1alpha1.RolloutStage{{Percent: 100}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			strategy := &cappv1alpha1.RolloutStrategy{Stages: tc.stages}
			out := Step(Input{
				Strategy:       strategy,
				Previous:       progressing(0, ago(time.Minute), 10),
				LatestCreated:  "r2",
				LatestReady:    "r2",
				CandidateReady: metav1.ConditionTrue,
				ServedTraffic:  split(90, 10),
				Now:            now,
			})
			require.Equal(t, cappv1alpha1.RolloutPhaseHolding, out.Status.Phase)
			require.Equal(t, ptr.To(int64(10)), out.Status.HeldPercent)
			require.Equal(t, split(90, 10), out.Status.Traffic)
			require.Zero(t, out.RequeueAfter)
			requireEvent(t, out.Events, corev1.EventTypeWarning, ReasonStagesChanged)

			// The hold is stable on later steps and does not repeat the event.
			again := Step(Input{
				Strategy:       strategy,
				Previous:       out.Status,
				LatestCreated:  "r2",
				LatestReady:    "r2",
				CandidateReady: metav1.ConditionTrue,
				Now:            now.Add(time.Hour),
			})
			require.Equal(t, out.Status, again.Status)
			require.Empty(t, again.Events)
		})
	}
}

func TestStep_StrategyEditHoldsAtServedShareWhenRequestedIsPending(t *testing.T) {
	// Knative still serves 10% while the requested 50% is pending. The edit must not raise the traffic to 40%.
	out := Step(Input{
		Strategy:       &cappv1alpha1.RolloutStrategy{Stages: []cappv1alpha1.RolloutStage{{Percent: 40, Duration: dur(10 * time.Minute)}, {Percent: 100}}},
		Previous:       progressing(1, ago(time.Minute), 50),
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		ServedTraffic:  split(90, 10),
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseHolding, out.Status.Phase)
	require.Equal(t, ptr.To(int64(10)), out.Status.HeldPercent)
	require.Equal(t, split(90, 10), out.Status.Traffic)
	requireEvent(t, out.Events, corev1.EventTypeWarning, ReasonStagesChanged)
}

func TestStep_StrategyEditServesLowerOfRequestedAndReported(t *testing.T) {
	// The requested split is 10% while Knative reports 50%. The edit uses 10%, so the hold is at 10%, not 50%.
	out := Step(Input{
		Strategy:       &cappv1alpha1.RolloutStrategy{Stages: []cappv1alpha1.RolloutStage{{Percent: 20, Duration: dur(5 * time.Minute)}, {Percent: 100}}},
		Previous:       progressing(0, ago(time.Minute), 10),
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		ServedTraffic:  split(50, 50),
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseHolding, out.Status.Phase)
	require.Equal(t, ptr.To(int64(10)), out.Status.HeldPercent)
	require.Equal(t, split(90, 10), out.Status.Traffic)
}

func TestStep_PendingAdvanceWithUnchangedStrategyIsNotPulledBack(t *testing.T) {
	// The candidate advanced to 50% and Knative still serves 10%. The strategy is unchanged, so the rollout waits
	// for the split instead of moving back to the 10% stage.
	prev := awaitingSplit(1, 50)
	out := Step(Input{
		Strategy:       canary(true),
		Previous:       prev,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		ServedTraffic:  split(90, 10),
		Now:            now,
	})
	require.Equal(t, int32(1), out.Status.StageIndex)
	require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, out.Status.Phase)
	require.Equal(t, prev.Traffic, out.Status.Traffic)
	require.Empty(t, out.Events)
}

func TestStep_HeldRolloutMovesToStageBelowHold(t *testing.T) {
	out := Step(Input{
		Strategy:       &cappv1alpha1.RolloutStrategy{Stages: []cappv1alpha1.RolloutStage{{Percent: 5, Duration: dur(5 * time.Minute)}, {Percent: 100}}},
		Previous:       heldAt(),
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	require.Nil(t, out.Status.HeldPercent)
	require.Equal(t, int32(0), out.Status.StageIndex)
	require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, out.Status.Phase)
	require.Equal(t, split(95, 5), out.Status.Traffic)
	requireEvent(t, out.Events, corev1.EventTypeNormal, ReasonStagesChanged)
}

func TestStep_ActionsClearHeldPercent(t *testing.T) {
	heldFailed := heldAt()
	heldFailed.Phase = cappv1alpha1.RolloutPhaseFailed
	tests := []struct {
		name      string
		prev      cappv1alpha1.RolloutStatus
		action    Action
		ready     metav1.ConditionStatus
		wantPhase cappv1alpha1.RolloutPhase
	}{
		{name: cappmeta.RolloutActionAbort, prev: heldAt(), action: ActionAbort, ready: metav1.ConditionTrue, wantPhase: cappv1alpha1.RolloutPhaseAborted},
		{name: cappmeta.RolloutActionRetry, prev: heldFailed, action: ActionRetry, ready: metav1.ConditionUnknown, wantPhase: cappv1alpha1.RolloutPhaseProgressing},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := Step(Input{
				Strategy:       canary(true),
				Previous:       tc.prev,
				LatestCreated:  "r2",
				LatestReady:    "r2",
				CandidateReady: tc.ready,
				Action:         tc.action,
				ActionRevision: "r2",
				Now:            now,
			})
			require.Equal(t, tc.wantPhase, out.Status.Phase)
			require.Nil(t, out.Status.HeldPercent)
		})
	}
}

func TestStep_NewRevisionClearsHold(t *testing.T) {
	out := Step(Input{
		Strategy:       canary(true),
		Previous:       heldAt(),
		LatestCreated:  "r3",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionUnknown,
		Now:            now,
	})
	require.Equal(t, "r3", out.Status.CandidateRevision)
	require.Nil(t, out.Status.HeldPercent)
	require.Equal(t, stableOnly("r1"), out.Status.Traffic)
}

func TestStep_StrategyShrinkOutOfRangeRemaps(t *testing.T) {
	strategy := &cappv1alpha1.RolloutStrategy{
		Stages: []cappv1alpha1.RolloutStage{
			{Percent: 10, Duration: dur(5 * time.Minute)},
			{Percent: 50, Duration: dur(10 * time.Minute)},
		},
	}
	out := Step(Input{
		Strategy:       strategy,
		Previous:       progressing(3, ago(time.Minute), 80),
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		ServedTraffic:  split(20, 80),
		Now:            now,
	})
	require.Equal(t, int32(1), out.Status.StageIndex)
	require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, out.Status.Phase)
	require.Equal(t, split(50, 50), out.Status.Traffic)
	require.Equal(t, splitConfirmationRequeue, out.RequeueAfter)
	requireEvent(t, out.Events, corev1.EventTypeNormal, ReasonStagesChanged)
}

func TestStep_ReadyOnceCandidateNotDeadlined(t *testing.T) {
	// The candidate was Ready once and its split is not confirmed. Going Unknown after the deadline is not a failure.
	prev := awaitingSplit(0, 10)
	prev.CandidateSince = ago(11 * time.Minute)
	out := Step(Input{
		Strategy:       canary(true),
		Previous:       prev,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionUnknown,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, out.Status.Phase)
	require.Empty(t, out.Events)
	require.Zero(t, out.RequeueAfter)
}

func TestStep_FailedClearsStageTimer(t *testing.T) {
	out := Step(Input{
		Strategy:       canary(false),
		Previous:       progressing(1, ago(time.Minute), 50),
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionFalse,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseFailed, out.Status.Phase)
	require.Nil(t, out.Status.StageStartTime)
	require.Equal(t, int32(1), out.Status.StageIndex)
	require.Equal(t, split(50, 50), out.Status.Traffic)
	require.Zero(t, out.RequeueAfter)
	requireEvent(t, out.Events, corev1.EventTypeWarning, ReasonFailed)

	// Recovery resumes the same stage, and its timer restarts once Knative confirms the split.
	recovered := Step(Input{
		Strategy:       canary(false),
		Previous:       out.Status,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, recovered.Status.Phase)
	require.Equal(t, int32(1), recovered.Status.StageIndex)
	require.Nil(t, recovered.Status.StageStartTime)
	require.Equal(t, split(50, 50), recovered.Status.Traffic)
	require.Equal(t, splitConfirmationRequeue, recovered.RequeueAfter)
}

func TestStep_UnreadyCandidateOmittedFromTraffic(t *testing.T) {
	// rev2 serves 50%. rev3 is created and not Ready, so rev1 serves everything and rev2 gets no traffic.
	out := Step(Input{
		Strategy:       canary(true),
		Previous:       progressing(1, ago(time.Minute), 50),
		LatestCreated:  "r3",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionUnknown,
		Now:            now,
	})
	require.Equal(t, "r3", out.Status.CandidateRevision)
	require.Equal(t, stableOnly("r1"), out.Status.Traffic)
}

func TestStep_RetryRestartsRollout(t *testing.T) {
	out := Step(Input{
		Strategy:       canary(true),
		Previous:       rolledBack(cappv1alpha1.RolloutPhaseRolledBack),
		LatestCreated:  "r2",
		LatestReady:    "r1",
		CandidateReady: metav1.ConditionUnknown,
		Action:         ActionRetry,
		ActionRevision: "r2",
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, out.Status.Phase)
	require.Equal(t, int32(0), out.Status.StageIndex)
	require.Nil(t, out.Status.StageStartTime)
	require.False(t, out.Status.CandidateReadyOnce)
	require.Equal(t, nowTime(), out.Status.CandidateSince)
	require.Equal(t, stableOnly("r1"), out.Status.Traffic)
	require.Equal(t, DefaultProgressDeadline, out.RequeueAfter)
	requireEvent(t, out.Events, corev1.EventTypeNormal, ReasonRetried)

	// The retried candidate is the same revision, not a new one, so its deadline is not reset again.
	again := Step(Input{
		Strategy:       canary(true),
		Previous:       out.Status,
		LatestCreated:  "r2",
		LatestReady:    "r1",
		CandidateReady: metav1.ConditionUnknown,
		Now:            now.Add(time.Minute),
	})
	require.Equal(t, "r2", again.Status.CandidateRevision)
	require.Equal(t, nowTime(), again.Status.CandidateSince)
	require.Empty(t, again.Events)
}

func TestStep_RetryAllowedPhases(t *testing.T) {
	for _, phase := range []cappv1alpha1.RolloutPhase{
		cappv1alpha1.RolloutPhaseFailed,
		cappv1alpha1.RolloutPhaseRolledBack,
		cappv1alpha1.RolloutPhaseAborted,
	} {
		t.Run(string(phase), func(t *testing.T) {
			out := Step(Input{
				Strategy:       canary(true),
				Previous:       rolledBack(phase),
				LatestCreated:  "r2",
				LatestReady:    "r1",
				CandidateReady: metav1.ConditionUnknown,
				Action:         ActionRetry,
				ActionRevision: "r2",
				Now:            now,
			})
			require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, out.Status.Phase)
			requireEvent(t, out.Events, corev1.EventTypeNormal, ReasonRetried)
		})
	}
}

func TestStep_RetryRejected(t *testing.T) {
	tests := []struct {
		name     string
		prev     cappv1alpha1.RolloutStatus
		revision string
	}{
		{name: "progressing", prev: progressing(1, ago(time.Minute), 50), revision: "r2"},
		{name: "holding", prev: holdingAtFinal(), revision: "r2"},
		{name: "stale revision", prev: rolledBack(cappv1alpha1.RolloutPhaseRolledBack), revision: "r3"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := Input{
				Strategy:       canary(true),
				Previous:       tc.prev,
				LatestCreated:  "r2",
				LatestReady:    "r2",
				CandidateReady: metav1.ConditionTrue,
				Now:            now,
			}
			baseline := Step(input)
			input.Action = ActionRetry
			input.ActionRevision = tc.revision
			out := Step(input)
			require.Equal(t, baseline.Status, out.Status)
			requireEvent(t, out.Events, corev1.EventTypeWarning, "RolloutRetryRejected")
		})
	}
}

func TestStep_RollbackClearsHold(t *testing.T) {
	out := Step(Input{
		Strategy:       canary(true),
		Previous:       heldAt(),
		LatestCreated:  "r2",
		LatestReady:    "r1",
		CandidateReady: metav1.ConditionFalse,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseRolledBack, out.Status.Phase)
	require.Nil(t, out.Status.HeldPercent)
	require.Equal(t, stableOnly("r1"), out.Status.Traffic)
}

func TestStep_RetryKeepsDeadlineAfterRetry(t *testing.T) {
	// r2 was created 2h ago and rolled back. The retry starts a new deadline, which the creation time must not overwrite.
	created := now.Add(-2 * time.Hour)
	retried := Step(Input{
		Strategy:         canary(true),
		Previous:         rolledBack(cappv1alpha1.RolloutPhaseRolledBack),
		LatestCreated:    "r2",
		LatestReady:      "r1",
		CandidateReady:   metav1.ConditionUnknown,
		CandidateCreated: created,
		Action:           ActionRetry,
		ActionRevision:   "r2",
		Now:              now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, retried.Status.Phase)
	require.Equal(t, nowTime(), retried.Status.CandidateSince)

	// Ten seconds later the candidate is still not Ready. It has a full deadline from the retry.
	out := Step(Input{
		Strategy:         canary(true),
		Previous:         retried.Status,
		LatestCreated:    "r2",
		LatestReady:      "r1",
		CandidateReady:   metav1.ConditionUnknown,
		CandidateCreated: created,
		Now:              now.Add(10 * time.Second),
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, out.Status.Phase)
	require.Equal(t, nowTime(), out.Status.CandidateSince)
	require.Empty(t, out.Events)
	require.Equal(t, DefaultProgressDeadline-10*time.Second, out.RequeueAfter)

	// The deadline still applies, counted from the retry.
	expired := Step(Input{
		Strategy:         canary(true),
		Previous:         out.Status,
		LatestCreated:    "r2",
		LatestReady:      "r1",
		CandidateReady:   metav1.ConditionUnknown,
		CandidateCreated: created,
		Now:              now.Add(DefaultProgressDeadline),
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseRolledBack, expired.Status.Phase)
	requireEvent(t, expired.Events, corev1.EventTypeWarning, ReasonRolledBack)
}

func TestStep_StaleActionDoesNotSuppressRemap(t *testing.T) {
	t.Run("stale promote", func(t *testing.T) {
		out := Step(Input{
			Strategy:       &cappv1alpha1.RolloutStrategy{Stages: []cappv1alpha1.RolloutStage{{Percent: 10, Duration: dur(5 * time.Minute)}, {Percent: 100}}},
			Previous:       progressing(1, ago(time.Minute), 50),
			LatestCreated:  "r2",
			LatestReady:    "r2",
			CandidateReady: metav1.ConditionTrue,
			AppliedTraffic: split(50, 50),
			ServedTraffic:  split(50, 50),
			Action:         ActionPromote,
			ActionRevision: "r9",
			Now:            now,
		})
		require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, out.Status.Phase)
		require.Equal(t, int32(0), out.Status.StageIndex)
		require.Equal(t, split(90, 10), out.Status.Traffic)
		requireEvent(t, out.Events, corev1.EventTypeWarning, ReasonPromoteRejected)
	})

	t.Run("rejected retry with stage index out of range", func(t *testing.T) {
		out := Step(Input{
			Strategy:       &cappv1alpha1.RolloutStrategy{Stages: []cappv1alpha1.RolloutStage{{Percent: 50}}},
			Previous:       progressing(2, ago(time.Minute), 50),
			LatestCreated:  "r2",
			LatestReady:    "r2",
			CandidateReady: metav1.ConditionTrue,
			AppliedTraffic: split(50, 50),
			ServedTraffic:  split(50, 50),
			Action:         ActionRetry,
			ActionRevision: "r2",
			Now:            now,
		})
		require.Equal(t, int32(0), out.Status.StageIndex)
		require.Equal(t, cappv1alpha1.RolloutPhaseHolding, out.Status.Phase)
		require.Equal(t, split(50, 50), out.Status.Traffic)
		requireEvent(t, out.Events, corev1.EventTypeWarning, ReasonRetryRejected)
	})
}

func TestStep_HoldAtStageWithoutDurationRestartsTimer(t *testing.T) {
	holding := progressing(1, ago(time.Hour), 50)
	holding.Phase = cappv1alpha1.RolloutPhaseHolding
	strategy := &cappv1alpha1.RolloutStrategy{Stages: []cappv1alpha1.RolloutStage{
		{Percent: 10, Duration: dur(5 * time.Minute)},
		{Percent: 50, Duration: dur(5 * time.Minute)},
		{Percent: 100},
	}}

	t.Run("split confirmed", func(t *testing.T) {
		out := Step(Input{
			Strategy:       strategy,
			Previous:       holding,
			LatestCreated:  "r2",
			LatestReady:    "r2",
			CandidateReady: metav1.ConditionTrue,
			AppliedTraffic: split(50, 50),
			Now:            now,
		})
		require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, out.Status.Phase)
		require.Equal(t, int32(1), out.Status.StageIndex)
		require.Equal(t, nowTime(), out.Status.StageStartTime)
		require.Equal(t, 5*time.Minute, out.RequeueAfter)
	})

	t.Run("split not confirmed", func(t *testing.T) {
		out := Step(Input{
			Strategy:       strategy,
			Previous:       holding,
			LatestCreated:  "r2",
			LatestReady:    "r2",
			CandidateReady: metav1.ConditionTrue,
			Now:            now,
		})
		require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, out.Status.Phase)
		require.Equal(t, int32(1), out.Status.StageIndex)
		require.Nil(t, out.Status.StageStartTime)
		require.Equal(t, splitConfirmationRequeue, out.RequeueAfter)
	})
}

func TestStep_RetryAcceptedWhileHeldByStrategyChange(t *testing.T) {
	strategy := &cappv1alpha1.RolloutStrategy{Stages: []cappv1alpha1.RolloutStage{
		{Percent: 50, Duration: dur(10 * time.Minute)},
		{Percent: 100},
	}}
	applied := split(90, 10)
	held := Step(Input{
		Strategy:       strategy,
		Previous:       progressing(0, ago(time.Minute), 10),
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		AppliedTraffic: applied,
		ServedTraffic:  applied,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseHolding, held.Status.Phase)
	require.Equal(t, ptr.To(int64(10)), held.Status.HeldPercent)

	out := Step(Input{
		Strategy:       strategy,
		Previous:       held.Status,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		AppliedTraffic: applied,
		ServedTraffic:  applied,
		Action:         ActionRetry,
		ActionRevision: "r2",
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseProgressing, out.Status.Phase)
	require.Nil(t, out.Status.HeldPercent)
	require.Equal(t, int32(0), out.Status.StageIndex)
	requireEvent(t, out.Events, corev1.EventTypeNormal, ReasonRetried)
	for _, e := range out.Events {
		require.NotEqual(t, ReasonRetryRejected, e.Reason)
	}
}

func TestStep_StrategyRemovedNeverPromotesOldCandidate(t *testing.T) {
	t.Run("newer revision ready", func(t *testing.T) {
		// r2 was never Ready. r3 is created and Ready in the same apply that removes the strategy.
		out := Step(Input{
			Previous:       progressing(0, nil, 0),
			LatestCreated:  "r3",
			LatestReady:    "r3",
			CandidateReady: metav1.ConditionTrue,
			Now:            now,
		})
		require.Equal(t, cappv1alpha1.RolloutPhaseStable, out.Status.Phase)
		require.Equal(t, "r3", out.Status.StableRevision)
		require.Empty(t, out.Status.CandidateRevision)
		require.Empty(t, out.Status.PinnedRevision)
		require.Empty(t, out.Events)
	})

	t.Run("newer revision created not ready", func(t *testing.T) {
		// r2 is Ready and rolling out. r3 is only created, so r2 must not be promoted.
		out := Step(Input{
			Previous:       progressing(1, ago(time.Minute), 50),
			LatestCreated:  "r3",
			LatestReady:    "r2",
			CandidateReady: metav1.ConditionUnknown,
			Now:            now,
		})
		require.NotEqual(t, cappv1alpha1.RolloutPhasePromoting, out.Status.Phase)
		require.Equal(t, "r1", out.Status.PinnedRevision)
		requireEvent(t, out.Events, corev1.EventTypeWarning, ReasonPinned)
	})
}

func TestStep_StrategyRemovedKeepsCandidateSince(t *testing.T) {
	prev := rolledBack(cappv1alpha1.RolloutPhaseFailed)
	out := Step(Input{
		Previous:       prev,
		LatestCreated:  "r2",
		LatestReady:    "r1",
		CandidateReady: metav1.ConditionUnknown,
		Now:            now,
	})
	require.Equal(t, "r1", out.Status.PinnedRevision)
	require.Equal(t, prev.CandidateSince, out.Status.CandidateSince)

	// Adopting the strategy again keeps the deadline of the failed candidate, which has been failing for 30 minutes.
	readopted := Step(Input{
		Strategy:       canary(true),
		Previous:       out.Status,
		LatestCreated:  "r2",
		LatestReady:    "r1",
		CandidateReady: metav1.ConditionUnknown,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseRolledBack, readopted.Status.Phase)
	requireEvent(t, readopted.Events, corev1.EventTypeWarning, ReasonRolledBack)
}

func TestStep_FailedPromotionReturnsTrafficToStageSplit(t *testing.T) {
	failing := Input{
		Strategy:       canary(false),
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionFalse,
		Now:            now,
	}

	t.Run("manual promote", func(t *testing.T) {
		promoting := Step(Input{
			Strategy:       canary(false),
			Previous:       progressing(1, ago(time.Minute), 50),
			LatestCreated:  "r2",
			LatestReady:    "r2",
			CandidateReady: metav1.ConditionTrue,
			AppliedTraffic: split(50, 50),
			Action:         ActionPromote,
			ActionRevision: "r2",
			Now:            now,
		})
		require.Equal(t, cappv1alpha1.RolloutPhasePromoting, promoting.Status.Phase)

		failing.Previous = promoting.Status
		out := Step(failing)
		require.Equal(t, cappv1alpha1.RolloutPhaseFailed, out.Status.Phase)
		require.Equal(t, int32(1), out.Status.StageIndex)
		require.Equal(t, split(50, 50), out.Status.Traffic)
		require.Zero(t, out.RequeueAfter)
		requireEvent(t, out.Events, corev1.EventTypeWarning, ReasonFailed)
	})

	t.Run("manual promote from hold", func(t *testing.T) {
		promoting := Step(Input{
			Strategy:       canary(false),
			Previous:       heldAt(),
			LatestCreated:  "r2",
			LatestReady:    "r2",
			CandidateReady: metav1.ConditionTrue,
			AppliedTraffic: split(90, 10),
			Action:         ActionPromote,
			ActionRevision: "r2",
			Now:            now,
		})
		require.Equal(t, cappv1alpha1.RolloutPhasePromoting, promoting.Status.Phase)

		failing.Previous = promoting.Status
		out := Step(failing)
		require.Equal(t, cappv1alpha1.RolloutPhaseFailed, out.Status.Phase)
		require.Equal(t, split(90, 10), out.Status.Traffic)
	})

	t.Run("final 100% stage", func(t *testing.T) {
		// The candidate reaches the final stage after its 50% stage elapsed, and is promoted.
		promoting := Step(Input{
			Strategy:       canary(false),
			Previous:       progressing(1, ago(11*time.Minute), 50),
			LatestCreated:  "r2",
			LatestReady:    "r2",
			CandidateReady: metav1.ConditionTrue,
			AppliedTraffic: split(50, 50),
			Now:            now,
		})
		require.Equal(t, cappv1alpha1.RolloutPhasePromoting, promoting.Status.Phase)
		require.Equal(t, int32(2), promoting.Status.StageIndex)

		failing.Previous = promoting.Status
		out := Step(failing)
		require.Equal(t, cappv1alpha1.RolloutPhaseFailed, out.Status.Phase)
		require.Equal(t, int32(1), out.Status.StageIndex)
		require.Equal(t, split(50, 50), out.Status.Traffic)
		requireEvent(t, out.Events, corev1.EventTypeWarning, ReasonFailed)
	})

	t.Run("only final stage", func(t *testing.T) {
		strategy := &cappv1alpha1.RolloutStrategy{Stages: []cappv1alpha1.RolloutStage{{Percent: 100}}}
		promoting := Step(Input{
			Strategy:       strategy,
			Previous:       cappv1alpha1.RolloutStatus{StableRevision: "r1", Phase: cappv1alpha1.RolloutPhaseStable},
			LatestCreated:  "r2",
			LatestReady:    "r2",
			CandidateReady: metav1.ConditionTrue,
			Now:            now,
		})
		require.Equal(t, cappv1alpha1.RolloutPhasePromoting, promoting.Status.Phase)

		out := Step(Input{
			Strategy:       &cappv1alpha1.RolloutStrategy{Stages: strategy.Stages},
			Previous:       promoting.Status,
			LatestCreated:  "r2",
			LatestReady:    "r2",
			CandidateReady: metav1.ConditionFalse,
			Now:            now,
		})
		require.Equal(t, cappv1alpha1.RolloutPhaseFailed, out.Status.Phase)
		require.Equal(t, stableOnly("r1"), out.Status.Traffic)
		requireEvent(t, out.Events, corev1.EventTypeWarning, ReasonFailed)
	})
}

func TestStep_SingleStageFailedPromotionRecoversAndPromotes(t *testing.T) {
	strategy := &cappv1alpha1.RolloutStrategy{Stages: []cappv1alpha1.RolloutStage{{Percent: 100}}}
	promoting := Step(Input{
		Strategy:       strategy,
		Previous:       cappv1alpha1.RolloutStatus{StableRevision: "r1", Phase: cappv1alpha1.RolloutPhaseStable},
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhasePromoting, promoting.Status.Phase)

	failed := Step(Input{
		Strategy:       strategy,
		Previous:       promoting.Status,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionFalse,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseFailed, failed.Status.Phase)
	require.Equal(t, stableOnly("r1"), failed.Status.Traffic)

	recovered := Step(Input{
		Strategy:       strategy,
		Previous:       failed.Status,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhasePromoting, recovered.Status.Phase)
	require.Equal(t, int32(0), recovered.Status.StageIndex)
	require.Equal(t, split(0, 100), recovered.Status.Traffic)
	require.Equal(t, splitConfirmationRequeue, recovered.RequeueAfter)

	done := Step(Input{
		Strategy:       strategy,
		Previous:       recovered.Status,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		AppliedTraffic: split(0, 100),
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseStable, done.Status.Phase)
	require.Equal(t, "r2", done.Status.StableRevision)
	requireEvent(t, done.Events, corev1.EventTypeNormal, ReasonCompleted)
}

func TestStep_StrategyChangeHoldSurvivesRecovery(t *testing.T) {
	// A candidate held at 0% by a strategy change must not be promoted when it recovers from a failure.
	zeroStage := &cappv1alpha1.RolloutStrategy{Stages: []cappv1alpha1.RolloutStage{{Percent: 0}}}
	singleFinal := &cappv1alpha1.RolloutStrategy{Stages: []cappv1alpha1.RolloutStage{{Percent: 100}}}
	atZero := split(100, 0)

	held := Step(Input{
		Strategy:       zeroStage,
		Previous:       cappv1alpha1.RolloutStatus{StableRevision: "r1", Phase: cappv1alpha1.RolloutPhaseStable},
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		AppliedTraffic: atZero,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseHolding, held.Status.Phase)
	require.Nil(t, held.Status.HeldPercent)

	edited := Step(Input{
		Strategy:       singleFinal,
		Previous:       held.Status,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		AppliedTraffic: atZero,
		ServedTraffic:  atZero,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseHolding, edited.Status.Phase)
	require.Equal(t, ptr.To(int64(0)), edited.Status.HeldPercent)
	require.Equal(t, int32(0), edited.Status.StageIndex)
	requireEvent(t, edited.Events, corev1.EventTypeWarning, ReasonStagesChanged)

	failed := Step(Input{
		Strategy:       singleFinal,
		Previous:       edited.Status,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionFalse,
		AppliedTraffic: atZero,
		ServedTraffic:  atZero,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseFailed, failed.Status.Phase)

	recovered := Step(Input{
		Strategy:       singleFinal,
		Previous:       failed.Status,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		AppliedTraffic: atZero,
		ServedTraffic:  atZero,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseHolding, recovered.Status.Phase)
	require.Equal(t, ptr.To(int64(0)), recovered.Status.HeldPercent)
	require.False(t, recovered.Status.PromotionFallback)
	require.Equal(t, atZero, recovered.Status.Traffic)

	// Later steps keep the hold: the candidate is not promoted without a user action.
	again := Step(Input{
		Strategy:       singleFinal,
		Previous:       recovered.Status,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		AppliedTraffic: atZero,
		ServedTraffic:  atZero,
		Now:            now.Add(time.Hour),
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseHolding, again.Status.Phase)
	require.Equal(t, ptr.To(int64(0)), again.Status.HeldPercent)
}

func TestStep_SingleStageFallbackStrategyEditDoesNotAutoPromote(t *testing.T) {
	single := &cappv1alpha1.RolloutStrategy{Stages: []cappv1alpha1.RolloutStage{{Percent: 100}}}
	edited := &cappv1alpha1.RolloutStrategy{Stages: []cappv1alpha1.RolloutStage{{Percent: 50, Duration: dur(10 * time.Minute)}, {Percent: 100}}}
	promoting := Step(Input{
		Strategy:       single,
		Previous:       cappv1alpha1.RolloutStatus{StableRevision: "r1", Phase: cappv1alpha1.RolloutPhaseStable},
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	failed := Step(Input{
		Strategy:       single,
		Previous:       promoting.Status,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionFalse,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseFailed, failed.Status.Phase)
	require.Equal(t, ptr.To(int64(0)), failed.Status.HeldPercent)
	require.True(t, failed.Status.PromotionFallback)

	// The strategy is edited while the candidate is failed. The 0% fallback hold stays.
	editedFailed := Step(Input{
		Strategy:       edited,
		Previous:       failed.Status,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionFalse,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseFailed, editedFailed.Status.Phase)
	require.Equal(t, ptr.To(int64(0)), editedFailed.Status.HeldPercent)

	recovered := Step(Input{
		Strategy:       edited,
		Previous:       editedFailed.Status,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseHolding, recovered.Status.Phase)
	require.Equal(t, ptr.To(int64(0)), recovered.Status.HeldPercent)
	require.False(t, recovered.Status.PromotionFallback)
	require.Equal(t, split(100, 0), recovered.Status.Traffic)
	// The recovery message says the rollout resumed, but the rollout is held, so the message must describe the hold.
	require.Equal(t, "Candidate revision r2 holds at 0% of traffic because no stage is at or below it; promote, abort or retry the rollout",
		recovered.Status.Message)
}

func TestStep_HeldRolloutKeepsMessageThatDescribesTheHold(t *testing.T) {
	const message = "Strategy changed; candidate revision r2 holds at 0% of traffic because no stage is at or below it; promote, abort or retry the rollout"
	edited := &cappv1alpha1.RolloutStrategy{Stages: []cappv1alpha1.RolloutStage{{Percent: 50, Duration: dur(10 * time.Minute)}, {Percent: 100}}}
	held := cappv1alpha1.RolloutStatus{
		StableRevision:     "r1",
		CandidateRevision:  "r2",
		Phase:              cappv1alpha1.RolloutPhaseHolding,
		CandidateReadyOnce: true,
		HeldPercent:        ptr.To(int64(0)),
		Traffic:            split(100, 0),
		Message:            message,
	}

	out := Step(Input{
		Strategy:       edited,
		Previous:       held,
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhaseHolding, out.Status.Phase)
	require.Equal(t, message, out.Status.Message)
}

func TestStep_LeavingHoldClearsPromotionFallback(t *testing.T) {
	fallback := cappv1alpha1.RolloutStatus{
		StableRevision:     "r1",
		CandidateRevision:  "r2",
		Phase:              cappv1alpha1.RolloutPhaseFailed,
		CandidateReadyOnce: true,
		CandidateSince:     ago(30 * time.Minute),
		HeldPercent:        ptr.To(int64(0)),
		PromotionFallback:  true,
		Traffic:            stableOnly("r1"),
	}
	tests := []struct {
		name  string
		in    Input
		phase cappv1alpha1.RolloutPhase
	}{
		{
			name:  cappmeta.RolloutActionAbort,
			in:    Input{LatestCreated: "r2", LatestReady: "r2", Action: ActionAbort, ActionRevision: "r2", CandidateReady: metav1.ConditionTrue},
			phase: cappv1alpha1.RolloutPhaseAborted,
		},
		{
			name:  cappmeta.RolloutActionRetry,
			in:    Input{LatestCreated: "r2", LatestReady: "r2", Action: ActionRetry, ActionRevision: "r2", CandidateReady: metav1.ConditionTrue},
			phase: cappv1alpha1.RolloutPhaseProgressing,
		},
		{
			name:  "new revision",
			in:    Input{LatestCreated: "r3", LatestReady: "r2", CandidateReady: metav1.ConditionUnknown},
			phase: cappv1alpha1.RolloutPhaseProgressing,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in
			in.Strategy = canary(true)
			in.Previous = fallback
			in.Now = now
			out := Step(in)
			require.Equal(t, tc.phase, out.Status.Phase)
			require.Nil(t, out.Status.HeldPercent)
			require.False(t, out.Status.PromotionFallback)
		})
	}
}

func TestStep_RetryRejectedUsesRolloutPrefix(t *testing.T) {
	out := Step(Input{
		Strategy:       canary(true),
		Previous:       progressing(1, ago(time.Minute), 50),
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Action:         ActionRetry,
		ActionRevision: "r2",
		Now:            now,
	})
	requireEvent(t, out.Events, corev1.EventTypeWarning, "RolloutRetryRejected")
}

func TestStep_NoStrategyRejectsActions(t *testing.T) {
	tests := []struct {
		name string
		prev cappv1alpha1.RolloutStatus
	}{
		{name: "stable"},
		{name: "pinned", prev: cappv1alpha1.RolloutStatus{StableRevision: "r1", PinnedRevision: "r1", Phase: cappv1alpha1.RolloutPhaseFailed}},
	}
	for _, tc := range tests {
		for _, action := range []Action{ActionPromote, ActionAbort, ActionRetry} {
			t.Run(tc.name+" "+string(action), func(t *testing.T) {
				out := Step(Input{
					Previous:       tc.prev,
					LatestCreated:  "r2",
					LatestReady:    "r1",
					Action:         action,
					ActionRevision: "r2",
					Now:            now,
				})
				require.Len(t, out.Events, 1)
				event := requireEvent(t, out.Events, corev1.EventTypeWarning, ReasonInvalidAction)
				require.Contains(t, event.Message, "no rollout strategy")
			})
		}
	}
}

func TestStep_ActionRejectedWithoutRevision(t *testing.T) {
	tests := []struct {
		name        string
		in          Input
		wantMessage string
	}{
		{
			name:        "no Knative Service",
			in:          Input{Strategy: canary(true)},
			wantMessage: "the Knative Service has no created revision yet",
		},
		{
			name:        "no stable revision yet",
			in:          Input{Strategy: canary(true), LatestCreated: "r1"},
			wantMessage: "no stable revision yet",
		},
	}
	for _, tt := range tests {
		for _, action := range []Action{ActionPromote, ActionAbort, ActionRetry} {
			t.Run(tt.name+" "+string(action), func(t *testing.T) {
				in := tt.in
				in.Action = action
				in.ActionRevision = "r1"
				in.Now = now

				out := Step(in)
				require.Empty(t, out.Status.StableRevision)
				require.Empty(t, out.Status.CandidateRevision)
				require.Len(t, out.Events, 1)
				event := requireEvent(t, out.Events, corev1.EventTypeWarning, ReasonInvalidAction)
				require.Equal(t, fmt.Sprintf("Rollout action %s:r1 rejected: %s", action, tt.wantMessage), event.Message)
			})
		}
	}
}

func TestStep_NoActionNoRejectionWithoutRevision(t *testing.T) {
	require.Empty(t, Step(Input{Strategy: canary(true), Now: now}).Events)
	require.Empty(t, Step(Input{Strategy: canary(true), LatestCreated: "r1", Now: now}).Events)
}

func TestStep_ReplayedActionIsNoOp(t *testing.T) {
	tests := []struct {
		name     string
		in       Input
		action   Action
		revision string
	}{
		{
			name:     "promote of the stable revision",
			in:       Input{Strategy: canary(true), Previous: stableStatus("r1"), LatestCreated: "r1", LatestReady: "r1", Now: now},
			action:   ActionPromote,
			revision: "r1",
		},
		{
			name: "promote of the stable revision during a newer rollout",
			in: Input{
				Strategy:       canary(true),
				Previous:       progressing(1, ago(time.Minute), 50),
				LatestCreated:  "r2",
				LatestReady:    "r2",
				CandidateReady: metav1.ConditionTrue,
				Now:            now,
			},
			action:   ActionPromote,
			revision: "r1",
		},
		{
			name: "abort of an aborted rollout",
			in: Input{
				Strategy:       canary(true),
				Previous:       rolledBack(cappv1alpha1.RolloutPhaseAborted),
				LatestCreated:  "r2",
				LatestReady:    "r1",
				CandidateReady: metav1.ConditionTrue,
				Now:            now,
			},
			action:   ActionAbort,
			revision: "r2",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			without := Step(tc.in)

			replay := tc.in
			replay.Action = tc.action
			replay.ActionRevision = tc.revision
			out := Step(replay)

			require.Equal(t, without.Status, out.Status)
			require.Equal(t, without.RequeueAfter, out.RequeueAfter)
			require.Empty(t, out.Events)
		})
	}
}

func TestStep_StrategyRemovedPromotesToDefaultRoute(t *testing.T) {
	out := Step(Input{
		Previous:       progressing(1, ago(time.Minute), 50),
		LatestCreated:  "r2",
		LatestReady:    "r2",
		CandidateReady: metav1.ConditionTrue,
		Now:            now,
	})
	require.Equal(t, cappv1alpha1.RolloutPhasePromoting, out.Status.Phase)
	// The ksvc has no traffic targets once the strategy is removed, so Knative routes all traffic to the latest
	// Ready revision. The status shows that route rather than the split the operator asks for.
	require.Equal(t, []cappv1alpha1.RevisionTraffic{{RevisionName: "r2", Percent: 100}}, out.Status.Traffic)
}
