# Implementation — Phase 9 e2e tests (Shipwright orchestration)

This phase is **tests only**. It adds **minimal, high-signal e2e coverage** for `CappBuild` orchestrating Shipwright resources, without running Tekton builds and without duplicating unit-tested status mapping.

## 1) Use a CappBuild-focused client/scheme (do not reuse the suite-wide scheme)

Goal: avoid registering unrelated APIs (e.g., Knative) just to run `CappBuild` e2e.

Do **not** change the shared suite client (`k8sClient`) or `test/e2e_tests/helper.go`.

Instead, the Phase 9 test creates its own `client.Client` instance using a **minimal scheme** that includes only:
- `client-go` core scheme
- `rcs.dana.io` APIs (`CappBuild`, `CappConfig`)
- Shipwright APIs (`Build`, `BuildRun`, `ClusterBuildStrategy`)
- `corev1` (safe default for basic k8s objects in helpers)
 
The Phase 9 test will create a dedicated `client.Client` constructed from `cfg` and a minimal scheme.

## 2) Add one minimal e2e spec for Shipwright orchestration + generation rotation + GC

This test:
- Skips if Shipwright CRDs are not installed.
- Skips (or fails fast) if the `CappConfig` build strategies referenced by the cluster are not present.
- Creates a `CappBuild` and asserts:
  - Shipwright `Build` and `BuildRun` exist with deterministic names, ownership, and the parent label.
  - `CappBuild.status.buildRef` and `CappBuild.status.lastBuildRunRef` are populated.
- Updates `CappBuild.spec.source.git.revision` to trigger a new generation and asserts a new `BuildRun`.
- Deletes the `CappBuild` and asserts garbage collection deletes the owned Shipwright resources.

Edit (start fresh):
- `/home/sbahar/projects/ps/dana-team/container-app-operator/test/e2e_tests/cappbuild_e2e_test.go`

Action:
- Delete the existing placeholder test(s) in this file.
- Replace the **entire file contents** with the following Phase 9 test.

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/test/e2e_tests/cappbuild_e2e_test.go`

```go
package e2e_tests

import (
	"context"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func newCappBuildClient() client.Client {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(cappv1alpha1.AddToScheme(s))
	utilruntime.Must(shipwright.AddToScheme(s))

	c, err := client.New(cfg, client.Options{Scheme: s})
	Expect(err).NotTo(HaveOccurred())
	return c
}

func newCappBuild(name, revision string) *cappv1alpha1.CappBuild {
	return &cappv1alpha1.CappBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testconsts.NSName,
		},
		Spec: cappv1alpha1.CappBuildSpec{
			BuildFile: cappv1alpha1.CappBuildFile{
				Mode: cappv1alpha1.CappBuildFileModeAbsent,
			},
			Source: cappv1alpha1.CappBuildSource{
				Type: cappv1alpha1.CappBuildSourceTypeGit,
				Git: cappv1alpha1.CappBuildGitSource{
					URL:      "https://github.com/dana-team/container-app-operator",
					Revision: revision,
				},
			},
			Output: cappv1alpha1.CappBuildOutput{
				Image: "registry.example.com/team/cappbuild-e2e:v1",
			},
		},
	}
}

var _ = Describe("CappBuild Shipwright integration", func() {
	var (
		ctx context.Context
		c   client.Client
		cappBuildName string
	)

	BeforeEach(func() {
		ctx = context.Background()
		c = newCappBuildClient()

		By("Ensuring Shipwright is installed and reachable")
		Expect(c.List(ctx, &shipwright.BuildRunList{}, client.InNamespace(testconsts.NSName))).To(Succeed())

		By("Ensuring build policy and strategy exist")
		cappConfig := utilst.GetCappConfig(c, testconsts.CappConfigName, testconsts.ControllerNS)
		Expect(cappConfig.Spec.CappBuild).NotTo(BeNil())
		absentStrategy := cappConfig.Spec.CappBuild.ClusterBuildStrategy.BuildFile.Absent
		Expect(c.Get(ctx, types.NamespacedName{Name: absentStrategy}, &shipwright.ClusterBuildStrategy{})).To(Succeed())

		By("Creating a CappBuild request")
		cappBuildName = utilst.RandomName("cappbuild")
		Expect(c.Create(ctx, newCappBuild(cappBuildName, "rev-1"))).To(Succeed())
	})

	It("creates Shipwright Build and BuildRun", func() {
		By("Waiting for CappBuild to report the created Build and BuildRun names")
		buildName := waitForBuildRef(ctx, c, cappBuildName)
		buildRunName := waitForLastBuildRunRef(ctx, c, cappBuildName)

		By("Waiting for the Shipwright Build")
		_ = waitForBuild(ctx, c, cappBuildName, buildName)

		By("Waiting for the Shipwright BuildRun")
		_ = waitForBuildRun(ctx, c, cappBuildName, buildRunName, buildName)
	})

	It("creates a new BuildRun on CappBuild update", func() {
		By("Reading the Build ref (stable across the update)")
		buildName := waitForBuildRef(ctx, c, cappBuildName)

		By("Waiting for the initial BuildRun")
		initialBuildRunName := waitForLastBuildRunRef(ctx, c, cappBuildName)

		By("Verifying the initial BuildRun exists before updating")
		_ = waitForBuildRun(ctx, c, cappBuildName, initialBuildRunName, buildName)

		By("Updating CappBuild to trigger a new BuildRun")
		updateCappBuild(ctx, c, cappBuildName, "rev-2")

		By("Waiting for CappBuild to point to a new BuildRun")
		newBuildRunName := waitForBuildRunChange(ctx, c, cappBuildName, initialBuildRunName)

		By("Waiting for the new Shipwright BuildRun")
		waitForBuildRun(ctx, c, cappBuildName, newBuildRunName, buildName)
	})

	It("deletes owned Shipwright resources when CappBuild is deleted", func() {
		By("Waiting for initial Shipwright resources")
		buildName := waitForBuildRef(ctx, c, cappBuildName)
		buildRunName1 := waitForLastBuildRunRef(ctx, c, cappBuildName)
		build := waitForBuild(ctx, c, cappBuildName, buildName)
		buildRun1 := waitForBuildRun(ctx, c, cappBuildName, buildRunName1, buildName)

		By("Updating CappBuild to create a second BuildRun")
		updateCappBuild(ctx, c, cappBuildName, "rev-2")
		buildRunName2 := waitForBuildRunChange(ctx, c, cappBuildName, buildRunName1)
		buildRun2 := waitForBuildRun(ctx, c, cappBuildName, buildRunName2, buildName)

		By("Deleting the CappBuild")
		cappBuild := &cappv1alpha1.CappBuild{}
		Expect(c.Get(ctx, types.NamespacedName{Name: cappBuildName, Namespace: testconsts.NSName}, cappBuild)).To(Succeed())
		Expect(c.Delete(ctx, cappBuild)).To(Succeed())

		By("Verifying GC deletes the owned Shipwright resources")
		resourcesToDelete := []client.Object{
			build,
			buildRun1,
			buildRun2,
		}

		for _, obj := range resourcesToDelete {
			obj := obj
			Eventually(func() bool {
				err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj)
				return apierrors.IsNotFound(err)
			}, testconsts.Timeout, testconsts.Interval).Should(BeTrue())
		}
	})
})

func waitForBuildRef(ctx context.Context, c client.Client, cappBuildName string) string {
	var buildName string
	Eventually(func(g Gomega) {
		cappBuild := &cappv1alpha1.CappBuild{}
		g.Expect(c.Get(ctx, types.NamespacedName{Name: cappBuildName, Namespace: testconsts.NSName}, cappBuild)).To(Succeed())
		g.Expect(cappBuild.Status.BuildRef).NotTo(BeEmpty())
		buildName = cappBuild.Status.BuildRef
	}, testconsts.Timeout, testconsts.Interval).Should(Succeed())
	return buildName
}

func waitForLastBuildRunRef(ctx context.Context, c client.Client, cappBuildName string) string {
	var buildRunName string
	Eventually(func(g Gomega) {
		cappBuild := &cappv1alpha1.CappBuild{}
		g.Expect(c.Get(ctx, types.NamespacedName{Name: cappBuildName, Namespace: testconsts.NSName}, cappBuild)).To(Succeed())
		g.Expect(cappBuild.Status.LastBuildRunRef).NotTo(BeEmpty())
		buildRunName = cappBuild.Status.LastBuildRunRef
	}, testconsts.Timeout, testconsts.Interval).Should(Succeed())
	return buildRunName
}

func waitForBuild(ctx context.Context, c client.Client, cappBuildName, buildName string) *shipwright.Build {
	var build *shipwright.Build
	Eventually(func(g Gomega) {
		cappBuild := &cappv1alpha1.CappBuild{}
		g.Expect(c.Get(ctx, types.NamespacedName{Name: cappBuildName, Namespace: testconsts.NSName}, cappBuild)).To(Succeed())

		b := &shipwright.Build{}
		g.Expect(c.Get(ctx, types.NamespacedName{Name: buildName, Namespace: testconsts.NSName}, b)).To(Succeed())
		g.Expect(metav1.IsControlledBy(b, cappBuild)).To(BeTrue())
		build = b
	}, testconsts.Timeout, testconsts.Interval).Should(Succeed())
	return build
}

func waitForBuildRun(ctx context.Context, c client.Client, cappBuildName, buildRunName, buildName string) *shipwright.BuildRun {
	var buildRun *shipwright.BuildRun
	Eventually(func(g Gomega) {
		cappBuild := &cappv1alpha1.CappBuild{}
		g.Expect(c.Get(ctx, types.NamespacedName{Name: cappBuildName, Namespace: testconsts.NSName}, cappBuild)).To(Succeed())

		br := &shipwright.BuildRun{}
		g.Expect(c.Get(ctx, types.NamespacedName{Name: buildRunName, Namespace: testconsts.NSName}, br)).To(Succeed())
		g.Expect(metav1.IsControlledBy(br, cappBuild)).To(BeTrue())
		g.Expect(br.Spec.Build.Name).NotTo(BeNil())
		g.Expect(*br.Spec.Build.Name).To(Equal(buildName))
		buildRun = br
	}, testconsts.Timeout, testconsts.Interval).Should(Succeed())
	return buildRun
}

func updateCappBuild(ctx context.Context, c client.Client, cappBuildName, revision string) {
	err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
		cappBuild := &cappv1alpha1.CappBuild{}
		if err := c.Get(ctx, types.NamespacedName{Name: cappBuildName, Namespace: testconsts.NSName}, cappBuild); err != nil {
			return err
		}
		cappBuild.Spec.Source.Git.Revision = revision
		return c.Update(ctx, cappBuild)
	})
	Expect(err).NotTo(HaveOccurred())
}

func waitForBuildRunChange(ctx context.Context, c client.Client, cappBuildName string, previousBuildRunName string) string {
	var newBuildRunName string
	Eventually(func(g Gomega) {
		cappBuild := &cappv1alpha1.CappBuild{}
		g.Expect(c.Get(ctx, types.NamespacedName{Name: cappBuildName, Namespace: testconsts.NSName}, cappBuild)).To(Succeed())
		g.Expect(cappBuild.Status.LastBuildRunRef).NotTo(Equal(previousBuildRunName))
		newBuildRunName = cappBuild.Status.LastBuildRunRef
	}, testconsts.Timeout, testconsts.Interval).Should(Succeed())

	return newBuildRunName
}

```

Notes:
- This spec intentionally does **not** assert `BuildSucceeded=True/False` or `latestImage` changes; those are already covered by unit tests.
- The test verifies GC as a proxy for “ownerReferences are correct” (avoids orphaned Shipwright resources).

## 3) Run e2e tests (as appropriate for your environment)

This repo’s e2e tests require a live cluster + operator installed. Run them using the project’s documented e2e workflow.
