# Design: Traffic Splitting & Staged Rollouts for Capp

- **Date:** 2026-10-21
- **Status:** Draft — open for team review
- **Author:** @dvirgilad

## Summary

Add a `rolloutStrategy` to `Capp.spec.routeSpec` so that users can run canary
releases, steady traffic splits and blue-green deployments. The user describes
the rollout once as a list of stages. Each time a new revision appears, the
operator moves traffic through those stages on its own. Rules point at roles
(**stable** and **candidate**), never at revision names, so the same strategy
works for every deploy.

Capps without a strategy behave exactly as they do today: 100% of traffic goes
to the latest revision.

## Background

Today the Knative Service built in
`internal/kinds/capp/resourcemanagers/ksvc.go` only calls
`RouteSpec.SetDefaults`, so every Capp sends 100% of traffic to its latest
revision.

`routeSpec.trafficTarget` is already in the API, and `docs/user-guide.md`
describes it as "advanced traffic routing for canary/A/B testing". It is never
copied into the Knative Service, though, so it has no effect. It is also a
single entry, so it couldn't describe a split even if it were wired up.

As a result, users have no way to do canary or blue-green releases.

## Goals

- No change for Capps that don't set a strategy (100% to latest).
- **Canary:** explicit stages, e.g. 10% for 5m → 50% for 10m → 100%.
- **Steady split:** e.g. hold 50/50 between stable and candidate indefinitely.
- **Blue-green:** the candidate gets 0% of traffic, can be tested on its own
  tag URL, and gets 100% at once when promoted.
- The operator moves traffic from stage to stage itself; users don't edit the
  Capp for each stage.
- Opt-in automatic rollback when the candidate revision isn't Ready.
- Manual abort and promote through an annotation.
- Rollout progress visible in Capp status and in events.

## Non-goals

- Analysis based on metrics such as error rate or latency. Whether the
  candidate revision is Ready is the only health signal.
- Splitting traffic across more than two revisions at once.
- Routing by header or cookie.
- Starting rollouts for changes that don't create a new Knative revision.
- Tag URLs on the Capp's custom hostname.

## Concepts

| Term | Meaning |
|---|---|
| **Stable revision** | The last revision that completed a rollout (reached 100%). Stored in Capp status. This is *not* simply the revision before the latest one. |
| **Candidate revision** | The latest created Knative revision (`ksvc.Status.LatestCreatedRevisionName`), while it differs from stable. |
| **Stage** | `{percent, duration}`: the candidate gets `percent` of traffic and stable gets `100 - percent`, for `duration`. |

## API

Under `routeSpec` (field names are indicative):

```yaml
routeSpec:
  rolloutStrategy:
    stages:
      - percent: 10
        duration: 5m
      - percent: 50
        duration: 10m
      - percent: 100
    autoRollback: true          # opt-in, default false
    progressDeadline: 10m       # max time for the candidate to become Ready
```

Validation (CRD CEL rules and/or the validating webhook in
`internal/webhook/rcs/v1alpha1/validator.go`):

- `stages` is not empty, and each `percent` is between 0 and 100.
- Every stage except the last must have a `duration`.
- If the last stage has no `duration`, traffic **holds** at that percent
  indefinitely (a steady split, or blue-green waiting at 0%).
- If the last stage is `percent: 100`, the candidate becomes stable when that
  stage is reached.

### Patterns

| Pattern | Stages |
|---|---|
| Canary | `[{10, 5m}, {50, 10m}, {100}]` |
| Steady 50/50 | `[{50}]` |
| Blue-green | `[{0}]`, followed by a manual `promote` |

### `routeSpec.trafficTarget`

Marked deprecated. It stays in the CRD for compatibility, stays ignored, and is
removed from the user guide.

### Manual control

Users set an annotation on the Capp. The operator acts on it and then removes
it:

- `rcs.dana.io/rollout: promote`: skip the remaining stages. The candidate goes
  to 100% and becomes stable.
- `rcs.dana.io/rollout: abort`: send 100% to stable and set the phase to
  `Aborted`.

`ksvc.go` already keeps `rcs.dana.io`-prefixed annotations off the Knative
Service, so this annotation won't leak through.

## Rollout behaviour

### Status

A new status block holds:

- `stableRevision`, `candidateRevision`
- `stageIndex`, `stageStartTime`
- `phase`: `Stable | Progressing | Holding | RolledBack | Aborted`
- the traffic currently applied (percent per revision)
- a rollout condition that feeds the Capp's `Ready` summary, the same way other
  capability conditions do (`CappConditionServingReady` etc.)

**Effect on `Ready`:** `Ready` stays **True** while the phase is `Progressing`
or `Holding`. It goes **False** only when the candidate has failed (with
`autoRollback: false`), or when the phase is `RolledBack` or `Aborted`. This
way, CI and tooling that wait on `Ready` don't hang during a normal canary.

All rollout state lives in status, so it survives operator restarts. Elapsed
time is worked out from `stageStartTime`, never from in-memory timers.

### Lifecycle

The existing Capp reconcile drives the rollout. Changes to the Knative Service
status already trigger a reconcile through `knativeServiceWatchPredicate`.

1. **First revision ever** (no stable yet): it gets 100% of traffic and is
   recorded as stable once it's Ready. No stages run.
2. **New revision detected** (`LatestCreatedRevisionName` is neither stable nor
   candidate): it becomes the candidate, at stage 0, with the phase set to
   `Progressing`. Stable keeps 100% until the operator applies the stage's
   traffic.
3. **Waiting for Ready:** stage timers don't start until the candidate is Ready.
   If the candidate isn't Ready within `progressDeadline`, or reports
   Ready=False:
   - with `autoRollback: true`, traffic goes 100% to stable, the phase becomes
     `RolledBack`, and a Warning event is emitted.
   - with `autoRollback: false`, traffic stays where it is, the condition
     reports the failure, and a Warning event is emitted. The user acts through
     the annotation.
4. **Moving through stages:** the operator sets Knative Service traffic to
   `[stable: 100-p, candidate: p]` and requeues with `RequeueAfter` set to the
   time left in the stage. When that time is up, it moves to the next stage.
5. **Completion:** when a final 100% stage is reached, the candidate becomes
   stable and the phase becomes `Stable`. When the final stage has no duration
   and is below 100%, the phase becomes `Holding`.

### Traffic targets

While a strategy is active, traffic targets always name revisions explicitly
(`revisionName`), never `latestRevision: true`. Otherwise a new revision would
take over the candidate's share before the operator could react.

Targets carry the Knative tags `stable` and `candidate`, so each one can be
reached on its own tag URL. Blue-green relies on this to test a candidate at
0%. Tag URLs exist only on the Knative default cluster domain. No extra
DomainMapping, DNS record or Certificate is created.

### Decision for review: a new revision arrives during a rollout

If revision 3 appears while a rollout from rev1 to rev2 is mid-stage or
holding, the operator **aborts** the rev1 → rev2 rollout and **starts a new
rollout from rev1 to rev3** at stage 0. Rev2 never becomes stable, and its
traffic stops right away. An event is emitted, and the user guide documents
this behaviour.

Other options we considered:

- *Queue rev3* until rev2 finishes. This is slower to react, and an urgent fix
  could end up waiting behind a slow canary.
- *Promote rev2 to stable first.* This treats a revision that was only partly
  validated as stable.

What this means for steady splits: a 50/50 split is always between **stable**
and the **latest** revision, not between "the two latest revisions".

### Edge cases

- **A strategy is added to an existing Capp:** the revision serving 100% at
  that moment is treated as stable. `stableRevision` is recorded for every Capp,
  with or without a strategy, so adopting one later is always well defined. A
  template change made in the same apply goes through the stages.
- **A strategy is removed mid-rollout:** this works like `promote`. The
  candidate (which is the latest revision) goes to 100% and becomes stable, and
  the Capp then behaves as it does today.
- **The strategy is changed mid-rollout:** the current stage index carries over
  to the new stage list, capped at its last stage. The rollout doesn't restart.
- **What `autoRollback` covers:** it also triggers when a candidate that was
  Ready goes Ready=False mid-stage or while holding.
- **Promote safety:** `promote` is refused (with a Warning event, and the
  annotation is removed) if the candidate isn't Ready or the phase is
  `RolledBack` or `Aborted`.
- **The spec no longer matches what's served:** after a rollback or abort, the
  Capp's template no longer matches the revision getting traffic. The rollout
  condition says so until the user changes the template. The spec itself is
  left alone.
- **Disable and re-enable:** `state: disabled` deletes the Knative Service. On
  re-enable, rollout state resets, and the next revision follows the "first
  revision ever" path.

## Implementation notes

- The Knative Service is rebuilt from the Capp spec on every reconcile, so the
  operator fully owns `ksvc.Spec.Traffic`. Traffic is worked out from the Capp
  spec and the Capp status.
- After `make manifests`, copy the CRD to `charts/container-app-operator/crds/`.
- Open implementation choices:
  - where the stepping logic lives: in `KnativeServiceManager`, or in a helper
    it calls
  - checking readiness through `LatestReadyRevisionName`, or by reading the
    Revision object
  - the default value of `progressDeadline`
  - how the tests are laid out (unit tests for the stage state machine, e2e
    tests for a canary run)

## Documentation

`docs/user-guide.md` gets a new "Traffic splitting & rollouts" section. It
covers the three patterns, the annotations, the abort-on-new-revision
behaviour, and the deprecation of `trafficTarget`.

## Decisions to review

1. A new revision during a rollout aborts it and restarts against stable.
2. `Ready` stays True during `Progressing` and `Holding`.
3. Removing a strategy mid-rollout works like `promote`.
4. `stableRevision` is tracked for every Capp, even without a strategy.
5. `trafficTarget` is deprecated and stays ignored rather than being removed.
6. Tag URLs exist only on the Knative default domain.
