# Build-from-Source for `Capp` using `Shipwright` — Phase 7 (Low-Level Plan)

## Scope
Phase 7 adds **Shipwright `BuildRun` creation** to the `CappBuild` reconciliation workflow:
- Create a `BuildRun` that references the already-reconciled Shipwright `Build`.
- Track `BuildRun` progress and outcome and reflect it in `CappBuild.status`:
  - `status.lastBuildRunRef`
  - `status.latestImage`
  - `status.conditions` (build execution result)

This phase assumes the `Build` reconciliation from earlier phases already exists and is reliable.

## Goals
- Create `BuildRun` **idempotently** using the existing associated `Build` as the ref.
- Make build execution observable via **stable, low-cardinality** `CappBuild.status.conditions`.
- Persist the build artifact reference (`status.latestImage`) on success.

## Non-goals
- Rebuild triggers (e.g., on-commit/webhooks) and retention policies.
- Updating the target `Capp` image on successful build (handover to runtime).
- Surfacing all Shipwright/Tekton details in `CappBuild.status` (we keep the operator contract stable).

## Deliverables

### 1) Reconcile flow: ensure a `BuildRun` exists (after `Build` exists)
Add a controller step after successful `Build` reconcile:
- Resolve the associated `Build` (deterministic name + controller ownership).
- Ensure a `BuildRun` exists that references that `Build`:
  - `BuildRun.spec.build.name = <build.name>`
  - `BuildRun.namespace = cappBuild.namespace`
  - `BuildRun.ownerReferences`: controller owner = `CappBuild` (so garbage collection is deterministic)

**Association rules**
- `BuildRun` lives in the same namespace as the `CappBuild`.
- `BuildRun` name is deterministic per `CappBuild` generation:
  - Recommended: `<cappBuild.name>-buildrun-<cappBuild.generation>`
  - Rationale: allows creating a new run when spec changes, while staying idempotent per generation.
- The controller is the only writer for `status.lastBuildRunRef`.

**Idempotency rules**
- If `status.lastBuildRunRef` points to an existing `BuildRun` for the current `generation`, do not create a new one.
- If the expected `BuildRun` exists but is not owned by the `CappBuild`, treat it as a conflict and stop (no requeue).

### 2) Controller wiring + RBAC for `BuildRun`
Update controller setup and permissions:
- `SetupWithManager`: also `Owns(&shipwright.BuildRun{})` so reconcile reacts to `BuildRun` status changes.
- Add `+kubebuilder:rbac` markers for `shipwright.io` `buildruns`:
  - `get;list;watch;create;update;patch;delete`

### 3) Status updates
Update `CappBuild.status` as follows:

- **`status.observedGeneration`**
  - Set to `metadata.generation` when the controller has successfully reconciled the desired state for that generation.

- **`status.lastBuildRunRef`**
  - Set on `BuildRun` creation (or when adopting the expected owned `BuildRun`):
    - `<namespace>/<buildRun.name>`

- **`status.latestImage`**
  - Set only when the `BuildRun` completes successfully.
  - If Shipwright provides an output digest (`BuildRun.status.output.digest`), record:
    - `<cappBuild.spec.output.image>@<digest>`
  - Otherwise (no digest available), record a best-effort reference derived from `cappBuild.spec.output.image`:
    - If `spec.output.image` already includes a tag or digest, record it as-is.
    - If `spec.output.image` is repository-only (no tag/digest), leave `status.latestImage` unchanged (or empty) to avoid implying an immutable produced image reference.

### 4) Conditions contract (answering: mirror Shipwright vs our own?)
**Decision**: implement **our own** `CappBuild` conditions contract, while *deriving* it from Shipwright `BuildRun.status.conditions[type=Succeeded]`.

**Why**
- Shipwright reasons/messages are not a stable API for our operator users (they may change across Shipwright versions and can be high-cardinality).
- We want a stable “source → image” contract even if the underlying build engine evolves.
- We still provide a pointer to the full raw status via `status.lastBuildRunRef` for deep debugging.

**Condition types**
- `Ready`: configuration/association readiness:
  - `Ready=True` when the controller has validated required policy/inputs and has successfully reconciled the associated Shipwright `Build` and the expected owned `BuildRun` for the current `generation`.
  - `Ready=False` only for configuration/association/reconcile blockers (e.g., missing policy, conflicts, API errors).
  - `Ready` should remain `True` even if the last `BuildRun` failed; that outcome is represented by `BuildSucceeded`.
- `BuildSucceeded`: outcome of the **latest** `BuildRun` referenced by `status.lastBuildRunRef`.

**Mapping from Shipwright**
Shipwright `BuildRun` uses the condition type `Succeeded`:
- If `Succeeded=True`:
  - `BuildSucceeded=True`
  - `Reason=BuildRunSucceeded`
- If `Succeeded=False`:
  - `BuildSucceeded=False`
  - `Reason=BuildRunFailed`
- If `Succeeded=Unknown` (running/starting):
  - `BuildSucceeded=Unknown`
  - `Reason=BuildRunRunning`
- If `Succeeded` condition is missing:
  - `BuildSucceeded=Unknown`
  - `Reason=BuildRunPending`

**Message guidance**
- Keep the `Reason` stable (low-cardinality).
- Message may include the underlying Shipwright `Succeeded.reason`/`Succeeded.message` for operator debugging, but should not be treated as a stable contract.

### 5) Failure modes (controller behavior)
Required failure handling and requeue semantics:
- **Missing/invalid `BuildRef`**:
  - `Ready=False`, `Reason=BuildMissing`
  - Requeue with backoff (e.g., 30s) since this is recoverable.
- **`BuildRun` conflict** (name exists but not owned):
  - `Ready=False`, `Reason=BuildRunConflict`
  - No requeue (requires human action).
- **Client failures** (create/get/patch):
  - `Ready=False`, `Reason=BuildRunReconcileFailed`
  - Requeue with backoff.
- **BuildRun failed** (`Succeeded=False`):
  - `BuildSucceeded=False`, `Reason=BuildRunFailed`
  - `Ready` remains `True` (configuration is still valid; only the run failed)
  - No hot-looping; rely on watch events + optional backoff if needed.


