# Traffic Splitting & Staged Rollouts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a Capp declare a `rolloutStrategy` (stages of candidate traffic percent + duration) that the operator drives automatically between a stable and a candidate Knative revision.

**Architecture:** A pure state machine (`internal/kinds/capp/rollout`) takes the previous `RolloutStatus`, the strategy, Knative revision facts and `now`, and returns the next `RolloutStatus`, events and a requeue delay. The controller runs it before resource managers and stores the result on the in-memory Capp. The ksvc manager turns `capp.Status.Rollout.Traffic` into `ksvc.Spec.Traffic`. `SyncStatus` persists it and derives a `RolloutReady` condition.

**Tech Stack:** Go, controller-runtime, Knative Serving v1, kubebuilder (CEL markers), testify (unit), Ginkgo/Gomega (e2e in `test/e2e`).

**Spec:** `docs/traffic-splitting-design.md`

## Global Constraints

- Capps without `routeSpec.rolloutStrategy` keep today's traffic: Knative defaults (100% latest). `ksvc.Spec.Traffic` must stay unset for them, except while a rejected candidate (from a removed strategy) is still the latest revision (Task 9).
- While a strategy is active, traffic targets use `revisionName` only, never `latestRevision: true`.
- Tags are exactly `stable` and `candidate`.
- Annotation key `rcs.dana.io/rollout`; values `promote:<revision>`, `abort:<revision>`, `retry:<revision>` (Tasks 8–9; originally bare `promote`/`abort`). The operator removes it after the status is persisted, with a JSON patch `test`+`remove` on the processed value.
- `Ready` keeps its EXISTING meaning (summary incl. unchanged `checkKnative`); the rollout does not feed it. New conditions `Available` and `RolloutSucceeded`, both outside the `Ready` summary (Task 9; replaces the original RolloutReady-in-Ready design).
- Default `progressDeadline`: `10m`.
- Rollout timing is derived from status timestamps only, never in-memory timers.
- After `make manifests`, copy the CRD to `charts/container-app-operator/crds/`.
- Commits: one commit for the whole feature branch. Tasks 1–7 are already squashed into one commit; amend Task 8 into it. Do not push or open a PR.
- Subagents: Haiku implements, Opus reviews.

## Plan Additions Beyond the Spec

- **New phase `Failed`:** the spec's state for "candidate not Ready, `autoRollback: false`" had no phase name. It is `Failed`. A candidate that recovers to Ready returns to `Progressing`.
- **New status field `CandidateSince`:** the candidate Revision's `creationTimestamp` (Task 8; originally detection time), the start of `progressDeadline`.

## Review Focus

1. **Operator restart mid-stage:** a stage whose duration already passed while the operator was down advances by one stage on the first reconcile; the next stage's timer then waits for Knative to confirm the split (Task 3 test `TestStep_ElapsedStageAdvancesImmediately`, adjusted in Task 8).
6. **Stage timer vs routing delay (PR #746 review):** `StageStartTime` is only set once Knative confirms the stage's split is serving (Task 8).
7. **Stale/misdirected action (PR #746 review):** an action naming a revision other than the current candidate is rejected (Task 8).
8. **Strategy edit never raises traffic (PR #746 review):** `[10,50,100]` at 50% → `[10,100]` lands on 10%, not 100% (Task 9).
9. **Strategy removal never resurrects a rejected candidate (PR #746 review):** traffic pinned to stable (Task 9).
10. **Unready new candidate is omitted from traffic** so Knative programs `[stable:100]` immediately (Task 9).
2. **Candidate recovers after failing:** a `Failed` candidate that becomes Ready resumes `Progressing` from its current stage (Task 3 test `TestStep_FailedCandidateRecovers`).
3. **Unknown annotation value (`promot`):** ignored, Warning event, annotation still removed (Task 3 `TestParseAction`, Task 6 test).
4. **Disable then re-enable mid-rollout:** with no ksvc (`LatestCreated == ""`), status resets to zero (Task 3 test `TestStep_NoKsvcResets`).
5. **Zero or negative non-final stage duration:** rejected by the webhook (Task 2 test case `zero duration on non-final stage`).

---

## Dependency graph

```
Task 1 (API types)
  ├── Task 2 (webhook validation)      ─┐
  ├── Task 3 (rollout state machine)    │ parallel
  ├── Task 4 (ksvc traffic)             │
  └── Task 5 (RolloutReady condition)  ─┘
Task 6 (controller wiring)  ← needs 3, 4
Task 7 (e2e + user guide)   ← needs all
Task 8 (PR #746 review fixes) ← needs all
Task 9 (PR #746 second review round) ← needs 8
```

Tasks 2–5 touch disjoint files and can be done by different people at the same time.

---

### Task 1: API types, CRD and deprecation

**Files:**
- Modify: `api/v1alpha1/capp_types.go` (RouteSpec ~L220–238, constants ~L30–63, CappStatus ~L372–417)
- Modify: `internal/kinds/capp/cappmeta/cappmeta.go` (annotation key)
- Regenerate: `api/v1alpha1/zz_generated.deepcopy.go`, `config/crd/bases/rcs.dana.io_capps.yaml`
- Copy: `charts/container-app-operator/crds/rcs.dana.io_capps.yaml`

**Interfaces:**
- Produces (package `v1alpha1`):

```go
type RolloutStrategy struct {
    // +kubebuilder:validation:MinItems=1
    Stages           []RolloutStage   `json:"stages"`
    AutoRollback     bool             `json:"autoRollback,omitempty"`
    ProgressDeadline *metav1.Duration `json:"progressDeadline,omitempty"`
}
type RolloutStage struct {
    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=100
    Percent  int64            `json:"percent"`
    Duration *metav1.Duration `json:"duration,omitempty"`
}
// RouteSpec gains:
RolloutStrategy *RolloutStrategy `json:"rolloutStrategy,omitempty"`

type RolloutPhase string
const (
    RolloutPhaseStable      RolloutPhase = "Stable"
    RolloutPhaseProgressing RolloutPhase = "Progressing"
    RolloutPhaseHolding     RolloutPhase = "Holding"
    RolloutPhaseFailed      RolloutPhase = "Failed"
    RolloutPhaseRolledBack  RolloutPhase = "RolledBack"
    RolloutPhaseAborted     RolloutPhase = "Aborted"
)
type RevisionTraffic struct {
    RevisionName string `json:"revisionName"`
    Percent      int64  `json:"percent"`
    Tag          string `json:"tag"`
}
type RolloutStatus struct {
    StableRevision    string            `json:"stableRevision,omitempty"`
    CandidateRevision string            `json:"candidateRevision,omitempty"`
    Phase             RolloutPhase      `json:"phase,omitempty"`
    StageIndex        int32             `json:"stageIndex,omitempty"`
    StageStartTime    *metav1.Time      `json:"stageStartTime,omitempty"` // nil until candidate Ready
    CandidateSince    *metav1.Time      `json:"candidateSince,omitempty"`
    Traffic           []RevisionTraffic `json:"traffic,omitempty"`
    Message           string            `json:"message,omitempty"`
}
// CappStatus gains:
Rollout *RolloutStatus `json:"rollout,omitempty"`

const (
    CappConditionRolloutReady      = "RolloutReady"
    CappReadyReasonRolloutNotReady = "RolloutNotReady"
)
```

- Produces (package `cappmeta`): `RolloutAnnotationKey = CappAPIGroup + "/rollout"`, `RolloutActionPromote = "promote"`, `RolloutActionAbort = "abort"`.
- `RouteSpec.TrafficTarget` doc comment gains `// Deprecated: ignored by the operator; use RolloutStrategy.`

- [ ] **Step 1:** Add the types, fields and constants above, each with a doc comment in the file's existing style.
- [ ] **Step 2:** Run `make manifests generate`. Expected: no errors. `git diff config/crd` shows `rolloutStrategy` and `rollout`.
- [ ] **Step 3:** Copy `config/crd/bases/rcs.dana.io_capps.yaml` to `charts/container-app-operator/crds/`. Also check whether `rcs.dana.io_capprevisions.yaml` changed (it embeds `RouteSpec`) and copy it if so.
- [ ] **Step 4:** Run `make test`. Expected: PASS (no behaviour change yet).
- [ ] **Step 5:** Commit: `feat(api): add rolloutStrategy and rollout status to Capp`.

---

### Task 2: Webhook validation of rolloutStrategy

**Files:**
- Modify: `internal/webhook/rcs/v1alpha1/validator.go` (add a call in `handle` after `validateScaleSpec`, add the func)
- Test: `internal/webhook/rcs/v1alpha1/validator_test.go`

**Interfaces:**
- Consumes: `RolloutStrategy`, `RolloutStage` (Task 1)
- Produces: `func validateRolloutStrategy(capp cappv1alpha1.Capp) error`. Returns nil when `RolloutStrategy` is nil.

- [ ] **Step 1: Write the failing table test** `TestValidateRolloutStrategy`:

| case | stages | want error contains |
|---|---|---|
| nil strategy | — | nil |
| canary | `{10,5m},{50,10m},{100,nil}` | nil |
| steady split | `{50,nil}` | nil |
| blue-green | `{0,nil}` | nil |
| missing duration on non-final stage | `{10,nil},{100,nil}` | `"stage 0: duration is required on all but the last stage"` |
| zero duration on non-final stage | `{10,0s},{100,nil}` | `"stage 0: duration must be positive"` |
| negative progressDeadline | `{100,nil}`, deadline `-1m` | `"progressDeadline must be positive"` |

- [ ] **Step 2:** Run `go test ./internal/webhook/rcs/v1alpha1/ -run TestValidateRolloutStrategy`. Expected: FAIL (undefined).
- [ ] **Step 3:** Implement `validateRolloutStrategy` and call it from `handle` with `admission.Denied(err.Error())`, matching the neighbouring validators.
- [ ] **Step 4:** Run the test again. Expected: PASS. Then run `make test`.
- [ ] **Step 5:** Commit: `feat(webhook): validate Capp rolloutStrategy`.

---

### Task 3: Rollout state machine

**Files:**
- Create: `internal/kinds/capp/rollout/rollout.go`
- Test: `internal/kinds/capp/rollout/rollout_test.go`

**Interfaces:**
- Consumes: Task 1 types, `cappmeta.RolloutAction*`
- Produces:

```go
type Action string // "", "promote", "abort"
const (ActionNone Action = ""; ActionPromote Action = "promote"; ActionAbort Action = "abort")

// ParseAction returns the action for an annotation value and ok=false for unknown values.
func ParseAction(v string) (Action, bool)

type Input struct {
    Strategy       *cappv1alpha1.RolloutStrategy
    Previous       cappv1alpha1.RolloutStatus // zero value when Capp has none
    LatestCreated  string                     // ksvc.Status.LatestCreatedRevisionName; "" when no ksvc
    LatestReady    string                     // ksvc.Status.LatestReadyRevisionName
    CandidateReady metav1.ConditionStatus     // Ready of the LatestCreated Revision object
    Action         Action
    Now            time.Time
}
type Event struct{ Type, Reason, Message string } // Type is corev1.EventTypeNormal/Warning

type Output struct {
    Status       cappv1alpha1.RolloutStatus
    RequeueAfter time.Duration // 0 = no timed requeue
    Events       []Event
}

const DefaultProgressDeadline = 10 * time.Minute

func Step(in Input) Output
```

`Step` is pure: no clients and no clock reads. The rules below are what it must do. Each bullet is a test.

**Test list for `rollout_test.go`** (table-driven or one test per rule; names fixed):

- `TestStep_NoKsvcResets`: `LatestCreated == ""` → zero `RolloutStatus`.
- `TestStep_NoStrategyTracksStable`: `Strategy == nil`, `LatestReady == "r1"` → `StableRevision == "r1"`, `Phase == Stable`, `Traffic` empty.
- `TestStep_FirstRevision`: strategy set, no stable, `LatestReady == "r1"` → stable `r1`, `Traffic == [{r1,100,"stable"}]`, `Phase == Stable`.
- `TestStep_FirstRevisionNotReady`: strategy set, no stable, `LatestReady == ""` → `Traffic` empty (Knative default), stable empty.
- `TestStep_NewRevisionStartsRollout`: stable `r1`, `LatestCreated == "r2"` → candidate `r2`, `StageIndex 0`, `StageStartTime nil`, `CandidateSince == Now`, `Phase == Progressing`, `Traffic == [{r1,100,"stable"},{r2,0,"candidate"}]`, `RequeueAfter == DefaultProgressDeadline`.
- `TestStep_CandidateReadyStartsStage`: candidate `r2` ready, stage 0 `{10,5m}` → `StageStartTime == Now`, traffic `r1:90 r2:10`, `RequeueAfter == 5m`.
- `TestStep_StageNotElapsedRequeuesRemaining`: started 2m ago, 5m stage → traffic unchanged, `RequeueAfter == 3m`.
- `TestStep_ElapsedStageAdvancesImmediately`: started 20m ago, stage 0 `{10,5m}`, stage 1 `{50,10m}` → `StageIndex 1`, `StageStartTime == Now`, traffic `r1:50 r2:50`.
- `TestStep_FinalHundredPromotes`: advancing into a final `{100}` stage → stable `r2`, candidate empty, `Traffic == [{r2,100,"stable"}]`, `Phase == Stable`, a Normal event with reason `RolloutCompleted`.
- `TestStep_FinalHoldBelowHundred`: final stage `{50}` reached → `Phase == Holding`, traffic `r1:50 r2:50`, `RequeueAfter == 0`.
- `TestStep_NewRevisionDuringRolloutRestarts`: stable `r1`, candidate `r2` at stage 1, `LatestCreated == "r3"` → candidate `r3`, `StageIndex 0`, `StageStartTime nil`, traffic `r1:100 r3:0`, a Warning event with reason `RolloutRestarted` whose message names `r2` and `r3`.
- `TestStep_DeadlineAutoRollback`: `autoRollback: true`, `CandidateSince` 11m ago, `CandidateReady != True` → `Phase == RolledBack`, traffic `r1:100` only, a Warning event with reason `RolloutRolledBack`.
- `TestStep_ReadyFalseAutoRollback`: `autoRollback: true`, `CandidateReady == False` mid-stage → `RolledBack` (same expectations).
- `TestStep_DeadlineNoAutoRollback`: `autoRollback: false`, deadline passed → `Phase == Failed`, traffic unchanged, a Warning event with reason `RolloutFailed`.
- `TestStep_FailedCandidateRecovers`: `Phase == Failed`, `CandidateReady == True` → `Phase == Progressing`, same `StageIndex`, `StageStartTime == Now`.
- `TestStep_RolledBackDoesNotRetrySameRevision`: `Phase == RolledBack`, candidate `r2`, `LatestCreated == "r2"` → status unchanged, no events.
- `TestStep_PromoteReady`: `Action == promote`, candidate Ready → same result as `FinalHundredPromotes`.
- `TestStep_PromoteRejected`: `Action == promote` with candidate not Ready, or `Phase` `RolledBack`/`Aborted` → status unchanged, a Warning event with reason `RolloutPromoteRejected`.
- `TestStep_Abort`: `Action == abort` → `Phase == Aborted`, traffic `r1:100`, a Normal event with reason `RolloutAborted`.
- `TestStep_StrategyRemovedPromotes`: `Strategy == nil`, `Previous` has candidate `r2` → stable `r2`, candidate empty, `Traffic` empty, `Phase == Stable`.
- `TestStep_StrategyShrinkClampsStage`: `StageIndex 3`, new strategy has 2 stages → `StageIndex 1`.
- `TestParseAction`: `"promote"`, `"abort"`, `""` → ok; `"promot"` → `ok == false`.

- [ ] **Step 1:** Write all tests above. Run `go test ./internal/kinds/capp/rollout/`. Expected: FAIL (package does not compile).
- [ ] **Step 2:** Implement `Step` and `ParseAction`. Order of evaluation inside `Step`:
  1. No ksvc → reset.
  2. Strategy nil → handle the candidate (promote), track stable.
  3. Apply the clamp.
  4. Detect a new revision (which may restart the rollout).
  5. Handle the action.
  6. Check the candidate's health against the deadline and `autoRollback`.
  7. Advance the stage timer (loop while elapsed, so multiple overdue stages collapse).
  8. Compute traffic and the requeue delay.

  Stage timers count from `StageStartTime`. The deadline counts from `CandidateSince`.
- [ ] **Step 3:** Run `go test ./internal/kinds/capp/rollout/ -v`. Expected: all PASS.
- [ ] **Step 4:** Commit: `feat(rollout): add stable/candidate rollout state machine`.

---

### Task 4: Knative Service traffic from rollout status

**Files:**
- Modify: `internal/kinds/capp/resourcemanagers/ksvc.go` (`prepareResource`, ~L80–83)
- Test: `internal/kinds/capp/resourcemanagers/ksvc_test.go` (extend `TestKnativeServiceManagerPrepareResource`)

**Interfaces:**
- Consumes: `capp.Status.Rollout.Traffic` (Task 1)
- Produces: `func buildTraffic(capp cappv1alpha1.Capp) []knativev1.TrafficTarget`. Returns nil when `capp.Spec.RouteSpec.RolloutStrategy == nil` or `capp.Status.Rollout == nil` or `len(Traffic) == 0`. Otherwise it returns one target per entry with `RevisionName`, `Percent: ptr(Percent)`, `Tag`, `LatestRevision: ptr(false)`.

- [ ] **Step 1: Write failing cases** in `TestKnativeServiceManagerPrepareResource`:
  - No strategy, status with traffic → `ksvc.Spec.Traffic` equals what `RouteSpec.SetDefaults` produces today (the existing default case keeps passing).
  - Strategy plus status traffic `[{r1,90,stable},{r2,10,candidate}]` → `ksvc.Spec.Traffic` has exactly those two targets, `LatestRevision == false`.
- [ ] **Step 2:** Run `go test ./internal/kinds/capp/resourcemanagers/ -run TestKnativeServiceManagerPrepareResource`. Expected: FAIL.
- [ ] **Step 3:** Implement `buildTraffic`. In `prepareResource`, when it returns non-nil, set `knativeService.Spec.Traffic` instead of relying on `RouteSpec.SetDefaults`.
- [ ] **Step 4:** Run the resourcemanagers package tests. Expected: PASS.
- [ ] **Step 5:** Commit: `feat(ksvc): apply rollout traffic split to Knative Service`.

---

### Task 5: RolloutReady condition

**Files:**
- Modify: `internal/kinds/capp/status/capp.go` (`capability` struct, `capabilities` slice, `isCapabilityConfigured`)
- Create: `internal/kinds/capp/status/rollout.go` (the check func)
- Test: `internal/kinds/capp/status/rollout_test.go`, plus a case in `capp_test.go` for the `Ready` summary

**Interfaces:**
- Consumes: `capp.Status.Rollout`, `CappConditionRolloutReady`, `CappReadyReasonRolloutNotReady` (Task 1)
- Produces:
  - A new optional field on `capability`: `configured func(capp cappv1alpha1.Capp) bool`. When set, `isCapabilityConfigured` uses it instead of `managers`.
  - `func checkRollout(ctx, k8sClient, capp, cappConfig) (*metav1.Condition, error)`, with the same signature as the other `check` funcs.
  - A capability entry: `{conditionType: RolloutReady, readyReason: RolloutNotReady, dependsOn: [ServingReady], configured: strategy != nil, check: checkRollout}`.

- [ ] **Step 1: Write failing tests** `TestCheckRollout`, phase → result:
  - `Stable`, `Progressing`, `Holding` → nil (Ready).
  - `Failed` → reason `RolloutFailed`.
  - `RolledBack` → reason `RolloutRolledBack`.
  - `Aborted` → reason `RolloutAborted`.
  - Each non-nil condition is `Status False`, and its message is `capp.Status.Rollout.Message`.

  Also add a case in `capp_test.go`: a Capp with ServingReady True and Rollout `RolledBack` → `Ready` False with reason `RolloutNotReady`. A Capp without a strategy has no `RolloutReady` condition.
- [ ] **Step 2:** Run `go test ./internal/kinds/capp/status/`. Expected: FAIL.
- [ ] **Step 3:** Implement the code above.
- [ ] **Step 4:** Run the status package tests. Expected: PASS.
- [ ] **Step 5:** Commit: `feat(status): add RolloutReady condition`.

---

### Task 6: Controller wiring

**Files:**
- Modify: `internal/kinds/capp/controllers/controller.go` (`Reconcile` ~L408–462, `SyncApplication` ~L464–482, RBAC markers ~L60–80)
- Modify: `internal/kinds/capp/status/capp.go` (`SyncStatus`: copy `capp.Status.Rollout` onto `cappObject.Status` before the diff)
- Create: `internal/kinds/capp/controllers/rollout.go`
- Test: `internal/kinds/capp/controllers/rollout_test.go` (fake client)

**Interfaces:**
- Consumes: `rollout.Step`, `rollout.ParseAction`, `rollout.Input/Output` (Task 3), `buildTraffic` via the ksvc manager (Task 4), `cappmeta.RolloutAnnotationKey` (Task 1)
- Produces: `func (r *CappReconciler) runRollout(ctx context.Context, capp *cappv1alpha1.Capp) (time.Duration, error)`, which:
  1. reads the ksvc (NotFound → `LatestCreated = ""`);
  2. reads the `serving.knative.dev/v1` Revision named `LatestCreated` and takes its `Ready` condition status (missing → `Unknown`);
  3. parses the annotation (unknown value → Warning event `RolloutInvalidAction`, treated as none);
  4. calls `rollout.Step` with `Now: time.Now()`;
  5. sets `capp.Status.Rollout = &out.Status`, or nil when it is the zero value;
  6. emits `out.Events` through `r.EventRecorder`;
  7. if the annotation was present, removes it with a merge patch on the Capp;
  8. returns `out.RequeueAfter`.

  `Reconcile` calls `runRollout` before `SyncApplication`. It returns `ctrl.Result{RequeueAfter: d}` when `d > 0` and no error occurred.

- [ ] **Step 1: Write failing tests** with the controller-runtime fake client:
  - `TestRunRollout_SetsStatusAndRequeue`: ksvc with `LatestCreated r2`, ready Revision `r2`, Capp stable `r1`, strategy `{10,5m},{100}` → `capp.Status.Rollout.CandidateRevision == "r2"`, `RequeueAfter == 5m`.
  - `TestRunRollout_RemovesAnnotation`: Capp annotated `promote` → after the call, the stored Capp has no `rcs.dana.io/rollout` annotation.
  - `TestRunRollout_InvalidAnnotation`: value `promot` → a Warning event `RolloutInvalidAction` is recorded, the annotation is removed, and the status is not promoted.
  - `TestRunRollout_NoKsvc`: no ksvc → `capp.Status.Rollout == nil`, `RequeueAfter == 0`.
- [ ] **Step 2:** Run `go test ./internal/kinds/capp/controllers/ -run TestRunRollout`. Expected: FAIL.
- [ ] **Step 3:** Implement `runRollout`, wire it into `Reconcile`, and copy `Rollout` in `SyncStatus`. Add the RBAC marker `// +kubebuilder:rbac:groups="serving.knative.dev",resources=revisions,verbs=get;list;watch` if it is not already present, then run `make manifests`.
- [ ] **Step 4:** Run `make test`. Expected: PASS.
- [ ] **Step 5:** Commit: `feat(controller): drive Capp rollouts from reconcile`.

---

### Task 7: E2E coverage and user guide

**Files:**
- Create: `test/e2e/rollout_test.go`
- Modify: `test/e2e/utils/` (add a helper to update the Capp image, if none exists)
- Modify: `docs/user-guide.md` (the `routeSpec` section ~L22–30, and a new section)

**Interfaces:**
- Consumes: all prior tasks.

- [ ] **Step 1: Write the e2e specs** (Ginkgo, matching `test/e2e/ksvc_test.go` style). Use short durations (`15s` stages):
  - *Canary*: create a Capp with stages `{10,15s},{100}`; change the image. `Eventually`, the ksvc traffic shows the old revision at 90 and the new one at 10. Then the phase is `Stable` with the new revision as stable.
  - *Restart on new revision*: during stage 0, change the image again. The candidate becomes the third revision, and a `RolloutRestarted` event exists.
  - *Auto rollback*: with `autoRollback: true` and `progressDeadline: 30s`, use an unpullable image. The phase becomes `RolledBack`, the old revision gets 100%, and Capp `Ready` is False.
  - *Promote*: blue-green `{0}`. Annotate `promote`. The new revision gets 100%, and the annotation is gone.
  - *No strategy*: the existing Capp ksvc keeps `latestRevision: true` at 100%.
- [ ] **Step 2:** Run `make test-e2e` against a cluster. Expected: PASS.
- [ ] **Step 3:** Update the user guide:
  - remove `trafficTarget` and note that it is deprecated;
  - add a "Traffic splitting & rollouts" section with the canary, steady split and blue-green YAML from the spec, the annotation usage, the tag URLs (Knative domain only), and the `Ready` semantics;
  - add a clearly marked **"Behaviour to know: a new revision restarts the rollout"** subsection that explains rev1→rev2 being aborted when rev3 arrives.
- [ ] **Step 4:** Commit: `test(e2e): cover Capp rollouts; docs: document traffic splitting`.

---

### Task 8: PR #746 review fixes (stage timer confirmation, deadline anchor, revision-bound actions)

Source: sbahar619's inline comments on `docs/traffic-splitting-design.md` (PR #746). The design doc has been updated to match. The third comment (use Knative's built-in rollout instead) was answered on the PR; it is now recorded under "Alternatives considered" and needs no code change.

**Files:**
- Modify: `internal/kinds/capp/rollout/rollout.go`, `rollout_test.go`
- Modify: `internal/kinds/capp/controllers/rollout.go`, `rollout_test.go`, `controller_test.go`
- Modify: `internal/kinds/capp/cappmeta/cappmeta.go` (action constants and parsing helper, if they live there)
- Modify: `internal/webhook/rcs/v1alpha1/validator.go`, `validator_test.go`
- Modify: `api/v1alpha1/capp_types.go` (doc comments for `StageStartTime` and `CandidateSince`), then `make manifests generate` and copy the CRDs to the chart
- Modify: `test/e2e/rollout_test.go` (promote uses `promote:<revision>`), `docs/user-guide.md`

**8a. Stage timer starts on confirmed traffic**
- New `rollout.Input` field `AppliedTraffic []cappv1alpha1.RevisionTraffic`. The controller fills it from `ksvc.Status.Traffic` (revisionName → percent; a missing percent counts as 0), but only when `ksvc.Status.ObservedGeneration == ksvc.Generation` and the ksvc condition `RoutesReady` is True. Otherwise it stays nil.
- `startStage(index)` sets `StageIndex` and the message, and clears `StageStartTime` (nil). It no longer stamps `Now`.
- `traffic()` must take the candidate percent from `stages[StageIndex]` once `CandidateReadyOnce` is true, not from `StageStartTime != nil`. Otherwise the split would never be applied and so never confirmed.
- In `advance`: if `StageStartTime == nil`, set it to `Now` only when `AppliedTraffic` matches `s.traffic()` for the current stage (same revision → percent map, treating absent as 0). Otherwise stay `Progressing` with the message "waiting for Knative to apply N% traffic split" and return without moving stages. The loop advances at most one stage per call. After `startStage(next)` the timer is nil again, so it waits for confirmation.
- `waitingForFirstReady` currently uses `StageStartTime == nil`. That is no longer a valid "never been ready" signal. Add an explicit status bool `CandidateReadyOnce` (json `candidateReadyOnce,omitempty`), set when the candidate is first seen Ready, and use it to decide when `progressDeadline` applies. Keep Failed→recover resuming at the current stage. On recovery, set `StageStartTime = nil` so the timer restarts after confirmation.
- A final `percent: 100` stage still promotes as soon as it is reached, with no confirmation wait. That matches the docs.
- `requeueAfter`: while waiting for confirmation, return 0 (the ksvc status watch triggers the reconcile). Add a safety requeue of 30s in case the watch event was missed.
- Tests:
  - `TestStep_StageTimerWaitsForAppliedTraffic`: no `AppliedTraffic` → `StageStartTime` is nil; matching `AppliedTraffic` → it is set to `Now`.
  - Mismatched percent → still nil.
  - Elapsed stage → advances exactly one stage with a nil timer.
  - Adjust the existing tests that assumed instant stamping or multi-stage catch-up.
  - Controller test: `AppliedTraffic` is nil when `ObservedGeneration` is stale or `RoutesReady` is not True.

**8b. progressDeadline anchored on Revision creationTimestamp**
- Change the controller's `revisionReady` to return the Revision's `CreationTimestamp` as well (zero if not found). New Input field `CandidateCreated time.Time`.
- In `detectNewRevision`: `CandidateSince = CandidateCreated` if non-zero, else `Now`. If `CandidateSince` was set from `Now` as a fallback and a later reconcile gets a non-zero `CandidateCreated` for the same candidate, replace it.
- Tests: the deadline counts from `CandidateCreated`, not `Now`, so a revision created before an operator restart that is already past the deadline fails or rolls back on the first reconcile. Also test the fallback when `CandidateCreated` is zero.

**8c. Revision-bound actions**
- `ParseAction(v) (Action, revision string, ok bool)`. Valid values: `""`, `promote:<rev>`, `abort:<rev>` with a non-empty `<rev>`. Anything else, including bare `promote`/`abort`, is invalid: emit a Warning event (`RolloutInvalidAction`) and still remove the annotation.
- New Input field `ActionRevision string`. In `handleAction`, if `ActionRevision != status.CandidateRevision`, emit a Warning with reason `PromoteRejected`/`AbortRejected` and message `"<Action> rejected: targets revision X but the current candidate is Y"`, and do nothing else.
- Annotation removal: replace the merge patch in `removeRolloutAnnotation` with a JSON patch `[{"op":"test","path":"/metadata/annotations/rcs.dana.io~1rollout","value":<processed value>},{"op":"remove","path":"/metadata/annotations/rcs.dana.io~1rollout"}]`. Pass the value read at the start of the reconcile. If the `test` op fails (apiserver 422 / Invalid), the user changed the value: don't return an error, just requeue so the new value is processed.
- Webhook: reject a Capp whose `rcs.dana.io/rollout` annotation is set but not of the form `promote:<rev>` or `abort:<rev>`. The controller-side handling stays as defence.
- Tests:
  - `ParseAction` table.
  - Step: a promote for a non-candidate revision is rejected and the status is unchanged.
  - Step: a promote for the current candidate works.
  - Controller: the JSON patch removes the annotation when the value matches and keeps it when the value changed.
  - Webhook table cases.

**Steps**
- [ ] **Step 1:** Write the failing tests for 8a–8c. Run `go test ./internal/...` and expect failures.
- [ ] **Step 2:** Implement 8a–8c. Update the `capp_types.go` doc comments, then run `make manifests generate` and copy both CRDs to `charts/container-app-operator/crds/`.
- [ ] **Step 3:** Update the e2e promote spec to `promote:<new revision name>`. Update `docs/user-guide.md`: annotation format, the stage timer starting once Knative serves the split, and the deadline being measured from revision creation.
- [ ] **Step 4:** `go build ./... && go vet ./... && make test` all pass.
- [ ] **Step 5:** Opus review of the Task 8 diff, Haiku fixes, then amend into the single branch commit (`git commit --amend`). Do not push.

---

### Task 9: PR #746 second review round

Source: two further rounds of inline comments from sbahar619 on PR #746 (7 comments on 2026-10-25, 7 more on 2026-10-26). The design doc has been updated (validation, Manual control, Status/conditions, Lifecycle, the "new revision during a rollout" decision, Edge cases, Decisions to review). Implement on top of Task 8.

**Files:**
- Modify: `internal/kinds/capp/rollout/rollout.go`, `rollout_test.go`
- Modify: `internal/kinds/capp/controllers/controller.go` (Revision watch), `rollout.go`, tests
- Modify: `internal/kinds/capp/status/rollout.go`, `capp.go`, tests (condition rework)
- Modify: `internal/kinds/capp/resourcemanagers/ksvc.go`, `ksvc_test.go` (stable pin without strategy)
- Modify: `internal/webhook/rcs/v1alpha1/validator.go`, `validator_test.go`
- Modify: `api/v1alpha1/capp_types.go` (condition constant, `CandidateReadyOnce` if Task 8 hasn't added it, doc comments), then `make manifests generate` and copy the CRDs to the chart
- Modify: `test/e2e/rollout_test.go`, `docs/user-guide.md`

**9a. Stage validation (webhook)**, in addition to the existing rules:
- percents strictly increasing
- the last stage must NOT have `duration`. This replaces Task 8's "ignored for a final 100% stage"; update that doc comment and the user guide line.

Table cases: decreasing, equal, 100 not last (covered by strictly increasing), final duration on a 100% stage, final duration on a 50% stage, valid canary, `[{0}]`, `[{50}]`.

**9b. `Ready` unchanged; new `Available` and `RolloutSucceeded` conditions**
- `Ready` keeps its existing meaning. Remove the rollout condition from the `Ready` summary, and do NOT change `checkKnative`.
- `RolloutSucceeded` (set on every Capp with `status.rollout`, outside the `Ready` summary):

  | Status | When |
  |---|---|
  | True | phase Stable, `stableRevision == ksvc LatestCreatedRevisionName`, and Knative has confirmed 100% to it (same check as 8a; for a Capp without a strategy, ksvc `status.traffic` gives that revision 100% and `RoutesReady=True`) |
  | Unknown | Progressing / Holding / Promoting; the reason is the phase |
  | False | Failed / RolledBack / Aborted; the reason is the phase |

- `Available` (every Capp, outside the `Ready` summary): read the ksvc. It is True when:
  - `RoutesReady=True`
  - `status.traffic` is non-empty
  - every revision in `status.traffic` with percent > 0 has a Revision `Ready=True`

  It is False with a reason naming the revision when a target with percent > 0 is not Ready, or when `RoutesReady=False`. It is Unknown when `RoutesReady` is Unknown or there is no ksvc. Ignore 0% targets.
- Tests:
  - after a rollback, `Available=True` while `Ready` (via `checkKnative`) can be False
  - a 0% target that is not Ready does not affect `Available`
  - `RolloutSucceeded` is Unknown while Promoting
- Look at how `capp.go` builds the conditions list and the `Ready` summary, and keep both new conditions out of the summary.
- Rename or replace the existing `RolloutReady` code and tests.

**9c. Retry action**
- `ParseAction` accepts `retry:<rev>`.
- In `handleAction`, retry is allowed when `ActionRevision == CandidateRevision` and the phase is in {Failed, RolledBack, Aborted}. It sets:
  - Phase Progressing
  - StageIndex 0
  - StageStartTime nil
  - CandidateReadyOnce false
  - CandidateSince = Now
  - Message, and a Normal event `RolloutRetried`

  Otherwise it is rejected with a Warning `RetryRejected`.
- After retry, `detectNewRevision` must not treat the candidate as new. It isn't new: it's the same name.
- Webhook: accept `retry:<rev>`.

**9d. Failure semantics with autoRollback: false.** Verify against the design and adjust:
- Failed → traffic frozen at the current stage's split (candidate omitted if never Ready), stage timer cleared (`StageStartTime = nil`).
- Recovery → Progressing at the same stage, with the timer restarted after confirmation (Task 8a).

**9e. Unready candidate omitted from traffic**
- In `traffic()`, while there is an active candidate and `!CandidateReadyOnce`, return `[stable:100]` only, with no candidate target.
- Test: rev2 at 50% → rev3 created and not ready → traffic is `[rev1:100]`.

**9f. Strategy change mid-rollout**
- In `Step`, before advancing: if there is an active candidate with `CandidateReadyOnce`, and `served` (the candidate percent in `Previous.Traffic`) is not equal to `stages[StageIndex].Percent`, or `StageIndex > last`:
  - set `StageIndex` = the last index i with `stages[i].Percent <= served`
  - if there is no such index (every new stage is above `served`): keep serving `served`. Add a status field `HeldPercent *int64`, which `traffic()` uses instead of the stage percent when set. Set the phase to Holding, set `StageIndex` to 0, and emit the Warning `RolloutStagesChanged`. This never promotes and never raises traffic. A `promote`, `abort` or `retry` clears `HeldPercent`.
  - set `StageStartTime = nil`
  - emit a Normal event `RolloutStagesChanged`
- If the percent at the index is unchanged, keep the index and the timer.
- Replace the old "cap at last stage" logic.
- Tests:
  - `[10,50,100]` at idx 1 (50) → `[10,100]` lands on idx 0 (10)
  - a duration-only change keeps the index and timer
  - a shorter list with the same percent keeps the index
  - at 10%, `[50,100]` → holds at 10% (Holding, `HeldPercent=10`)
  - at 10%, `[100]` → holds at 10%, NOT promoted

**9g. Strategy removal never resurrects a rejected candidate**
- In `stepWithoutStrategy`:
  - If the previous phase is Progressing or Holding and the candidate is Ready (`CandidateReady == True`), promote it. This is the current behaviour.
  - If the candidate is not Ready, or the phase is Failed, RolledBack or Aborted, set the new status field `PinnedRevision = StableRevision`, and remember the rejected candidate in `CandidateRevision`. Emit a Warning event `RolloutPinned`.
  - Keep the pin until a NEWER revision is Ready: `LatestReady == LatestCreated`, and `LatestCreated` is neither the rejected candidate nor stable. A newer revision that is only created is not enough.
  - Then clear the pin and go back to the existing path (stable = LatestReady).
- `ksvc.go buildTraffic`: with no strategy but a pin set, return `[{revisionName: pinned, percent: 100, tag: stable}]`. Otherwise keep it unset.
- Tests:
  - a removed strategy with a RolledBack candidate gets a pin
  - a removed strategy with a Ready Progressing candidate is promoted
  - the pin stays while a newer revision is created but not Ready (LatestReady is still the rejected rev2)
  - the pin clears once the newer revision is Ready
  - ksvc traffic with the pin

**9h. Revision watch**
- In `SetupWithManager`, watch `knativev1.Revision`, mapping via the label `serving.knative.dev/service` to the Capp with the same name and namespace.
- Predicate: only the Ready condition status changed, or Create/Delete.
- Unit-test the mapping and the predicate in the style of the existing watch-predicate tests (if any). Otherwise just test the map func.

**9i. `Promoting` phase: confirm 100% before marking stable**
- New phase `RolloutPhasePromoting`. A final 100% stage, a manual promote, and a strategy-removal promote all set Phase Promoting, and `traffic()` returns `[stable:0, candidate:100]`.
- Only when `AppliedTraffic` (8a) matches that split does the status become stable=candidate, Phase Stable, and the event `ReasonCompleted` is emitted.
- Health checks still apply while Promoting (rollback / Failed as usual).
- Requeue: the ksvc watch, plus the 30s safety requeue.
- Without a strategy, the removal-promote waits the same way. With no strategy, ksvc traffic is unset (Knative default), so confirm with: ksvc `status.traffic` gives the candidate 100% and `RoutesReady=True`.
- Tests:
  - stays Promoting until confirmed
  - Stable after confirmation
  - manual promote → Promoting
  - Ready=False during Promoting → rollback

**9j. Stage duration is a minimum wait.** No code change beyond 8a. Make sure the doc comments on `StageStartTime` and the stage `Duration`, and the user guide, say that the duration is a minimum counted from when the operator observes Knative's confirmation, and that operator downtime can lengthen a stage.

**Steps**
- [ ] **Step 1:** Write the failing tests for 9a–9j. Run `go test ./internal/...` and expect failures.
- [ ] **Step 2:** Implement 9a–9j. Run `make manifests generate` and copy the CRDs to the chart.
- [ ] **Step 3:** e2e:
  - The auto-rollback spec now asserts `Available=True` and `RolloutSucceeded=False`, and does NOT assert anything about `Ready`.
  - The canary spec waits on `RolloutSucceeded=True`.
  - Add a retry spec, and a strategy-removal-after-rollback spec (traffic stays on stable).
- [ ] **Step 4:** Update the user guide:
  - the validation rules
  - retry
  - `RolloutSucceeded` and what CI should wait on
  - strategy removal and strategy change behaviour
  - the 100% capacity note for the new-revision restart
- [ ] **Step 5:** `go build ./... && go vet ./... && make test` all pass.
- [ ] **Step 6:** Opus review of the combined Task 8+9 diff, Haiku fixes, then amend into the single branch commit. Do not push.
