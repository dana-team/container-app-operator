# Build-from-Source for `Capp` using `Shipwright` — Phase 5 (Low-Level Plan)

## Scope
Phase 5 defines the **start of the `CappBuild` reconciliation workflow**:
- Fetching/validating the `CappBuild` resource.
- Resolving platform policy needed to choose a build strategy.
- Probing the source to determine whether a **`Dockerfile` or `Containerfile`**
  exists in the build context.
- Selecting the **ClusterBuildStrategy** name to use for the build.

This phase stops **before** creating Shipwright `Build`/`BuildRun` objects.

## Goals
- Make the controller’s “first mile” deterministic and observable:
  resource fetch, policy resolution, source probing, and strategy selection.
- Select the build strategy based on **presence/absence of a build file**:
  `Dockerfile` or `Containerfile` within the repo (respecting `contextDir`).
- Fail fast with clear `status.conditions` when inputs or policy are missing.

## Deliverables

### 1) Reconcile entry + fetch `CappBuild`
- Implement the initial reconcile skeleton for `CappBuildReconciler.Reconcile`.
- Fetch the `CappBuild` by `req.NamespacedName`.
  - If not found: return success (standard controller behavior).
- Initialize/maintain:
  - `status.observedGeneration`
  - `status.conditions[]` (at minimum: `Ready`)

### Implementation locations (files + naming)
- Controllers: `internal/kinds/cappbuild/controllers`
- Reconciler: `internal/kinds/cappbuild/controllers/controller.go`
- Helpers: `internal/kinds/cappbuild/controllers/shipwright_build.go`
- Condition consts: `internal/kinds/cappbuild/controllers/conditions.go`
- Tests: `internal/kinds/cappbuild/controllers/shipwright_build_test.go`

### 2) Find or create the associated Shipwright `Build`
We must be able to deterministically answer: “is there already a Shipwright
`Build` for this `CappBuild`?”

**Association rules (best practice)**:
- The `Build` lives in the **same namespace** as the `CappBuild`.
- The `Build` name is deterministic: `cappBuild.Name + "-build"`.
- The `Build` must be controller-owned by the `CappBuild` via `OwnerReference`
  (`controllerutil.SetControllerReference`).
- Label (optional): `<apiGroup>/parent-cappbuild: <cappBuild.name>`.

**Behavior**:
- `Get()` the `Build` by deterministic name.
- If owned: reconcile/patch to desired spec.
- If not owned: `Ready=False (BuildConflict)` and stop.
- If missing: compute strategy and create.

**Build association helpers (enforces association logic)**:

Copy to: `internal/kinds/cappbuild/controllers/conditions.go`

```go
package controllers

import "errors"

const (
	conditionTypeReady = "Ready"

	reasonBuildReconcileFailed = "BuildReconcileFailed"
	reasonBuildConflict        = "BuildConflict"
)

var ErrBuildConflict = errors.New("build conflict")
```

Copy to: `internal/kinds/cappbuild/controllers/shipwright_build.go`

```go
package controllers

func (r *CappBuildReconciler) setReadyCondition(
	ctx context.Context,
	cb *cappv1alpha1.CappBuild,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	orig := cb.DeepCopy()

	meta.SetStatusCondition(&cb.Status.Conditions, metav1.Condition{
		Type:               conditionTypeReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: cb.Generation,
		// LastTransitionTime is set by meta.SetStatusCondition
	})

	return r.Status().Patch(ctx, cb, client.MergeFrom(orig))
}

// getBuild returns the associated Build for this CappBuild.
// It only returns a Build when it exists AND is owned by the CappBuild.
func (r *CappBuildReconciler) getBuild(
	ctx context.Context,
	cb *cappv1alpha1.CappBuild,
) (*shipwrightv1alpha1.Build, bool, error) {
	buildName := cb.Name + "-build"
	key := types.NamespacedName{Namespace: cb.Namespace, Name: buildName}

	existing := &shipwrightv1alpha1.Build{}
	if err := r.Get(ctx, key, existing); err == nil {
		if !metav1.IsControlledBy(existing, cb) {
			return nil, false, fmt.Errorf("%w: %s exists but is not owned by CappBuild %s/%s",
				ErrBuildConflict, key.String(), cb.Namespace, cb.Name)
		}
		return existing, true, nil
	} else if apierrors.IsNotFound(err) {
		return nil, false, nil
	} else {
		return nil, false, err
	}
}

func newBuild(
	cb *cappv1alpha1.CappBuild,
	selectedStrategyName string,
	scheme *runtime.Scheme,
) (*shipwrightv1alpha1.Build, error) {
	build := &shipwrightv1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cb.Name + "-build",
			Namespace: cb.Namespace,
			Labels: map[string]string{
				cappv1alpha1.GroupVersion.Group + "/parent-cappbuild": cb.Name,
			},
		},
		Spec: shipwrightv1alpha1.BuildSpec{
			Strategy: shipwrightv1alpha1.Strategy{
				Kind: "ClusterBuildStrategy",
				Name: selectedStrategyName,
			},
			// Source and Output are derived from cb.Spec. Later phases can expand
			// credentials/params wiring, but the selected strategy is fixed here.
			Source: shipwrightv1alpha1.Source{
				// TODO: map cb.Spec.Source (git url/revision/contextDir/cloneSecret)
			},
			Output: shipwrightv1alpha1.Image{
				// TODO: map cb.Spec.Output (image/pushSecret) or platform defaults
			},
		},
	}

	if err := controllerutil.SetControllerReference(cb, build, scheme); err != nil {
		return nil, err
	}
	return build, nil
}

// ensureBuild returns an existing associated Build or creates it.
// This is the authoritative place that enforces the CappBuild ⇄ Build association.
func (r *CappBuildReconciler) ensureBuild(
	ctx context.Context,
	cb *cappv1alpha1.CappBuild,
	selectedStrategyName string,
) (*shipwrightv1alpha1.Build, error) {
	if existing, ok, err := r.getBuild(ctx, cb); err != nil {
		if errors.Is(err, ErrBuildConflict) {
			_ = r.setReadyCondition(ctx, cb, metav1.ConditionFalse, reasonBuildConflict, err.Error())
			return nil, err
		}
		_ = r.setReadyCondition(ctx, cb, metav1.ConditionFalse, reasonBuildReconcileFailed, err.Error())
		return nil, err
	} else if ok {
		// Reconcile the existing Build to match the current CappBuild desired state.
		// In Phase 5, we avoid re-probing source here; we preserve the existing
		// strategy selection unless explicitly recomputed by the caller.
		desired, err := newBuild(cb, existing.Spec.Strategy.Name, r.Scheme)
		if err != nil {
			return nil, err
		}

		orig := existing.DeepCopy()
		existing.Labels = desired.Labels
		existing.Spec = desired.Spec

		if err := r.Patch(ctx, existing, client.MergeFrom(orig)); err != nil {
			_ = r.setReadyCondition(ctx, cb, metav1.ConditionFalse, reasonBuildReconcileFailed, err.Error())
			return nil, err
		}
		return existing, nil
	}

	desired, err := newBuild(cb, selectedStrategyName, r.Scheme)
	if err != nil {
		return nil, err
	}
	if err := r.Create(ctx, desired); err != nil {
		_ = r.setReadyCondition(ctx, cb, metav1.ConditionFalse, reasonBuildReconcileFailed, err.Error())
		return nil, err
	}
	return desired, nil
}
```

### 3) Resolve policy inputs (strategy names) (only when no `Build` exists yet)
Resolve the strategy name pair from `CappConfig`:
- `spec.cappBuild.clusterBuildStrategy.buildFile.present`
- `spec.cappBuild.clusterBuildStrategy.buildFile.absent`

Policy is enforced in the controller (no webhook required).

If the policy is not available (missing `CappConfig`, missing required fields,
or feature disabled / not configured):
- Set `Ready=False` with reason `MissingPolicy`, emit an event, and requeue with
  backoff.

### 4) Probe the source for `Dockerfile` / `Containerfile` (only when no `Build` exists yet)
For `spec.source.type=Git`, determine build-file presence by inspecting the
repository at the requested revision:

- **Implementation approach (Option A / provider-agnostic)**:
  - Use `github.com/go-git/go-git/v5` to fetch the requested ref (shallow when
    possible) into a temporary workspace.
  - Check file existence with a simple `Stat` on:
    - `<contextDir>/Dockerfile`
    - `<contextDir>/Containerfile`

- **Inputs**:
  - `url`, `revision` (or default branch/HEAD), `contextDir`
  - `authRef` (optional Secret reference) for private repos
- **Probe behavior**:
  - Fetch only what’s needed to answer: “does a file named `Dockerfile` or
    `Containerfile` exist under `contextDir`?”
  - Treat any of these as present:
    - `<contextDir>/Dockerfile`
    - `<contextDir>/Containerfile`
  - If both exist, `Dockerfile` wins (documented tie-breaker for determinism).
- **Errors**:
  - If the repository cannot be accessed or inspected, set `Ready=False` with
    reason `SourceAccessFailed` and include a concise message.

### 5) Select effective ClusterBuildStrategy name (only when no `Build` exists yet)
Based on the probe result:
- If build file is **present**: select policy `buildFile.present`
- If build file is **absent**: select policy `buildFile.absent`

Persist the decision for later phases:
- Record the selected strategy name in controller-local state for the reconcile
  loop, and optionally in `status` (if/when we introduce a status field such as
  `status.selectedBuildStrategy`).

### 6) Create Shipwright `Build` CR (skeleton)
Create or patch the `Build` after strategy selection. Minimum fields:
- `metadata.name`: `<cappBuild.name>-build`
- `metadata.namespace`: `cappBuild.namespace`
- `metadata.ownerReferences`: controller owner = `CappBuild`
- `spec.strategy.kind/name`
- `spec.source`, `spec.output` (mapped from `CappBuild`)

### 7) Status / conditions updates (wired into the workflow)
Phase 5 sets `Ready=False` only on failures (no “success/progress” conditions).

Recommended reason strings (stable, low cardinality):
Define these as constants (single source of truth) and keep them stable:
- `InvalidSpec`
- `BuildReconcileFailed`
- `BuildConflict`
- `MissingPolicy`
- `SourceAccessFailed`
- `StrategySelected`

### 8) Test plan (unit-level)
Add focused unit tests around the decision logic:
- Missing/invalid `CappBuild.spec.source` ⇒ `Ready=False (InvalidSpec)`
- Missing policy ⇒ `Ready=False (MissingPolicy)`
- Existing Shipwright `Build` associated ⇒ updates it if needed
- `getBuild`: returns not-found without error
- `getBuild`: conflicts when `Build` name exists but not owned
- Probe finds `Dockerfile` ⇒ selects `buildFile.present`
- Probe finds `Containerfile` only ⇒ selects `buildFile.present`
- Probe finds neither ⇒ selects `buildFile.absent`
- Probe error ⇒ `Ready=False (SourceAccessFailed)`


