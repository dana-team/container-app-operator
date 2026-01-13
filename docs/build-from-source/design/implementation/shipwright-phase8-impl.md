# Implementation — Phase 8 tests (BuildRun + status mapping + latestImage)

This phase is **tests only**. It adds unit-test coverage for the Phase 7 behavior (Shipwright `BuildRun` creation + mapping `BuildRun` status → `CappBuild` status).

## 1) Split tests by responsibility (Build vs BuildRun)

Rename:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/controller_test.go`
  → `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/build_test.go`

Guidance:
- `build_test.go` should stay focused on **Shipwright `Build` reconcile** (policy/strategy selection, Build ownership/conflict, Build drift correction, `status.buildRef`, `status.observedGeneration`).
- `buildrun_test.go` (next step) owns **BuildRun + status mapping + latestImage** tests.

## 1.1) Add shared test helpers (recommended)

Create:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/helpers_test.go`

Move shared test scaffolding here (so helpers are not “owned” by one test file, and so `buildrun_test.go` does not need to redefine them):
- `testScheme(t)`
- `newReconciler(t, ...)`
- `newCappConfig()`
- `newCappBuild(name, namespace)`
- shared constants (e.g. `absentStrategy`)

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/helpers_test.go`

```go
package controllers

import (
	"testing"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	capputils "github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const absentStrategy = "absent-strategy"

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, rcs.AddToScheme(s))
	require.NoError(t, shipwright.AddToScheme(s))
	return s
}

func newReconciler(t *testing.T, objs ...client.Object) (*CappBuildReconciler, client.Client) {
	t.Helper()

	s := testScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&rcs.CappBuild{}).
		WithObjects(objs...).
		Build()

	return &CappBuildReconciler{
		Client: c,
		Scheme: s,
	}, c
}

func newCappConfig() *rcs.CappConfig {
	return &rcs.CappConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      capputils.CappConfigName,
			Namespace: capputils.CappNS,
		},
		Spec: rcs.CappConfigSpec{
			CappBuild: &rcs.CappBuildConfig{
				ClusterBuildStrategy: rcs.CappBuildClusterStrategyConfig{
					BuildFile: rcs.CappBuildFileStrategyConfig{
						Present: "present-strategy",
						Absent:  absentStrategy,
					},
				},
			},
		},
	}
}

func newCappBuild(name, namespace string) *rcs.CappBuild {
	return &rcs.CappBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: rcs.CappBuildSpec{
			BuildFile: rcs.CappBuildFileSpec{Mode: rcs.CappBuildFileModeAbsent},
			Source: rcs.CappBuildSource{
				Type: rcs.CappBuildSourceTypeGit,
				Git:  rcs.CappBuildGitSource{URL: "https://example.invalid/repo.git"},
			},
			Output: rcs.CappBuildOutputSpec{Image: "registry.example.com/team/app"},
		},
	}
}
```

## 1.2) Remove helper funcs from `build_test.go` (after adding `helpers_test.go`)

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/build_test.go`

Goal:
- `build_test.go` contains **only Build-focused tests**.
- All shared helpers/constants are defined **once** in `helpers_test.go`.

Remove (move to `helpers_test.go` if not already there):
- `const absentStrategy = ...`
- `func testScheme(t *testing.T) *runtime.Scheme`
- `func newReconciler(t *testing.T, objs ...client.Object) ...`
- `func newCappConfig() *rcs.CappConfig`
- `func newCappBuild(name, namespace string) *rcs.CappBuild`

Also:
- Delete any now-unused imports from `build_test.go` (e.g. `runtime`, `fake`, `capputils`, etc.),
  since those should only be needed by the helpers.
- Keep the tests calling the helpers as-is (same names), so other test files (like `buildrun_test.go`)
  can reuse them without redefining anything.

## 2) Add BuildRun unit tests + mapping contract tests

Create:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/buildrun_test.go`

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/buildrun_test.go`

```go
package controllers

import (
	"context"
	"testing"
	"time"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Uses shared helpers from helpers_test.go:
// - testScheme(t)
// - newReconciler(t, ...)
// - newCappConfig()
// - newCappBuild(name, namespace)

func TestReconcileCreatesBuildRun(t *testing.T) {
	ctx := context.Background()

	cb := newCappBuild("cb-"+t.Name(), "ns-"+t.Name())
	r, c := newReconciler(t, cb)

	br, err := r.reconcileBuildRun(ctx, cb)
	require.NoError(t, err)
	require.NotNil(t, br)

	actualBuildRun := &shipwright.BuildRun{}
	require.NoError(t, c.Get(ctx, types.NamespacedName{Name: buildRunNameFor(cb), Namespace: cb.Namespace}, actualBuildRun))
	require.Equal(t, buildRunNameFor(cb), actualBuildRun.Name)
	require.Equal(t, cb.Namespace, actualBuildRun.Namespace)

	require.True(t, metav1.IsControlledBy(actualBuildRun, cb), "BuildRun should be controller-owned by CappBuild")
	require.NotNil(t, actualBuildRun.Spec.Build.Name)
	require.Equal(t, buildNameFor(cb), *actualBuildRun.Spec.Build.Name)
}

func TestReconcileReusesExistingBuildRun(t *testing.T) {
	ctx := context.Background()

	cb := newCappBuild("cb-"+t.Name(), "ns-"+t.Name())
	// This test is scoped to a single generation. If CappBuild.Generation changes,
	// the desired BuildRun name changes and creating a new BuildRun is expected.
	existingBuildRun := newBuildRun(cb)
	existingBuildRun.UID = types.UID("existing-buildrun-uid")

	// Make it owned by cb.
	controller := true
	existingBuildRun.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: rcs.GroupVersion.String(),
		Kind:       "CappBuild",
		Name:       cb.Name,
		UID:        types.UID("cb-uid"),
		Controller: &controller,
	}}
	cb.UID = types.UID("cb-uid")

	r, _ := newReconciler(t, cb, existingBuildRun)
	br, err := r.reconcileBuildRun(ctx, cb)
	require.NoError(t, err)
	require.Equal(t, existingBuildRun.Name, br.Name)
	require.Equal(t, existingBuildRun.UID, br.UID, "expected reconcileBuildRun to return the existing BuildRun object")
}

func TestReconcileBuildRunConflict(t *testing.T) {
	ctx := context.Background()

	cb := newCappBuild("cb-"+t.Name(), "ns-"+t.Name())
	conflict := newBuildRun(cb)

	// Owned by someone else.
	controller := true
	conflict.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: rcs.GroupVersion.String(),
		Kind:       "CappBuild",
		Name:       "someone-else",
		UID:        types.UID("other-uid"),
		Controller: &controller,
	}}

	r, _ := newReconciler(t, cb, conflict)
	br, err := r.reconcileBuildRun(ctx, cb)
	require.Nil(t, br)
	require.Error(t, err)

	var alreadyOwned *controllerutil.AlreadyOwnedError
	require.ErrorAs(t, err, &alreadyOwned)
}

func TestPatchBuildSucceededCondition(t *testing.T) {
	ctx := context.Background()

	newBR := func(t *testing.T) *shipwright.BuildRun {
		t.Helper()
		return &shipwright.BuildRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "br",
				Namespace: "ns-" + t.Name(),
			},
		}
	}

	assertBuildSucceeded := func(t *testing.T, br *shipwright.BuildRun, expectedStatus metav1.ConditionStatus, expectedReason string) {
		t.Helper()

		cb := newCappBuild("cb-"+t.Name(), "ns-"+t.Name())
		r, _ := newReconciler(t, cb)

		require.NoError(t, r.patchBuildSucceededCondition(ctx, cb, br))

		buildSucceededCond := meta.FindStatusCondition(cb.Status.Conditions, TypeBuildSucceeded)
		require.NotNil(t, buildSucceededCond, "BuildSucceeded condition should be set")
		require.Equal(t, expectedStatus, buildSucceededCond.Status)
		require.Equal(t, expectedReason, buildSucceededCond.Reason)
		require.Equal(t, cb.Generation, buildSucceededCond.ObservedGeneration)
	}

	t.Run("Succeeded condition missing => Pending/Unknown", func(t *testing.T) {
		br := newBR(t)
		assertBuildSucceeded(t, br, metav1.ConditionUnknown, ReasonBuildRunPending)
	})

	t.Run("Succeeded=True => Succeeded/True", func(t *testing.T) {
		br := newBR(t)
		br.Status.SetCondition(&shipwright.Condition{Type: shipwright.Succeeded, Status: corev1.ConditionTrue})
		assertBuildSucceeded(t, br, metav1.ConditionTrue, ReasonBuildRunSucceeded)
	})

	t.Run("Succeeded=False => Failed/False", func(t *testing.T) {
		br := newBR(t)
		br.Status.SetCondition(&shipwright.Condition{Type: shipwright.Succeeded, Status: corev1.ConditionFalse})
		assertBuildSucceeded(t, br, metav1.ConditionFalse, ReasonBuildRunFailed)
	})

	t.Run("Succeeded=Unknown => Running/Unknown", func(t *testing.T) {
		br := newBR(t)
		br.Status.SetCondition(&shipwright.Condition{Type: shipwright.Succeeded, Status: corev1.ConditionUnknown})
		assertBuildSucceeded(t, br, metav1.ConditionUnknown, ReasonBuildRunRunning)
	})
}

func TestComputeLatestImage(t *testing.T) {
	cb := newCappBuild("cb-"+t.Name(), "ns-"+t.Name())

	t.Run("digest present => image@digest", func(t *testing.T) {
		br := &shipwright.BuildRun{}
		br.Status.Output = &shipwright.Output{Digest: "sha256:abc"}
		require.Equal(t, cb.Spec.Output.Image+"@sha256:abc", computeLatestImage(cb, br))
	})

	t.Run("no digest, image already tagged => keep spec.output.image", func(t *testing.T) {
		cb := cb.DeepCopy()
		cb.Spec.Output.Image = "registry.example.com/team/app:v1"
		br := &shipwright.BuildRun{}
		require.Equal(t, "registry.example.com/team/app:v1", computeLatestImage(cb, br))
	})

	t.Run("repo-only image => no-op (empty)", func(t *testing.T) {
		cb := cb.DeepCopy()
		cb.Spec.Output.Image = "registry.example.com/team/app"
		br := &shipwright.BuildRun{}
		require.Equal(t, "", computeLatestImage(cb, br))
	})
}

func TestReconcileRequeuesWhileBuildSucceededUnknown(t *testing.T) {
	ctx := context.Background()

	cappConfig := newCappConfig()
	cb := newCappBuild("cb-"+t.Name(), "ns-"+t.Name())

	// Satisfy "strategy exists" precondition.
	clusterBuildStrategy := &shipwright.ClusterBuildStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: cappConfig.Spec.CappBuild.ClusterBuildStrategy.BuildFile.Absent},
	}

	r, c := newReconciler(t, cb, cappConfig, clusterBuildStrategy)

	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: cb.Name, Namespace: cb.Namespace}})
	require.NoError(t, err)
	require.Equal(t, 10*time.Second, res.RequeueAfter)

	// Assert this reconcile pass is about BuildRun progress, not Ready.
	latest := &rcs.CappBuild{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(cb), latest))
	ready := meta.FindStatusCondition(latest.Status.Conditions, TypeReady)
	require.True(t, ready == nil || ready.Status != metav1.ConditionTrue)
}
```

Notes:
- The tests above intentionally keep **Build** assertions out of the BuildRun mapping tests.
- The `RequeueAfter=10s` assertion belongs with BuildRun progress tests (not Build reconciliation tests).

## 3) Run unit tests

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator` (run in a shell)

```bash
cd /home/sbahar/projects/ps/dana-team/container-app-operator
go test ./internal/kinds/cappbuild/controllers
```

