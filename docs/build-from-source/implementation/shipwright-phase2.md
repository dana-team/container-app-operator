# Shipwright Phase 2 — `CappBuild` API + Controller (implementation runbook)

Goal: implement the `CappBuild` CRD and a dedicated controller that translates `CappBuild` into Shipwright `Build` / `BuildRun`, then (optionally) updates a referenced `Capp` with the new image.

This doc is intentionally minimal. It assumes the schema in `docs/build-from-source/design/lld/shipwright-phase2.md`.

## 0) Prereqs
- Run Phase 1 prereqs first (Tekton + Shipwright installed).
- Ensure `controller-gen` is available via `make controller-gen`.

## 1) Scaffold the API + controller (kubebuilder)
From repo root:

```bash
kubebuilder create api --group rcs --version v1alpha1 --kind CappBuild --resource --controller
```

This will generate:
- API type in `api/v1alpha1/`
- Controller skeleton in `internal/controller/` (kubebuilder default)
- RBAC markers in controller code

## 2) Move controller into repo structure (optional but recommended)
This repo keeps controllers under `internal/kinds/<kind>/controllers/`.

- Create a new folder: `internal/kinds/cappbuild/controllers/`
- Move the generated reconciler into that folder (rename package accordingly).

If you keep kubebuilder’s default `internal/controller/`, skip this step.

## 3) Register the new type in the manager scheme
Update `cmd/main.go`:
- In `init()` add:
  - `utilruntime.Must(<your cappbuild api>.AddToScheme(scheme))`

If you keep the CRD in the existing `api/v1alpha1` package, this is typically already covered by the existing `cappv1alpha1.AddToScheme(scheme)`; verify the generated API is in the same package as existing types.

## 4) Wire the controller into `cmd/main.go`
In `main()` (near other `.SetupWithManager` calls), add the `CappBuildReconciler`:
- `(&cappbuildcontroller.CappBuildReconciler{ ... }).SetupWithManager(mgr)`

Use the same patterns as existing reconcilers:
- pass `Client`, `Scheme`, and an `EventRecorder`.

## 5) Add Shipwright API to scheme (controller runtime)
The controller needs to create/read Shipwright resources.

Add Shipwright Build API types to the manager scheme (in `cmd/main.go init()`), by importing and registering Shipwright’s Go API package.

At minimum you need the Shipwright `Build` and `BuildRun` types on the scheme.

## 6) Implement the `CappBuild` → Shipwright mapping
In the `CappBuild` reconciler:

### 6.1 Create/patch a Shipwright `Build`
Create a namespaced Shipwright `Build` derived from `CappBuild.spec`:
- `spec.source.type = Git`
- `spec.source.git.url = CappBuild.spec.source.git.url`
- `spec.source.git.revision = CappBuild.spec.source.git.revision` (omit if empty)
- `spec.source.contextDir = CappBuild.spec.source.git.contextDir` (omit if empty)
- `spec.source.git.cloneSecret = CappBuild.spec.source.authRef.name` (omit if authRef not set)

Strategy selection is platform-owned; pick a default strategy here (or read policy from `CappConfig` later).

### 6.2 Create a Shipwright `BuildRun`
On initial reconcile (or when user requests a run / or commit trigger), create a `BuildRun` that references the `Build`.

### 6.3 Update `CappBuild.status`
Update only what the LLD requires:
- `status.observedGeneration`
- `status.conditions` (`Ready`, `BuildSucceeded`)
- `status.latestImage` (from successful Shipwright output)
- `status.lastBuildRunRef`
- `status.lastAppliedCapp` (only if `spec.cappRef` is set and the handover succeeded)

### 6.4 Optional handover to `Capp`
If `spec.cappRef` is set:
- patch the referenced `Capp` to use `status.latestImage`
- record `status.lastAppliedCapp`

## 7) RBAC
RBAC for `CappBuild` controller must include:
- CRUD/watch on `CappBuild`
- create/patch/watch Shipwright `Build` and `BuildRun`
- patch/update `CappBuild/status`
- (optional) patch/update `Capp` when `spec.cappRef` is set

Use kubebuilder RBAC markers in the controller file to generate manifests.

## 8) Generate manifests + CRDs
Run:

```bash
make manifests generate
```

Ensure the new CRD appears under:
- `config/crd/bases/`

## 9) Add sample YAML
Add a sample under `config/samples/` for `CappBuild` (use the examples from the LLD).

## 10) Quick validation
Apply CRD and deploy controller (dev cluster):

```bash
make install
make deploy
```

Create a `CappBuild` instance and verify:
- Shipwright `Build` + `BuildRun` are created
- `CappBuild.status.latestImage` is populated on success
- If `spec.cappRef` is set, the `Capp` is updated


