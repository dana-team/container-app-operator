// Package rollout holds the state machine of Capp staged rollouts. Step is pure: it derives the
// next rollout status, traffic and events from the previous status and observed revisions.
package rollout

import (
	"fmt"
	"maps"
	"strings"
	"time"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Action is a user action on a rollout, read from the cappmeta.RolloutAnnotationKey annotation.
type Action string

const (
	ActionNone    Action = ""
	ActionPromote Action = cappmeta.RolloutActionPromote
	ActionAbort   Action = cappmeta.RolloutActionAbort
	ActionRetry   Action = cappmeta.RolloutActionRetry
)

// Traffic tags of the stable and candidate revisions.
const (
	TagStable    = "stable"
	TagCandidate = "candidate"
)

// Event reasons emitted by Step.
const (
	ReasonCompleted       = "RolloutCompleted"
	ReasonRestarted       = "RolloutRestarted"
	ReasonRolledBack      = "RolloutRolledBack"
	ReasonFailed          = "RolloutFailed"
	ReasonAborted         = "RolloutAborted"
	ReasonPromoteRejected = "RolloutPromoteRejected"
	ReasonAbortRejected   = "RolloutAbortRejected"
	ReasonRetried         = "RolloutRetried"
	ReasonRetryRejected   = "RolloutRetryRejected"
	ReasonPinned          = "RolloutPinned"
	ReasonStagesChanged   = "RolloutStagesChanged"
	ReasonInvalidAction   = "RolloutInvalidAction"
)

// DefaultProgressDeadline is how long a candidate may take to become ready when the strategy sets no deadline.
const DefaultProgressDeadline = 10 * time.Minute

// splitConfirmationRequeue is how long to wait for Knative to confirm a stage's traffic split before checking again.
// The Knative Service status watch normally triggers the reconcile sooner; this is a safety net.
const splitConfirmationRequeue = 30 * time.Second

// ParseAction parses the value of the rollout annotation, promote:<revision>, abort:<revision> or retry:<revision>,
// and returns the action and the revision it targets. An empty value means no action. ok is false for any other value.
func ParseAction(v string) (Action, string, bool) {
	if v == "" {
		return ActionNone, "", true
	}
	name, revision, found := strings.Cut(v, ":")
	if !found || revision == "" {
		return ActionNone, "", false
	}
	switch a := Action(name); a {
	case ActionPromote, ActionAbort, ActionRetry:
		return a, revision, true
	}
	return ActionNone, "", false
}

// Input is everything Step needs to compute the next rollout state.
type Input struct {
	Strategy         *cappv1alpha1.RolloutStrategy
	Previous         cappv1alpha1.RolloutStatus     // zero value when Capp has none
	LatestCreated    string                         // ksvc.Status.LatestCreatedRevisionName; "" when no ksvc
	LatestReady      string                         // ksvc.Status.LatestReadyRevisionName
	CandidateReady   metav1.ConditionStatus         // Ready of the LatestCreated Revision object
	CandidateCreated time.Time                      // creationTimestamp of the LatestCreated Revision; zero when unknown
	AppliedTraffic   []cappv1alpha1.RevisionTraffic // traffic Knative confirms is serving; nil when not confirmed
	ServedTraffic    []cappv1alpha1.RevisionTraffic // ksvc.Status.Traffic as last reported, unfiltered; unlike AppliedTraffic, not nil while a change is pending
	Action           Action
	ActionRevision   string // revision the Action targets
	Now              time.Time
}

// Event is a Kubernetes event to record on the Capp.
type Event struct{ Type, Reason, Message string } // Type is corev1.EventTypeNormal/Warning

// Output is the result of Step.
type Output struct {
	Status       cappv1alpha1.RolloutStatus
	RequeueAfter time.Duration // 0 = no timed requeue
	Events       []Event
}

// stepper holds the state of a single Step evaluation.
type stepper struct {
	in       Input
	stages   []cappv1alpha1.RolloutStage
	deadline time.Duration
	status   cappv1alpha1.RolloutStatus
	events   []Event
}

// Step computes the next rollout status, the delay until the next timed step and the events to emit.
func Step(in Input) Output {
	if in.LatestCreated == "" {
		return Output{Events: rejectedAction(in, "the Knative Service has no created revision yet")}
	}
	if in.Strategy == nil || len(in.Strategy.Stages) == 0 {
		return stepWithoutStrategy(in)
	}

	s := &stepper{
		in:       in,
		stages:   in.Strategy.Stages,
		deadline: DefaultProgressDeadline,
		status:   *in.Previous.DeepCopy(),
	}
	if in.Strategy.ProgressDeadline != nil {
		s.deadline = in.Strategy.ProgressDeadline.Duration
	}
	// A pin only holds while the strategy is removed. A strategy takes the rollout over again.
	s.status.PinnedRevision = ""

	if s.status.StableRevision == "" && s.status.CandidateRevision == "" {
		if in.LatestReady == "" {
			return Output{Events: rejectedAction(in, "no stable revision yet")}
		}
		s.status.StableRevision = in.LatestReady
		s.status.Phase = cappv1alpha1.RolloutPhaseStable
	}

	s.detectNewRevision()
	s.remapStage()
	s.keepStageIndexInRange()

	if done := s.handleAction(); !done {
		if healthy := s.checkHealth(); healthy {
			s.advance()
		}
	}

	s.status.Traffic = s.traffic()
	return Output{Status: s.status, RequeueAfter: s.requeueAfter(), Events: s.events}
}

// stepWithoutStrategy tracks the revision Knative serves as stable and rejects any action, since there is no rollout to
// act on. A rollout left without a strategy is finished or pinned by stepRemovedRollout.
func stepWithoutStrategy(in Input) Output {
	out := trackWithoutStrategy(in)
	out.Events = append(out.Events, rejectedAction(in, "the Capp has no rollout strategy")...)
	return out
}

// rejectedAction returns the Warning that reports the action of in as rejected for the given reason. It returns nil
// when there is no action. Steps that return before any rollout exists use it, so that an action is never dropped
// silently.
func rejectedAction(in Input, reason string) []Event {
	if in.Action == ActionNone {
		return nil
	}
	return []Event{{
		Type:    corev1.EventTypeWarning,
		Reason:  ReasonInvalidAction,
		Message: fmt.Sprintf("Rollout action %s:%s rejected: %s", in.Action, in.ActionRevision, reason),
	}}
}

// trackWithoutStrategy returns the status of a Capp without a rollout strategy, see stepWithoutStrategy.
func trackWithoutStrategy(in Input) Output {
	prev := in.Previous
	if prev.PinnedRevision != "" && !pinReleased(in, prev) {
		return Output{Status: *prev.DeepCopy()}
	}
	if prev.CandidateRevision != "" && prev.PinnedRevision == "" && !pinReleased(in, prev) {
		return stepRemovedRollout(in)
	}

	status := cappv1alpha1.RolloutStatus{StableRevision: prev.StableRevision}
	if in.LatestReady != "" {
		status.StableRevision = in.LatestReady
	}
	if status.StableRevision != "" {
		status.Phase = cappv1alpha1.RolloutPhaseStable
	}
	return Output{Status: status}
}

// stepRemovedRollout handles a rollout whose strategy was removed. A Ready candidate that was rolling out is promoted,
// and becomes stable once Knative serves it all. Any other candidate is rejected: traffic is pinned to stable until a
// newer revision is Ready, so that Knative's default routing cannot send traffic back to the rejected candidate.
// CandidateReady describes the latest created revision, so a candidate that a newer revision superseded is never promoted.
func stepRemovedRollout(in Input) Output {
	prev := in.Previous
	superseded := in.LatestCreated != prev.CandidateRevision
	if !superseded && isRollingOut(prev.Phase) && in.CandidateReady == metav1.ConditionTrue {
		promoting := cappv1alpha1.RolloutStatus{
			StableRevision:    prev.StableRevision,
			CandidateRevision: prev.CandidateRevision,
			Phase:             cappv1alpha1.RolloutPhasePromoting,
			Traffic:           defaultRouteTraffic(prev.CandidateRevision),
			Message:           fmt.Sprintf("Rollout strategy removed; promoting revision %s to 100%% of traffic", prev.CandidateRevision),
		}
		if !trafficConfirmed(promoting.Traffic, in.AppliedTraffic) {
			return Output{Status: promoting, RequeueAfter: splitConfirmationRequeue}
		}
		return Output{
			Status: stableStatus(prev.CandidateRevision),
			Events: []Event{{
				Type:    corev1.EventTypeNormal,
				Reason:  ReasonCompleted,
				Message: fmt.Sprintf("Rollout strategy removed; revision %s promoted to stable", prev.CandidateRevision),
			}},
		}
	}

	phase := prev.Phase
	if isRollingOut(phase) {
		phase = cappv1alpha1.RolloutPhaseFailed
	}
	message := fmt.Sprintf("Rollout strategy removed; candidate revision %s is not promoted; traffic stays pinned to stable revision %s until a newer revision is ready",
		prev.CandidateRevision, prev.StableRevision)
	return Output{
		Status: cappv1alpha1.RolloutStatus{
			StableRevision:         prev.StableRevision,
			CandidateRevision:      prev.CandidateRevision,
			PinnedRevision:         prev.StableRevision,
			Phase:                  phase,
			CandidateSince:         prev.CandidateSince,
			CandidateSinceFallback: prev.CandidateSinceFallback,
			Traffic:                []cappv1alpha1.RevisionTraffic{{RevisionName: prev.StableRevision, Percent: 100, Tag: TagStable}},
			Message:                message,
		},
		Events: []Event{{Type: corev1.EventTypeWarning, Reason: ReasonPinned, Message: message}},
	}
}

// pinReleased reports whether a newer revision is Ready, so that the pin can be dropped. A revision that is only created
// does not release it, and neither does the rejected candidate or the stable revision.
func pinReleased(in Input, prev cappv1alpha1.RolloutStatus) bool {
	return in.LatestCreated == in.LatestReady &&
		in.LatestCreated != prev.CandidateRevision &&
		in.LatestCreated != prev.StableRevision
}

// isActive reports whether a candidate in this phase is still being rolled out.
func isActive(phase cappv1alpha1.RolloutPhase) bool {
	switch phase {
	case cappv1alpha1.RolloutPhaseProgressing, cappv1alpha1.RolloutPhaseHolding,
		cappv1alpha1.RolloutPhaseFailed, cappv1alpha1.RolloutPhasePromoting:
		return true
	}
	return false
}

// isRollingOut reports whether a candidate in this phase is moving toward stable and can be promoted.
func isRollingOut(phase cappv1alpha1.RolloutPhase) bool {
	switch phase {
	case cappv1alpha1.RolloutPhaseProgressing, cappv1alpha1.RolloutPhaseHolding, cappv1alpha1.RolloutPhasePromoting:
		return true
	}
	return false
}

// retryable reports whether a retry may restart the rollout of the candidate: when it failed, was rolled back or
// aborted, or is held because the strategy changed.
func (s *stepper) retryable() bool {
	switch s.status.Phase {
	case cappv1alpha1.RolloutPhaseFailed, cappv1alpha1.RolloutPhaseRolledBack, cappv1alpha1.RolloutPhaseAborted:
		return true
	case cappv1alpha1.RolloutPhaseHolding:
		return s.status.HeldPercent != nil
	}
	return false
}

func (s *stepper) hasActiveCandidate() bool {
	return s.status.CandidateRevision != "" && isActive(s.status.Phase)
}

func (s *stepper) emit(eventType, reason, message string) {
	s.events = append(s.events, Event{Type: eventType, Reason: reason, Message: message})
}

// detectNewRevision makes a revision that is neither stable nor candidate the new candidate,
// restarting any rollout in progress.
func (s *stepper) detectNewRevision() {
	latest := s.in.LatestCreated
	if latest == s.status.StableRevision {
		return
	}
	if latest == s.status.CandidateRevision {
		s.anchorCandidateSince()
		return
	}

	stable := s.status.StableRevision
	if s.hasActiveCandidate() {
		old := s.status.CandidateRevision
		s.status.Message = fmt.Sprintf("Revision %s replaced candidate %s; rolling out %s from stable revision %s",
			latest, old, latest, stable)
		s.emit(corev1.EventTypeWarning, ReasonRestarted, fmt.Sprintf(
			"New revision %s created during rollout of %s; rollout restarted from stable revision %s", latest, old, stable))
	} else {
		s.status.Message = fmt.Sprintf("Rolling out revision %s from stable revision %s", latest, stable)
	}

	since := metav1.NewTime(s.in.Now)
	if !s.in.CandidateCreated.IsZero() {
		since = metav1.NewTime(s.in.CandidateCreated)
	}
	s.status.CandidateRevision = latest
	s.status.Phase = cappv1alpha1.RolloutPhaseProgressing
	s.status.StageIndex = 0
	s.status.StageStartTime = nil
	s.clearHold()
	s.status.CandidateReadyOnce = false
	s.status.CandidateSince = &since
	s.status.CandidateSinceFallback = s.in.CandidateCreated.IsZero()
}

// anchorCandidateSince moves a fallback candidateSince back to the creation time of the candidate, once the Revision can
// be read. A candidateSince from a retry is not a fallback and is never moved.
func (s *stepper) anchorCandidateSince() {
	created := s.in.CandidateCreated
	if created.IsZero() || !s.status.CandidateSinceFallback {
		return
	}
	since := metav1.NewTime(created)
	s.status.CandidateSince = &since
	s.status.CandidateSinceFallback = false
}

// remapStage moves an active rollout when the strategy changed under it, so that a change never raises the candidate's
// traffic. The share compared with the stages is the lower of the split the rollout requested and the split Knative
// reports, so a split that Knative has not applied yet is not taken as served. The rollout moves to the last stage at
// or below that share. When every stage is above it, the candidate holds at that share until the user promotes,
// aborts or retries.
func (s *stepper) remapStage() {
	if s.status.Phase == cappv1alpha1.RolloutPhasePromoting || !s.hasActiveCandidate() || !s.status.CandidateReadyOnce {
		return
	}

	if s.status.HeldPercent != nil {
		if index := s.stageAtOrBelow(*s.status.HeldPercent); index >= 0 {
			s.clearHold()
			s.moveToStage(index)
		}
		return
	}

	requested := candidateShare(s.in.Previous.Traffic, s.status.CandidateRevision)
	if index := int(s.status.StageIndex); index < len(s.stages) && s.stages[index].Percent == requested {
		stage := s.stages[index]
		// A rollout held at a stage without a duration has no timer to run out. If the edit gave the stage a duration,
		// the timer restarts, so the time already spent holding does not count toward that duration.
		if s.status.Phase == cappv1alpha1.RolloutPhaseHolding && stage.Duration != nil {
			s.status.StageStartTime = nil
		}
		return
	}

	served := min(requested, candidateShare(s.in.ServedTraffic, s.status.CandidateRevision))
	if index := s.stageAtOrBelow(served); index >= 0 {
		s.moveToStage(index)
		return
	}

	held := served
	s.status.HeldPercent = &held
	s.status.StageIndex = 0
	s.status.StageStartTime = nil
	if s.status.Phase != cappv1alpha1.RolloutPhaseFailed {
		s.status.Phase = cappv1alpha1.RolloutPhaseHolding
	}
	s.status.Message = fmt.Sprintf("Strategy changed; candidate revision %s holds at %d%% of traffic because no stage is at or below it; promote, abort or retry the rollout",
		s.status.CandidateRevision, served)
	s.emit(corev1.EventTypeWarning, ReasonStagesChanged, s.status.Message)
}

// keepStageIndexInRange guards the stage index against a strategy that no longer has the stage. A promotion uses no
// stage, so its index moves to the last stage, which a failed promotion falls back from. Otherwise the index moves to
// the first stage, which is where a candidate that never became Ready stays.
func (s *stepper) keepStageIndexInRange() {
	last := len(s.stages) - 1
	if int(s.status.StageIndex) <= last {
		return
	}
	if s.status.Phase == cappv1alpha1.RolloutPhasePromoting {
		s.status.StageIndex = toStageIndex(last)
	} else {
		s.status.StageIndex = 0
	}
	s.status.StageStartTime = nil
}

// toStageIndex converts a position in the stages of a strategy to the int32 StageIndex status field.
func toStageIndex(index int) int32 {
	//nolint:gosec // G115: the stages come from a Capp object, which etcd bounds far below MaxInt32 entries
	return int32(index)
}

// moveToStage starts the stage at index. Its timer starts again once Knative confirms the stage's split.
func (s *stepper) moveToStage(index int) {
	stage := s.stages[index]
	s.status.StageIndex = toStageIndex(index)
	s.status.StageStartTime = nil
	s.status.Message = fmt.Sprintf("Strategy changed; candidate revision %s moves to stage %d of %d at %d%% of traffic",
		s.status.CandidateRevision, index+1, len(s.stages), stage.Percent)
	s.emit(corev1.EventTypeNormal, ReasonStagesChanged, s.status.Message)
}

// stageAtOrBelow returns the index of the last stage whose percent is at most percent, or -1 when there is none.
func (s *stepper) stageAtOrBelow(percent int64) int {
	index := -1
	for i, stage := range s.stages {
		if stage.Percent <= percent {
			index = i
		}
	}
	return index
}

// candidateShare returns the percent of traffic the candidate revision has in a traffic table.
func candidateShare(traffic []cappv1alpha1.RevisionTraffic, candidate string) int64 {
	var percent int64
	for _, t := range traffic {
		if t.RevisionName == candidate {
			percent += t.Percent
		}
	}
	return percent
}

// handleAction applies a promote, abort or retry action to the candidate it names. It returns true when the rollout
// needs no further stepping. An action that was already applied, such as a promote of the stable revision, is a no-op.
func (s *stepper) handleAction() bool {
	switch s.in.Action {
	case ActionPromote:
		if s.in.ActionRevision == s.status.StableRevision {
			return false
		}
		if !s.actionTargetsCandidate(ReasonPromoteRejected, "Promote") {
			return false
		}
		if !s.hasActiveCandidate() || s.in.CandidateReady != metav1.ConditionTrue {
			s.emit(corev1.EventTypeWarning, ReasonPromoteRejected, s.promoteRejectedMessage())
			return false
		}
		// The candidate becomes stable once Knative confirms the split, which advance checks.
		s.startPromotion()
		return false
	case ActionAbort:
		if s.status.Phase == cappv1alpha1.RolloutPhaseAborted && s.in.ActionRevision == s.status.CandidateRevision {
			return false
		}
		if !s.actionTargetsCandidate(ReasonAbortRejected, "Abort") {
			return false
		}
		if !s.hasActiveCandidate() {
			s.emit(corev1.EventTypeWarning, ReasonAbortRejected, "Abort rejected: no rollout in progress")
			return false
		}
		s.status.Phase = cappv1alpha1.RolloutPhaseAborted
		s.clearHold()
		s.status.Message = fmt.Sprintf("Rollout of revision %s aborted; all traffic is on stable revision %s",
			s.status.CandidateRevision, s.status.StableRevision)
		s.emit(corev1.EventTypeNormal, ReasonAborted, s.status.Message)
		return true
	case ActionRetry:
		if !s.actionTargetsCandidate(ReasonRetryRejected, "Retry") {
			return false
		}
		if !s.retryable() {
			s.emit(corev1.EventTypeWarning, ReasonRetryRejected, fmt.Sprintf(
				"Retry rejected: rollout of revision %s is %s; retry needs Failed, RolledBack, Aborted or a rollout held by a strategy change",
				s.status.CandidateRevision, s.status.Phase))
			return false
		}
		s.restartCandidate()
		return false
	}
	return false
}

// actionTargetsCandidate reports whether the action names the current candidate revision. Otherwise the action
// is rejected with a Warning event and must not change the rollout.
func (s *stepper) actionTargetsCandidate(reason, action string) bool {
	if s.in.ActionRevision == s.status.CandidateRevision {
		return true
	}
	current := s.status.CandidateRevision
	if current == "" {
		current = "none"
	}
	s.emit(corev1.EventTypeWarning, reason, fmt.Sprintf("%s rejected: targets revision %s but the current candidate is %s",
		action, s.in.ActionRevision, current))
	return false
}

func (s *stepper) promoteRejectedMessage() string {
	switch {
	case s.status.CandidateRevision == "":
		return "Promote rejected: no candidate revision"
	case !isActive(s.status.Phase):
		return fmt.Sprintf("Promote rejected: rollout of revision %s is %s", s.status.CandidateRevision, s.status.Phase)
	default:
		return fmt.Sprintf("Promote rejected: candidate revision %s is not ready", s.status.CandidateRevision)
	}
}

// startPromotion sends the candidate all traffic. The candidate becomes stable once Knative confirms the split.
// A held share is kept, so that a failed promotion returns to it.
func (s *stepper) startPromotion() {
	s.status.Phase = cappv1alpha1.RolloutPhasePromoting
	s.status.Message = fmt.Sprintf("Promoting revision %s to 100%% of traffic; waiting for Knative to apply the split",
		s.status.CandidateRevision)
}

// confirmPromotion makes the candidate stable and ends the rollout once Knative serves the promotion split.
func (s *stepper) confirmPromotion() {
	if !trafficConfirmed(s.traffic(), s.in.AppliedTraffic) {
		return
	}
	candidate := s.status.CandidateRevision
	s.status = stableStatus(candidate)
	s.status.Message = fmt.Sprintf("Revision %s promoted to stable", candidate)
	s.emit(corev1.EventTypeNormal, ReasonCompleted, s.status.Message)
}

// restartCandidate starts the rollout of the current candidate again from its first stage. Its progress deadline
// starts again too.
func (s *stepper) restartCandidate() {
	since := metav1.NewTime(s.in.Now)
	s.status.Phase = cappv1alpha1.RolloutPhaseProgressing
	s.status.StageIndex = 0
	s.status.StageStartTime = nil
	s.clearHold()
	s.status.CandidateReadyOnce = false
	s.status.CandidateSince = &since
	s.status.CandidateSinceFallback = false
	s.status.Message = fmt.Sprintf("Retrying rollout of revision %s from stage 1 of %d", s.status.CandidateRevision, len(s.stages))
	s.emit(corev1.EventTypeNormal, ReasonRetried, s.status.Message)
}

// waitingForFirstReady reports whether the candidate has not been ready yet, which is when the progress deadline applies.
func (s *stepper) waitingForFirstReady() bool {
	return !s.status.CandidateReadyOnce && s.status.CandidateSince != nil
}

// checkHealth fails or rolls back a candidate that reports Ready=False or misses the progress deadline,
// and resumes a failed candidate that became ready. It returns true when stages may advance.
func (s *stepper) checkHealth() bool {
	if !s.hasActiveCandidate() {
		return false
	}

	ready := s.in.CandidateReady
	missedDeadline := ready != metav1.ConditionTrue && s.waitingForFirstReady() &&
		!s.in.Now.Before(s.status.CandidateSince.Add(s.deadline))

	if ready == metav1.ConditionFalse || missedDeadline {
		candidate, stable := s.status.CandidateRevision, s.status.StableRevision
		reason := "reports Ready=False"
		if ready != metav1.ConditionFalse {
			reason = fmt.Sprintf("did not become ready within %s", s.deadline)
		}

		if s.in.Strategy.AutoRollback {
			s.status.Phase = cappv1alpha1.RolloutPhaseRolledBack
			s.clearHold()
			s.status.Message = fmt.Sprintf("Candidate revision %s %s; rolled back to stable revision %s",
				candidate, reason, stable)
			s.emit(corev1.EventTypeWarning, ReasonRolledBack, s.status.Message)
		} else {
			// The traffic stays at the split of the current stage and the stage timer restarts on recovery.
			s.status.StageStartTime = nil
			if s.status.Phase != cappv1alpha1.RolloutPhaseFailed {
				fromPromotion := s.status.Phase == cappv1alpha1.RolloutPhasePromoting
				s.status.Phase = cappv1alpha1.RolloutPhaseFailed
				if fromPromotion {
					s.fallBackFromPromotion()
				}
				s.status.Message = fmt.Sprintf("Candidate revision %s %s; stable revision is %s; abort the rollout, or wait for the candidate to become Ready to promote",
					candidate, reason, stable)
				s.emit(corev1.EventTypeWarning, ReasonFailed, s.status.Message)
			}
		}
		return false
	}

	if ready != metav1.ConditionTrue {
		return false
	}

	s.status.CandidateReadyOnce = true
	if s.status.Phase == cappv1alpha1.RolloutPhaseFailed {
		s.status.Phase = cappv1alpha1.RolloutPhaseProgressing
		s.status.StageStartTime = nil
		s.clearPromotionFallback()
		s.status.Message = fmt.Sprintf("Candidate revision %s recovered; rollout resumed at stage %d of %d",
			s.status.CandidateRevision, s.status.StageIndex+1, len(s.stages))
	}
	return true
}

// fallBackFromPromotion returns the traffic of a failed promotion to the split the candidate had before it. A held share
// is kept. A promotion from the final 100% stage falls back to the previous stage, or to 0% when that stage is the only
// one. A promotion from any other stage keeps the split of that stage.
func (s *stepper) fallBackFromPromotion() {
	last := len(s.stages) - 1
	if s.status.HeldPercent != nil || int(s.status.StageIndex) != last || s.stages[last].Percent != 100 {
		return
	}
	if last == 0 {
		zero := int64(0)
		s.status.HeldPercent = &zero
		s.status.PromotionFallback = true
		return
	}
	s.status.StageIndex = toStageIndex(last - 1)
}

// clearPromotionFallback lifts the 0% hold that fallBackFromPromotion set when a promotion from the only, final 100%
// stage failed. The rollout then resumes at that stage, which promotes again. A strategy edit during the fallback can
// move the rollout off that stage, and then the hold stays as an ordinary strategy-change hold that needs a user action.
func (s *stepper) clearPromotionFallback() {
	if !s.status.PromotionFallback {
		return
	}
	s.status.PromotionFallback = false
	last := len(s.stages) - 1
	if int(s.status.StageIndex) == last && s.stages[last].Percent == 100 {
		s.status.HeldPercent = nil
	}
}

// describeHold sets the message of a held rollout, unless the message already describes the hold. A recovery or a strategy
// edit can leave the message of another phase behind, while the rollout still holds.
func (s *stepper) describeHold() {
	clause := fmt.Sprintf("holds at %d%% of traffic because no stage is at or below it", *s.status.HeldPercent)
	if strings.Contains(s.status.Message, clause) {
		return
	}
	s.status.Message = fmt.Sprintf("Candidate revision %s %s; promote, abort or retry the rollout", s.status.CandidateRevision, clause)
}

// clearHold lifts the hold of the rollout, if any, and the promotion fallback that the hold may be.
func (s *stepper) clearHold() {
	s.status.HeldPercent = nil
	s.status.PromotionFallback = false
}

// advance moves a ready candidate through its stages. A stage's timer starts only once Knative confirms the stage's
// traffic split, and at most one stage is entered per call. A final 100% stage promotes as soon as it is reached,
// and a final stage below 100% holds once its split is confirmed. A held rollout does not move.
func (s *stepper) advance() {
	if s.status.Phase == cappv1alpha1.RolloutPhasePromoting {
		s.confirmPromotion()
		return
	}
	if s.status.HeldPercent != nil {
		s.status.Phase = cappv1alpha1.RolloutPhaseHolding
		s.describeHold()
		return
	}

	last := toStageIndex(len(s.stages) - 1)
	if s.status.StageStartTime != nil && s.stageElapsed() && s.status.StageIndex < last {
		s.status.StageIndex++
		s.status.StageStartTime = nil
	}

	if s.status.StageIndex == last && s.stages[last].Percent == 100 {
		s.startPromotion()
		s.confirmPromotion()
		return
	}

	if s.status.StageStartTime == nil {
		if !s.splitConfirmed() {
			s.awaitSplit()
			return
		}
		start := metav1.NewTime(s.in.Now)
		s.status.StageStartTime = &start
		s.status.Message = fmt.Sprintf("Candidate revision %s receiving %d%% of traffic at stage %d of %d",
			s.status.CandidateRevision, s.stages[s.status.StageIndex].Percent, s.status.StageIndex+1, len(s.stages))
	}

	stage := s.stages[s.status.StageIndex]
	switch {
	case stage.Duration == nil || (s.status.StageIndex == last && s.stageElapsed()):
		if s.status.Phase != cappv1alpha1.RolloutPhaseHolding {
			s.status.Phase = cappv1alpha1.RolloutPhaseHolding
			s.status.Message = fmt.Sprintf("Holding candidate revision %s at %d%% of traffic; promote or abort the rollout",
				s.status.CandidateRevision, stage.Percent)
		}
	default:
		s.status.Phase = cappv1alpha1.RolloutPhaseProgressing
	}
}

// awaitSplit keeps the rollout at its current stage until Knative confirms the stage's traffic split.
func (s *stepper) awaitSplit() {
	s.status.Phase = cappv1alpha1.RolloutPhaseProgressing
	s.status.Message = fmt.Sprintf("Candidate revision %s: waiting for Knative to apply %d%% traffic split at stage %d of %d",
		s.status.CandidateRevision, s.stages[s.status.StageIndex].Percent, s.status.StageIndex+1, len(s.stages))
}

// splitConfirmed reports whether Knative serves the traffic that the rollout expects, as given by AppliedTraffic.
func (s *stepper) splitConfirmed() bool {
	return trafficConfirmed(s.traffic(), s.in.AppliedTraffic)
}

// trafficConfirmed reports whether applied gives each revision the percent it has in want. A revision that is missing
// from either table, or has no traffic, counts as 0%.
func trafficConfirmed(want, applied []cappv1alpha1.RevisionTraffic) bool {
	return maps.Equal(nonZeroPercents(want), nonZeroPercents(applied))
}

func nonZeroPercents(traffic []cappv1alpha1.RevisionTraffic) map[string]int64 {
	percents := make(map[string]int64, len(traffic))
	for _, t := range traffic {
		if t.Percent != 0 {
			percents[t.RevisionName] += t.Percent
		}
	}
	return percents
}

// stageRemaining returns the time left in the current stage, or ok=false when the stage has no duration.
func (s *stepper) stageRemaining() (time.Duration, bool) {
	stage := s.stages[s.status.StageIndex]
	if stage.Duration == nil || s.status.StageStartTime == nil {
		return 0, false
	}
	return s.status.StageStartTime.Add(stage.Duration.Duration).Sub(s.in.Now), true
}

func (s *stepper) stageElapsed() bool {
	remaining, ok := s.stageRemaining()
	return ok && remaining <= 0
}

// candidatePercent returns the share of traffic the candidate gets: the held share, or the share of the current stage.
func (s *stepper) candidatePercent() int64 {
	if s.status.HeldPercent != nil {
		return *s.status.HeldPercent
	}
	return s.stages[s.status.StageIndex].Percent
}

// traffic returns the traffic table for the current status: stable first, then candidate. A candidate that has never
// been Ready is left out, so Knative keeps serving stable only.
func (s *stepper) traffic() []cappv1alpha1.RevisionTraffic {
	stable := s.status.StableRevision
	if stable == "" {
		return nil
	}
	if !s.hasActiveCandidate() {
		return []cappv1alpha1.RevisionTraffic{{RevisionName: stable, Percent: 100, Tag: TagStable}}
	}
	if s.status.Phase == cappv1alpha1.RolloutPhasePromoting {
		return promotionTraffic(stable, s.status.CandidateRevision)
	}
	if !s.status.CandidateReadyOnce {
		return []cappv1alpha1.RevisionTraffic{{RevisionName: stable, Percent: 100, Tag: TagStable}}
	}

	percent := s.candidatePercent()
	if s.status.Phase == cappv1alpha1.RolloutPhaseFailed && percent == 0 {
		return []cappv1alpha1.RevisionTraffic{{RevisionName: stable, Percent: 100, Tag: TagStable}}
	}
	return []cappv1alpha1.RevisionTraffic{
		{RevisionName: stable, Percent: 100 - percent, Tag: TagStable},
		{RevisionName: s.status.CandidateRevision, Percent: percent, Tag: TagCandidate},
	}
}

// promotionTraffic returns the split of a promotion. The candidate gets all traffic, and stable stays in the table at 0%.
func promotionTraffic(stable, candidate string) []cappv1alpha1.RevisionTraffic {
	return []cappv1alpha1.RevisionTraffic{
		{RevisionName: stable, Percent: 0, Tag: TagStable},
		{RevisionName: candidate, Percent: 100, Tag: TagCandidate},
	}
}

// defaultRouteTraffic returns the split of Knative's default routing, which applies once the Knative Service has no
// traffic targets. All traffic goes to the latest Ready revision, which is the candidate here.
func defaultRouteTraffic(candidate string) []cappv1alpha1.RevisionTraffic {
	return []cappv1alpha1.RevisionTraffic{{RevisionName: candidate, Percent: 100}}
}

// stableStatus returns the status of a rollout that has no candidate, with revision as stable.
func stableStatus(revision string) cappv1alpha1.RolloutStatus {
	return cappv1alpha1.RolloutStatus{StableRevision: revision, Phase: cappv1alpha1.RolloutPhaseStable}
}

// requeueAfter returns when the rollout must be stepped again without an external trigger.
func (s *stepper) requeueAfter() time.Duration {
	if s.status.Phase == cappv1alpha1.RolloutPhasePromoting {
		return splitConfirmationRequeue
	}
	if s.status.Phase != cappv1alpha1.RolloutPhaseProgressing || s.status.CandidateRevision == "" {
		return 0
	}
	if s.in.CandidateReady != metav1.ConditionTrue {
		if !s.waitingForFirstReady() {
			return 0
		}
		return s.status.CandidateSince.Add(s.deadline).Sub(s.in.Now)
	}
	if s.status.StageStartTime == nil {
		return splitConfirmationRequeue
	}
	if remaining, ok := s.stageRemaining(); ok && remaining > 0 {
		return remaining
	}
	return 0
}
