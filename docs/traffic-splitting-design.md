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
- Percents are **strictly increasing** from stage to stage. A repeated percent
  adds nothing that a longer `duration` can't express, and a decreasing one
  would shift traffic back to stable mid-rollout. This also means `100` can
  only be the last stage.
- Every stage except the last **must** have a `duration`, and it must be
  positive.
- The last stage **must not** have a `duration`. What happens there is decided
  by its percent:
  - `percent: 100`: the candidate becomes stable as soon as the stage is
    reached.
  - below 100: traffic **holds** at that percent until the user promotes or
    aborts (a steady split, or blue-green waiting at 0%).

  A final stage such as `{percent: 50, duration: 5m}` is rejected. Its meaning
  would be ambiguous (hold, promote, or roll back after 5 minutes), and both
  intended outcomes can already be written explicitly:
  - `[{50, 5m}, {100}]` to promote after 5 minutes
  - `[{50}]` to hold
- `progressDeadline`, if set, must be positive.

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

Users set an annotation on the Capp. The value names the action and the
candidate revision it is meant for. The operator acts on it and then removes
it:

- `rcs.dana.io/rollout: promote:<revision>`: skip the remaining stages. The
  candidate goes to 100% (`Promoting`) and becomes stable once Knative confirms
  the split.
- `rcs.dana.io/rollout: abort:<revision>`: send 100% to stable and set the
  phase to `Aborted`.
- `rcs.dana.io/rollout: retry:<revision>`: restart the rollout of the same
  candidate from stage 0. This is allowed only when the phase is `Failed`,
  `RolledBack` or `Aborted`. A retry resets:
  - the phase to `Progressing`
  - `candidateSince` to the retry time, so the candidate gets a full
    `progressDeadline` again
  - the "has been Ready" flag

  Use it when a candidate failed for a temporary reason, such as a dependency
  outage, and is healthy again. No template change is needed. A retry is
  rejected with a Warning event in any other phase.

Binding the action to a revision keeps it from hitting the wrong rollout. If
`<revision>` isn't the current `candidateRevision`, the action is rejected with
a Warning event and the annotation is removed. For example, a `promote` meant
for rev2 is rejected when rev3 has already replaced rev2 as the candidate. This
also covers the case where the operator applies an action and then crashes
before removing the annotation: after a restart the same action is replayed,
but it can only match the same candidate, and replaying it there changes
nothing. A value without `:<revision>`, or with an unknown action, is rejected
as invalid.

The operator removes the annotation with a JSON patch that first `test`s the
value it processed and then `remove`s it. If the user changed the annotation in
the meantime, the patch fails, the new value is kept, and it is handled on the
next reconcile.

`ksvc.go` already keeps `rcs.dana.io`-prefixed annotations off the Knative
Service, so this annotation won't leak through.

## Rollout behaviour

### Status

A new status block holds:

- `stableRevision`, `candidateRevision`
- `stageIndex`, `stageStartTime` (the time Knative confirmed that the stage's
  traffic split is serving; unset until then)
- `candidateSince`: the candidate Revision's `metadata.creationTimestamp`, the
  start time for `progressDeadline`
- `candidateReadyOnce`: whether the candidate has been Ready at least once.
  `progressDeadline` applies only while this is false.
- `phase`: `Stable | Progressing | Holding | Promoting | Failed | RolledBack |
  Aborted`. `Promoting` means the candidate has been sent 100% of traffic and
  the operator is waiting for Knative to confirm that split (see Lifecycle
  step 5).
- the traffic currently applied (percent per revision)

**Conditions.** `Ready` keeps its current meaning. Two new condition types sit
next to it, and neither feeds into `Ready`:

- `Ready`: **unchanged**. It is still the summary of the capability conditions,
  including the existing Knative check, which looks at the latest revision and
  at the Knative Service's overall readiness. So a failed latest revision makes
  `Ready` False even after a rollback, exactly as it does today without a
  strategy. The rollout condition does **not** feed into `Ready`. Tooling that
  already depends on `Ready` sees the same contract as before.
- `Available` (new): **the revisions actually serving traffic can serve it.**
  It is True when all of these hold:
  - the Knative Service has `RoutesReady=True`
  - `status.traffic` isn't empty
  - every revision in `status.traffic` with a percent above 0 has `Ready=True`

  It is False with a reason if any target with a percent above 0 isn't Ready,
  or if `RoutesReady=False`. It is Unknown while routing is being updated. It
  ignores revisions that serve no traffic, so after a rollback, with stable
  serving 100% normally, `Available` is True even though the failed candidate
  makes `Ready` False. Every Capp reports `Available`, with or without a
  strategy.
- `RolloutSucceeded` (new): **whether the latest revision has fully rolled
  out.**

  | Status | When |
  |---|---|
  | `True` | Phase `Stable`, `stableRevision == LatestCreatedRevisionName`, and Knative has confirmed that revision serves 100%. Capps without a strategy report the same, so there is one signal for every Capp. |
  | `Unknown` | Phase `Progressing`, `Holding` or `Promoting`. The reason names the phase, and the message names the stage. |
  | `False` | Phase `Failed`, `RolledBack` or `Aborted`. |

**What CI should wait for:**
- A deploy pipeline waits for `RolloutSucceeded=True`, e.g.
  `kubectl wait capp/<name> --for=condition=RolloutSucceeded`, and fails on
  False.
- Availability monitoring and alerting use `Available`.
- A blue-green Capp stays `Unknown` (Holding) until it is promoted. Its pipeline
  should wait after the promote step, not after the deploy.

The user guide documents all of this.

All rollout state lives in status, so it survives operator restarts. Elapsed
time is worked out from status timestamps, never from in-memory timers. The two
timestamps behave differently on a restart:
- **`candidateSince`** comes from the cluster (the Revision's
  `creationTimestamp`). Operator downtime or a failed status write can't extend
  `progressDeadline`.
- **`stageStartTime`** is the time the operator *observed* Knative's
  confirmation of the split. If the split is confirmed while the operator is
  down, or the status write that records it fails, the timer starts later, when
  the operator does observe it. A stage's `duration` is therefore a **minimum**
  time the candidate serves that percent after the operator has seen the split
  confirmed, not an exact time. Downtime can make a stage longer, never
  shorter.

### Lifecycle

The existing Capp reconcile drives the rollout. Changes to the Knative Service
status already trigger a reconcile through `knativeServiceWatchPredicate`.

**Candidate health** is always read from the **candidate Revision object's own
`Ready` condition** on each reconcile. `LatestReadyRevisionName` only says that
a revision *became* ready at some point. It stays set if the revision later
fails, so it can't drive a rollback. A failure after the first Ready (e.g.
crash-looping pods at 50%) would go unnoticed until something else triggered a
reconcile. To catch it right away, the operator also **watches Knative
Revisions**: Revisions labelled `serving.knative.dev/service=<capp>` map to
their Capp, with a predicate on changes to the `Ready` condition status.

1. **First revision ever** (no stable yet): it gets 100% of traffic and is
   recorded as stable once it's Ready. No stages run.
2. **New revision detected** (`LatestCreatedRevisionName` is neither stable nor
   candidate): it becomes the candidate, at stage 0, with the phase set to
   `Progressing`. Until it has been Ready once, the candidate is **left out of
   the traffic targets** completely, and stable is the only target at 100%.
   This matters because Knative won't program a route that targets a revision
   that isn't Ready. Even a 0% target to it would leave the *previous* split
   serving until the revision became Ready.
3. **Waiting for Ready:** stage timers don't start until the candidate is Ready.
   `progressDeadline` is measured from the candidate Revision's
   `creationTimestamp` (recorded as `candidateSince`). If the operator can't
   read the Revision, it falls back to the time it first saw the revision. If
   the candidate isn't Ready within `progressDeadline`, or reports Ready=False:
   - with `autoRollback: true`, traffic goes 100% to stable, the phase becomes
     `RolledBack`, and a Warning event is emitted.
   - with `autoRollback: false`, the phase becomes `Failed`, a Warning event
     is emitted, and **stage advancement stops**:
     - traffic stays at the current stage's split (0% if the candidate never
       became Ready)
     - the stage timer is cleared

     If the candidate becomes Ready again, the rollout resumes at the **same
     stage**, and that stage's timer starts again from the full duration once
     Knative confirms the split. Otherwise the user aborts it. `promote` is
     refused while the candidate is not Ready.

   The same applies when a candidate that was Ready goes Ready=False mid-stage
   or while holding. In both cases the user can retry the rollout of the same
   revision later with `retry:<revision>`.
4. **Moving through stages:** the operator sets Knative Service traffic to
   `[stable: 100-p, candidate: p]`. The stage timer (`stageStartTime`) starts
   only once Knative confirms that split is serving, which means all of these
   hold:
   - `status.observedGeneration == metadata.generation`
   - `RoutesReady=True`
   - `status.traffic` gives each revision the stage's percent

   Routing delays therefore don't count toward the stage. "10% for 5m" means
   at least 5 minutes of serving 10%, counted from when the operator observes
   the confirmation (see Status). The Knative Service status change
   triggers a reconcile, which sees the confirmation. Once the timer is
   running, the operator requeues with `RequeueAfter` set to the time left in
   the stage. When that time is up, it moves to the next stage, and that
   stage's timer again waits for confirmation. As a result the operator
   advances at most one stage per confirmed split, including after a restart.
5. **Completion:** reaching a final 100% stage, or a manual `promote`, sets
   the phase to `Promoting` and the traffic to `[stable: 0, candidate: 100]`.
   The candidate becomes stable, and the phase becomes `Stable`, **only once
   Knative confirms** that the 100% split is serving. Confirmation uses the
   same check as step 4. Until then:
   - `RolloutSucceeded` stays Unknown, so success is never reported while
     traffic is still split or while the routing update has failed.
   - Candidate health checks still apply. If the candidate goes Ready=False,
     it is rolled back or marked `Failed` as in step 3.

   A promote from removing the strategy follows the same rule. When the final
   stage is below 100%, the phase becomes `Holding` once its split is
   confirmed.

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

The transition, spelled out for a 50/50 split of stable rev1 and candidate
rev2:

1. Rev3 is created and isn't Ready yet. The traffic targets become
   `[rev1: 100%]`, and rev3 is left out (see Lifecycle step 2). **Rev1 serves
   100% until rev3 is Ready.** Rev2 gets no traffic.
2. Rev3 becomes Ready. Traffic moves to `[rev1: 100-p, rev3: p]` for the first
   stage, and that stage's timer starts once Knative confirms the split.
3. If rev3 never becomes Ready, `progressDeadline` applies as usual, and rev1
   keeps 100%.

Capacity: rev1 goes from 50% to 100% of traffic in one step. Knative's
autoscaler scales rev1 up, but a cold burst is possible, so stable should keep
enough `minScale` headroom. The user guide documents this.

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
- **A strategy is removed:** removal never sends traffic to a revision that
  `promote` would refuse. The promote safety rules still apply:
  - If the phase is `Progressing` or `Holding` and the candidate is **Ready**,
    removal works like `promote`. The candidate goes to 100% and becomes
    stable, and the Capp then behaves as it does today.
  - If the candidate is **not Ready** (`Failed`, or still waiting for its first
    Ready), or the phase is `RolledBack` or `Aborted`, the candidate is
    **rejected**, and traffic stays pinned to stable (`[stable: 100%]` by
    `revisionName`). The pin is dropped only once a **newer revision is
    Ready**: `LatestReadyRevisionName == LatestCreatedRevisionName`, and that
    revision is neither the rejected candidate nor stable. A newer revision
    that has only been *created* is not enough. A rejected revision can still
    be Ready (for example after a manual abort), so Knative's default routing
    would send traffic back to it while the newer revision is starting. Once
    the newer revision is Ready, the Capp goes back to Knative's default
    routing (100% to the latest Ready revision). Without this pin, Knative
    could send 100% to a rolled-back revision that had been Ready once. A
    Warning event explains the pin.
  - This is the only case where a Capp without a strategy sets
    `ksvc.Spec.Traffic`.
- **The strategy is changed mid-rollout:** a change never **raises** the
  candidate's traffic.
  - If the stage at the current index has the same percent as what is being
    served, the index and its timer carry over. This covers changes to
    durations only.
  - Otherwise the rollout moves to the **last stage whose percent is at most
    the percent being served**. That stage's timer starts again once Knative
    confirms its split.
  - If **every** new stage is above the percent being served, the candidate
    **stays at its current percent**, the phase becomes `Holding`, and a
    Warning event names the percent being held. The rollout then waits for the
    user, who can:
    - `promote` it
    - `abort` it
    - `retry` it, which restarts from the new stage 0 as an explicit choice

    Nothing raises traffic on its own.
  - Examples:
    - at the 50% stage of `[10, 50, 100]`, changing to `[10, 100]` moves to the
      10% stage. It does not jump to 100%.
    - at 10%, changing to `[50, 100]` or to `[100]` holds at 10%. It neither
      jumps to 50% nor promotes.
  - These rules apply only once the candidate has been Ready, because before
    that it serves 0%.
- **What `autoRollback` covers:** it also triggers when a candidate that was
  Ready goes Ready=False mid-stage or while holding.
- **Promote safety:** `promote` is refused (with a Warning event, and the
  annotation is removed) if the candidate isn't Ready or the phase is
  `RolledBack` or `Aborted`. To give a rolled-back or aborted candidate another
  chance, use `retry`, which runs the stages again.
- **The spec no longer matches what's served:** after a rollback or abort, the
  Capp's template no longer matches the revision getting traffic. The rollout
  condition says so until the user changes the template. The spec itself is
  left alone.
- **Disable and re-enable:** `state: disabled` deletes the Knative Service. On
  re-enable, rollout state resets, and the next revision follows the "first
  revision ever" path.

## Alternatives considered

**Expose Knative's traffic splitting and `rollout-duration` directly.**

Knative already provides the building blocks. With explicit traffic targets, a
user can:
- keep a new revision at 0%
- test it through a tag URL
- move traffic to it by editing the targets by hand

So blue-green is *possible* with plain Knative. What it doesn't provide is the
**workflow**: a rollout that runs on its own for every new revision. Knative's
gradual rollout (`rollout-duration`) covers only part of that:
- it applies only to `latestRevision: true` targets
- it shifts traffic linearly over a single duration
- it has no stages that hold at a percent

What Capp adds on top of Knative:
- **Automated staged rollouts** described once as roles (stable and
  candidate), not as revision names, and applied to every new revision
  without anyone editing traffic targets.
- **`promote`, `abort` and `retry` commands** bound to a candidate revision,
  instead of hand edits to the traffic targets.
- **Health-based rollback** when the candidate fails.
- **Rollout status and conditions** (`RolloutSucceeded`, `Available`) that CI
  can wait on.

We discussed this and concluded that this workflow is worth owning in Capp.

## Implementation notes

- The Knative Service is rebuilt from the Capp spec on every reconcile, so the
  operator fully owns `ksvc.Spec.Traffic`. Traffic is worked out from the Capp
  spec and the Capp status.
- After `make manifests`, copy the CRD to `charts/container-app-operator/crds/`.
- Open implementation choices:
  - where the stepping logic lives: in `KnativeServiceManager`, or in a helper
    it calls
  - the default value of `progressDeadline`
  - how the tests are laid out (unit tests for the stage state machine, e2e
    tests for a canary run)
- Settled: candidate readiness comes from the Revision object's `Ready`
  condition, plus a watch on Revisions (see Lifecycle). It doesn't come from
  `LatestReadyRevisionName`.
- Settled: the existing `checkKnative` and the `Ready` summary are not changed.
  `Available` is a new check that reads the Knative Service's `status.traffic`
  and the `Ready` condition of each Revision serving traffic.

## Documentation

`docs/user-guide.md` gets a new "Traffic splitting & rollouts" section. It
covers:
- the three patterns
- the annotations (`promote`, `abort` and `retry`, each with a revision)
- the abort-on-new-revision behaviour and its capacity note
- which condition CI should wait on (`RolloutSucceeded`), what `Available`
  means, and that `Ready` is unchanged
- that a stage `duration` is a minimum wait
- what happens when a strategy is removed or changed
- the deprecation of `trafficTarget`

## Decisions to review

1. A new revision during a rollout aborts it and restarts against stable.
   Stable serves 100% until the new candidate is Ready.
2. `Ready` keeps its existing meaning. Two new conditions sit next to it and
   neither feeds into `Ready`: `Available` (the revisions actually serving can
   serve) and `RolloutSucceeded` (the rollout outcome, which CI waits on).
3. Removing a strategy works like `promote` only if the candidate is Ready.
   Otherwise traffic stays pinned to stable until a newer revision is Ready.
4. Changing the strategy mid-rollout never raises the candidate's traffic. If
   every new stage is above the current percent, the candidate holds at that
   percent.
5. A rollout is marked stable, and `RolloutSucceeded=True` is set, only after
   Knative confirms the 100% split (the `Promoting` phase).
6. A stage's duration is a minimum time from when the operator observes
   Knative's confirmation of the split. Operator downtime can extend a stage.
7. A final stage can't have a duration, and percents must strictly increase.
8. `retry:<revision>` restarts a failed, rolled-back or aborted rollout of the
   same revision.
9. `stableRevision` is tracked for every Capp, even without a strategy.
10. `trafficTarget` is deprecated and stays ignored rather than being removed.
11. Tag URLs exist only on the Knative default domain.
