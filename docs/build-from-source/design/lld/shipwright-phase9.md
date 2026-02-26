# Build-from-Source for `Capp` using `Shipwright` — Phase 9 (Low-Level Plan)

## Scope
Phase 9 adds **minimal e2e coverage** for the `CappBuild` ↔ Shipwright integration:
- Shipwright `Build` creation/ownership
- Shipwright `BuildRun` creation/ownership
- `CappBuild.status` reference fields (`buildRef`, `lastBuildRunRef`) updated based on the created Shipwright resources

This phase is **tests only**. We do **not** add new controller behavior.

## Goals
- Validate integration aspects that unit tests cannot cover:
  - RBAC is sufficient for the controller to read/create Shipwright resources and patch `CappBuild.status`
  - deterministic naming + ownership/labels are enforced in a real API server
- Keep the e2e suite **small, stable, and fast**:
  - do not depend on Tekton execution (pods, registry pushes)
  - avoid duplicating unit-test-only coverage (reason mapping edge-cases, helper pure functions)

## Non-goals
- Running real Shipwright/Tekton builds (pods, registry pushes, build logs).
- Exhaustive strategy-selection matrices (already covered in unit tests).
- Re-testing `BuildSucceeded` success/failure mapping (unit-tested).
- Conflict-path and error-injection scenarios (hard to make stable in shared clusters).

## Deliverables

### 1) E2E prerequisites (scheme + capability gating)
- Extend the e2e test `Scheme` to include Shipwright API types needed by the tests:
  - `shipwright.io` `Build`, `BuildRun`, and `ClusterBuildStrategy`
- Add a small **capability check** helper used by all Phase 9 tests:
  - If Shipwright CRDs are not installed, **skip** these tests with a clear message.
  - If the `CappConfig` references missing `ClusterBuildStrategy` names, **skip** (or fail fast) with a clear message.

**Rationale**
- Keeps Phase 9 tests high-signal while avoiding failures in clusters where Shipwright isn’t provisioned.

### 2) E2E test: `CappBuild` creates Shipwright `Build` and `BuildRun`
Add one e2e spec that validates orchestration and idempotent rotation:
- Creates a `CappBuild` with:
  - `spec.source.type=Git` (URL required)
  - `spec.output.image` set to a **tagged** image (avoid “repo-only” ambiguity)
  - `spec.buildFile.mode` set to one of the supported modes (pick **one**; do not test both)
- Asserts (Eventually):
  - A Shipwright `Build` exists in the same namespace:
    - `metadata.name == <cappBuild.name>-build`
    - controller-owned by the `CappBuild`
    - includes the operator’s parent label (`rcs.dana.io/parent-cappbuild=<cappBuild.name>`)
  - A Shipwright `BuildRun` exists for the current generation:
    - `metadata.name == <cappBuild.name>-buildrun-<cappBuild.generation>`
    - controller-owned by the `CappBuild`
    - references the expected `Build` by name
    - includes the same parent label
  - `CappBuild.status.buildRef` and `CappBuild.status.lastBuildRunRef` are populated.

- Updates the `CappBuild.spec` to trigger a new generation (pick a minimal, safe field change, e.g. `spec.source.git.revision`).
- Asserts (Eventually):
  - A **new** Shipwright `BuildRun` exists for the new generation:
    - `metadata.name == <cappBuild.name>-buildrun-<newGeneration>`
    - controller-owned by the `CappBuild`
  - `CappBuild.status.lastBuildRunRef` updates to point at the new BuildRun.

- Deletes the `CappBuild`.
- Asserts (Eventually):
  - the controller-owned Shipwright `Build` and both `BuildRun` objects are deleted via Kubernetes garbage collection (ownerReferences).

**Assertion guidance**
- Assert only stable contract fields (name/namespace, ownership, and the key `status.*Ref` fields).
- Do not assert internal message strings or full spec trees unless required for correctness.
- It is acceptable for `BuildSucceeded` to remain `Unknown` in this test (we are not executing Tekton).

### 3) Test hygiene (keep it minimal and low-flake)
- Use random names (consistent with existing e2e utilities).
- No additional global fixtures; rely on the existing suite namespace cleanup.
- Keep to **one** new e2e spec total (Deliverable 2).
