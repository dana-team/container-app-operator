# Build-from-Source for `Capp` using `Shipwright` — Phase 6 (Low-Level Plan)

## Scope
Phase 6 adds **test coverage** for the Phase 5 implementation (CappBuild reconcile start):
- Policy read (`CappConfig.spec.cappBuild`) and strategy name selection from `spec.buildFile.mode`.
- Deterministic association + reconcile of Shipwright `Build` (create/patch + ownership).
- Status updates on `CappBuild` (`status.observedGeneration`, `status.buildRef`).
- Error handling and conditions for missing platform configuration / missing strategy / build conflicts.

This phase is **tests only** (no new controller behavior beyond what is already implemented in Phase 5).

## Goals
- Validate Phase 5 behavior with **minimal, high-signal tests**.
- Split coverage between **unit** and **e2e** to avoid redundant assertions.
- Keep reasons/messages stable and low-cardinality; tests should assert contracts, not incidental implementation details.

## Non-goals
- Testing image build execution success (requires Shipwright `BuildRun`; out of scope).
- Testing Git source probing for Dockerfile/Containerfile (not implemented in Phase 5 controller flow).
- Adding new CRD fields or controller behavior.

## Deliverables

### 1) Unit tests (authoritative for branching + failure modes)
Add unit tests covering controller decision paths and error handling. These are preferred for:
- Deterministic coverage of rare failure paths.
- Precise assertions on condition reasons/messages and requeue behavior.

**Required unit test cases**
- **Missing policy**:
  - When `CappConfig` is missing OR `CappConfig.spec.cappBuild` is `nil`:
    - `Ready=False`, `Reason=MissingPolicy`
    - reconcile returns `RequeueAfter=30s`
- **Strategy selection**:
  - `CappBuild.spec.buildFile.mode=Absent` selects `CappConfig.spec.cappBuild.clusterBuildStrategy.buildFile.absent`
  - `CappBuild.spec.buildFile.mode=Present` selects `CappConfig.spec.cappBuild.clusterBuildStrategy.buildFile.present`
- **ClusterBuildStrategy precondition**:
  - When the selected `ClusterBuildStrategy` name does not exist:
    - `Ready=False`, `Reason=BuildStrategyNotFound`
    - reconcile returns `RequeueAfter=30s`
- **Build conflict**:
  - When Shipwright `Build` exists but is not controller-owned by the `CappBuild`:
    - `Ready=False`, `Reason=BuildConflict`
    - reconcile returns no requeue
- **Build reconcile failure**:
  - Generic client/create/patch failure:
    - `Ready=False`, `Reason=BuildReconcileFailed`
    - reconcile returns `RequeueAfter=30s`
- **Status updates**:
  - On success:
    - `status.observedGeneration == metadata.generation`
    - `status.buildRef == "<namespace>/<cappBuild.name>-build"`

**Unit test guidance**
- Prefer fake client tests (controller-runtime fake client) for reconciliation logic.
- Assert only stable contracts:
  - Condition `Type=Ready`, `Status=False`, stable `Reason` values.
  - Requeue semantics (`RequeueAfter` values).
  - Deterministic naming / ownership constraints.

### 2) E2E tests (minimal integration contracts, non-redundant)
E2E tests should assert only integration-level guarantees that unit tests cannot provide:
- Shipwright `Build` object is actually created in the cluster.
- OwnerReferences are set correctly.
- Strategy updates propagate to the real CR.

**Required e2e test cases**
- **Creates Build + updates status**:
  - Create a `CappBuild` and assert eventually:
    - `status.observedGeneration == metadata.generation`
    - `status.buildRef` is populated deterministically
    - associated Shipwright `Build` exists and is controller-owned by the `CappBuild`
- **Updates strategy on mode change**:
  - Patch `CappBuild.spec.buildFile.mode` and assert eventually:
    - Shipwright `Build.spec.strategy.name` updates accordingly

**E2E anti-redundancy rule**
- Do not duplicate unit-test assertions (detailed error branches, reason/message permutations).
- Keep e2e assertions to resource existence/ownership and high-level spec effects.

### 3) Condition/reason contracts (for test assertions)
Tests must assert stable, low-cardinality reasons:
- `MissingPolicy`
- `BuildStrategyNotFound`
- `BuildConflict`
- `BuildReconcileFailed`

### 4) Expected artifacts
- New/updated unit tests for CappBuild controller.
- Updated e2e tests for CappBuild controller (only the two required cases above).


