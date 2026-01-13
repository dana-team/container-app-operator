package controllers

import (
	"context"
	"testing"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

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
	cb.UID = types.UID("cb-uid")
	existingBuildRun := newBuildRun(cb)
	existingBuildRun.UID = types.UID("existing-buildrun-uid")

	require.NoError(t, controllerutil.SetControllerReference(cb, existingBuildRun, testScheme(t)))

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

	otherOwner := &rcs.CappBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name: "someone-else",
			UID:  types.UID("other-uid"),
		},
	}
	require.NoError(t, controllerutil.SetControllerReference(otherOwner, conflict, testScheme(t)))

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

		requireCondition(t, cb.Status.Conditions, TypeBuildSucceeded, expectedStatus, expectedReason)
		buildSucceededCond := meta.FindStatusCondition(cb.Status.Conditions, TypeBuildSucceeded)
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
