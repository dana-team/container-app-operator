# Build-from-Source for `Capp` using `Shipwright` — Phase 8 (Low-Level Plan)

## Scope
Phase 8 adds **unit test coverage** for the Phase 7 implementation (Shipwright `BuildRun` creation + status updates).

This phase is **tests only** (no new controller behavior beyond what is already implemented in Phase 7).

Also included: a small test-file cleanup to make intent clearer:
- Rename `controller_test.go` → `build_test.go` (tests that focus on Shipwright `Build` reconciliation).
- Add `buildrun_test.go` (tests that focus on Shipwright `BuildRun` reconciliation + status mapping).

## Goals
- Validate Phase 7 behavior with **minimal, high-signal unit tests**.
- Assert **stable operator contracts** (condition types/status/reasons, requeue semantics, deterministic naming/ownership), not incidental implementation details.
- Keep reason assertions **low-cardinality** and aligned with the Phase 7 condition contract.

## Non-goals
- E2E tests for `BuildRun` execution (Tekton/Shipwright runtime behavior is out of scope for unit tests).
- Testing Shipwright/Tekton internals beyond the controller’s mapping rules.
- Adding new CRD fields, controller logic, or new condition types/reasons.

## Deliverables

### 1) Test file organization
- Rename: `internal/kinds/cappbuild/controllers/controller_test.go` → `internal/kinds/cappbuild/controllers/build_test.go`.
  - The file currently tests `Build` reconcile behavior (policy/strategy selection, `Build` conflict, `status.buildRef`, `Ready`).
- Create: `internal/kinds/cappbuild/controllers/buildrun_test.go`.
  - This file owns Phase 7 tests (BuildRun creation/idempotency + `BuildSucceeded` + `latestImage` behavior).

**Clarification (test intent)**
- Keep **Build-focused** tests limited to `Build` association/ownership and drift correction.
- Avoid coupling Build-focused tests to `BuildRun` execution/progress (e.g., `BuildSucceeded`, `Ready`, `RequeueAfter`) because those are driven by BuildRun status and may legitimately change without affecting Build reconciliation correctness.

### 2) Unit tests: BuildRun reconciliation (authoritative for branching + failure modes)
Add unit tests that cover the decision paths and failure handling introduced in Phase 7.

**Required unit test cases**
- **Create BuildRun when missing**
  - When the expected `BuildRun` does not exist:
    - The created `BuildRun.name` matches the deterministic naming rule for the `CappBuild` generation.
    - Controller creates it in the same namespace.
    - `BuildRun.spec.build.name` references the deterministic `Build` name.
    - `BuildRun` is controller-owned by the `CappBuild`.
- **BuildRun idempotency**
  - When the expected `BuildRun` exists and is controller-owned:
    - Reconcile does not create a new `BuildRun` and returns the existing one.
- **BuildRun conflict**
  - When the expected `BuildRun` name exists but is not controller-owned by the `CappBuild`:
    - `Ready=False`, `Reason=BuildRunConflict`
    - Reconcile returns no requeue (requires human action).
- **BuildRun reconcile failure**
  - On client failures (create/get other than NotFound):
    - `Ready=False`, `Reason=BuildRunReconcileFailed`
    - Reconcile returns `RequeueAfter=30s`.

### 3) Unit tests: BuildSucceeded condition mapping contract
Add unit tests for the mapping from Shipwright `BuildRun.status.conditions[type=Succeeded]` to operator-facing `CappBuild.status.conditions[type=BuildSucceeded]`.

**Required unit test cases**
- **Succeeded condition missing**
  - `BuildSucceeded=Unknown`, `Reason=BuildRunPending`
- **Succeeded=True**
  - `BuildSucceeded=True`, `Reason=BuildRunSucceeded`
- **Succeeded=False**
  - `BuildSucceeded=False`, `Reason=BuildRunFailed`
- **Succeeded=Unknown (running/starting)**
  - `BuildSucceeded=Unknown`, `Reason=BuildRunRunning`

**Clarification (why two `Unknown` reasons?)**
- `BuildRunPending`: Shipwright has **not published** a `Succeeded` condition yet (missing condition).
- `BuildRunRunning`: Shipwright has published `Succeeded`, but it is **explicitly `Unknown`** (in progress).

**Assertion guidance**
- Assert only stable fields:
  - `Type=BuildSucceeded`, `Status`, stable `Reason` values, and `ObservedGeneration`.
- Do not assert exact message strings beyond sanity (messages may include upstream reason/message for debugging).

### 4) Unit tests: `status.latestImage` update rules
Add unit tests to validate how the controller populates `CappBuild.status.latestImage` on successful builds.

**Required unit test cases**
- **Digest present**
  - If `BuildRun.status.output.digest` is set and the BuildRun is successful:
    - `status.latestImage == <spec.output.image>@<digest>`
- **No digest, but output image has tag or digest**
  - If the BuildRun is successful and `spec.output.image` includes a tag (or is already digested):
    - `status.latestImage == spec.output.image`
- **Repository-only image**
  - If the BuildRun is successful and `spec.output.image` has no tag/digest:
    - `status.latestImage` is not set/changed (empty/no-op).

### 5) Unit tests: requeue behavior for “running” BuildRuns
- If `BuildSucceeded` is `Unknown` after mapping (running/pending):
  - Reconcile returns `RequeueAfter=10s` to refresh status even if watch events are delayed.

