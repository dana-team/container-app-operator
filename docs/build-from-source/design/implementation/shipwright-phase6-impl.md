# Implementation — Phase 6 tests (Phase 5 CappBuild “first mile”)

This phase is **tests only**. It adds unit and e2e coverage for the Phase 5 CappBuild controller behavior.

## 0) Prereq: ensure Shipwright types are available in e2e test scheme

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/test/e2e_tests/helper.go`

Add import:

```go
shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
```

Add to `newScheme()`:

```go
utilruntime.Must(shipwright.AddToScheme(scheme))
```

## 1) Unit tests for CappBuild controller

Create:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/controller_test.go`

```go
package controllers

import (
	"context"
	"testing"
	"time"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	capputils "github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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

func newCappConfig(present, absent string) *rcs.CappConfig {
	return &rcs.CappConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      capputils.CappConfigName,
			Namespace: capputils.CappNS,
		},
		Spec: rcs.CappConfigSpec{
			// DNSConfig/AutoscaleConfig/DefaultResources are required by the API,
			// but fake client tests do not run OpenAPI validation.
			CappBuild: &rcs.CappBuildConfig{
				ClusterBuildStrategy: rcs.CappBuildClusterStrategyConfig{
					BuildFile: rcs.CappBuildFileStrategyConfig{
						Present: present,
						Absent:  absent,
					},
				},
			},
		},
	}
}

func newCappBuild(name, namespace string, mode rcs.CappBuildFileMode) *rcs.CappBuild {
	return &rcs.CappBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: rcs.CappBuildSpec{
			BuildFile: rcs.CappBuildFileSpec{Mode: mode},
			Source: rcs.CappBuildSource{
				Type: rcs.CappBuildSourceTypeGit,
				Git:  rcs.CappBuildGitSource{URL: "https://example.invalid/repo.git"},
			},
			Output: rcs.CappBuildOutputSpec{Image: "registry.example.com/team/app"},
		},
	}
}

func TestReconcileMissingPolicy(t *testing.T) {
	ctx := context.Background()

	// No CappConfig present -> MissingPolicy.
	cb := newCappBuild("cb", "ns", rcs.CappBuildFileModeAbsent)

	r, c := newReconciler(t, cb)
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: cb.Name, Namespace: cb.Namespace}})
	require.NoError(t, err)
	require.Greater(t, res.RequeueAfter, time.Duration(0))

	latest := &rcs.CappBuild{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(cb), latest))
	cond := meta.FindStatusCondition(latest.Status.Conditions, TypeReady)
	require.NotNil(t, cond, "Ready condition should be set")
	require.Equal(t, metav1.ConditionFalse, cond.Status)
	require.Equal(t, ReasonMissingPolicy, cond.Reason)
}

func TestReconcileStrategyNotFound(t *testing.T) {
	ctx := context.Background()

	// Policy exists but selected strategy does not.
	cappConfig := newCappConfig("present-strategy", "absent-strategy")
	cb := newCappBuild("cb", "ns", rcs.CappBuildFileModeAbsent)

	r, c := newReconciler(t, cb, cappConfig)
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: cb.Name, Namespace: cb.Namespace}})
	require.NoError(t, err)
	require.Greater(t, res.RequeueAfter, time.Duration(0))

	latest := &rcs.CappBuild{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(cb), latest))
	cond := meta.FindStatusCondition(latest.Status.Conditions, TypeReady)
	require.NotNil(t, cond, "Ready condition should be set")
	require.Equal(t, metav1.ConditionFalse, cond.Status)
	require.Equal(t, ReasonBuildStrategyNotFound, cond.Reason)
}

func TestReconcileBuildConflict(t *testing.T) {
	ctx := context.Background()

	absentStrategy := "absent-strategy"
	cappConfig := newCappConfig("present-strategy", absentStrategy)
	cb := newCappBuild("cb", "ns", rcs.CappBuildFileModeAbsent)

	// Strategy exists -> pass precondition.
	clusterBuildStrategy := &shipwright.ClusterBuildStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: absentStrategy},
	}

	// Existing Build is already controlled by someone else.
	controller := true
	existingBuild := &shipwright.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildNameFor(cb),
			Namespace: cb.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: rcs.GroupVersion.String(),
					Kind:       "CappBuild",
					Name:       "someone-else",
					UID:        types.UID("other-uid"),
					Controller: &controller,
				},
			},
		},
	}

	r, c := newReconciler(t, cb, cappConfig, clusterBuildStrategy, existingBuild)

	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: cb.Name, Namespace: cb.Namespace}})
	require.NoError(t, err)
	require.Equal(t, time.Duration(0), res.RequeueAfter)

	latest := &rcs.CappBuild{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(cb), latest))
	cond := meta.FindStatusCondition(latest.Status.Conditions, TypeReady)
	require.NotNil(t, cond, "Ready condition should be set")
	require.Equal(t, metav1.ConditionFalse, cond.Status)
	require.Equal(t, ReasonBuildConflict, cond.Reason)
}

func TestReconcileCreatesBuild(t *testing.T) {
	ctx := context.Background()

	absentStrategy := "absent-strategy"
	cappConfig := newCappConfig("present-strategy", absentStrategy)
	cb := newCappBuild("cb", "ns", rcs.CappBuildFileModeAbsent)

	clusterBuildStrategy := &shipwright.ClusterBuildStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: absentStrategy},
	}

	r, c := newReconciler(t, cb, cappConfig, clusterBuildStrategy)
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: cb.Name, Namespace: cb.Namespace}})
	require.NoError(t, err)
	require.Equal(t, time.Duration(0), res.RequeueAfter)

	// CappBuild status contract
	latest := &rcs.CappBuild{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(cb), latest))
	require.Equal(t, latest.Generation, latest.Status.ObservedGeneration)
	require.Equal(t, cb.Namespace+"/"+buildNameFor(cb), latest.Status.BuildRef)

	// Shipwright Build contract
	build := &shipwright.Build{}
	require.NoError(t, c.Get(ctx, types.NamespacedName{Name: buildNameFor(cb), Namespace: cb.Namespace}, build))
	require.True(t, metav1.IsControlledBy(build, latest), "Build should be controller-owned by CappBuild")
	require.Equal(t, absentStrategy, build.Spec.Strategy.Name)
}
```

## 2) E2E tests for CappBuild controller (minimal integration contracts)

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/test/e2e_tests/cappbuild_e2e_test.go`

Replace the current single test with a single test that covers both:
- initial create (Absent mode) and
- update on mode flip (Present).

```go
package e2e_tests

import (
	"context"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"

	shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
)

func newCappBuild(name string, mode cappv1alpha1.CappBuildFileMode) *cappv1alpha1.CappBuild {
	return &cappv1alpha1.CappBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testconsts.NSName,
		},
		Spec: cappv1alpha1.CappBuildSpec{
			BuildFile: cappv1alpha1.CappBuildFileSpec{
				Mode: mode,
			},
			Source: cappv1alpha1.CappBuildSource{
				Type: cappv1alpha1.CappBuildSourceTypeGit,
				Git: cappv1alpha1.CappBuildGitSource{
					URL: "https://github.com/dana-team/container-app-operator",
				},
			},
			Output: cappv1alpha1.CappBuildOutputSpec{
				Image: "registry.example.com/team/cappbuild-e2e",
			},
		},
	}
}

var _ = Describe("Validate CappBuild controller", func() {
	It("Creates Shipwright Build and updates strategy on mode change", func() {
		ctx := context.Background()

		// Read platform policy (assumed present in the cluster running e2e).
		cappConfig := utilst.GetCappConfig(k8sClient, testconsts.CappConfigName, testconsts.ControllerNS)
		Expect(cappConfig.Spec.CappBuild).ToNot(BeNil())

		presentStrategy := cappConfig.Spec.CappBuild.ClusterBuildStrategy.BuildFile.Present
		absentStrategy := cappConfig.Spec.CappBuild.ClusterBuildStrategy.BuildFile.Absent
		Expect(presentStrategy).ToNot(BeEmpty())
		Expect(absentStrategy).ToNot(BeEmpty())

		name := utilst.RandomName("cappbuild")
		cb := newCappBuild(name, cappv1alpha1.CappBuildFileModeAbsent)

		Expect(k8sClient.Create(ctx, cb)).To(Succeed())

		cbKey := types.NamespacedName{Name: name, Namespace: testconsts.NSName}
		buildKey := types.NamespacedName{Name: name + "-build", Namespace: testconsts.NSName}

		Eventually(func(g Gomega) {
			latest := &cappv1alpha1.CappBuild{}
			g.Expect(k8sClient.Get(ctx, cbKey, latest)).To(Succeed())
			g.Expect(latest.Status.ObservedGeneration).To(Equal(latest.Generation))
			g.Expect(latest.Status.BuildRef).To(Equal(testconsts.NSName + "/" + name + "-build"))

			build := &shipwright.Build{}
			g.Expect(k8sClient.Get(ctx, buildKey, build)).To(Succeed())
			g.Expect(build.Spec.Strategy.Name).To(Equal(absentStrategy))
		}, testconsts.Timeout, testconsts.Interval).Should(Succeed())

		// Flip mode to Present and expect strategy update.
		// Use retry-on-conflict to avoid spurious flakes when controller updates status.
		Expect(retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			latest := &cappv1alpha1.CappBuild{}
			if err := k8sClient.Get(ctx, cbKey, latest); err != nil {
				return err
			}
			latest.Spec.BuildFile.Mode = cappv1alpha1.CappBuildFileModePresent
			return k8sClient.Update(ctx, latest)
		})).To(Succeed())

		Eventually(func(g Gomega) {
			build := &shipwright.Build{}
			g.Expect(k8sClient.Get(ctx, buildKey, build)).To(Succeed())
			g.Expect(build.Spec.Strategy.Name).To(Equal(presentStrategy))
		}, testconsts.Timeout, testconsts.Interval).Should(Succeed())
	})
})
```

## 3) Run tests

Unit tests (exclude e2e suite):

```bash
cd /home/sbahar/projects/ps/dana-team/container-app-operator
go test $(go list ./... | grep -v '/test/e2e_tests')
```

E2E tests (requires a configured cluster + operator deployed):

```bash
cd /home/sbahar/projects/ps/dana-team/container-app-operator
go test ./test/e2e_tests -run TestE2E
```


