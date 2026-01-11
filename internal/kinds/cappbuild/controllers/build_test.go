package controllers

import (
	"context"
	"testing"
	"time"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestReconcileMissingPolicy(t *testing.T) {
	ctx := context.Background()

	cb := newCappBuild("cb-"+t.Name(), "ns-"+t.Name())

	r, c := newReconciler(t, cb)
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: cb.Name, Namespace: cb.Namespace}})
	require.NoError(t, err)
	require.Greater(t, res.RequeueAfter, time.Duration(0))

	latest := &rcs.CappBuild{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(cb), latest))
	requireCondition(t, latest.Status.Conditions, TypeReady, metav1.ConditionFalse, ReasonMissingPolicy)
}

func TestReconcileStrategyNotFound(t *testing.T) {
	ctx := context.Background()

	cappConfig := newCappConfig()
	cb := newCappBuild("cb-"+t.Name(), "ns-"+t.Name())

	r, c := newReconciler(t, cb, cappConfig)
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: cb.Name, Namespace: cb.Namespace}})
	require.NoError(t, err)
	require.Greater(t, res.RequeueAfter, time.Duration(0))

	latest := &rcs.CappBuild{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(cb), latest))
	requireCondition(t, latest.Status.Conditions, TypeReady, metav1.ConditionFalse, ReasonBuildStrategyNotFound)
}

func TestReconcileBuildConflict(t *testing.T) {
	ctx := context.Background()

	cappConfig := newCappConfig()
	cb := newCappBuild("cb-"+t.Name(), "ns-"+t.Name())

	clusterBuildStrategy := &shipwright.ClusterBuildStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: absentStrategy},
	}

	otherOwner := &rcs.CappBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name: "someone-else",
			UID:  types.UID("other-uid"),
		},
	}

	conflictingBuild := &shipwright.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildNameFor(cb),
			Namespace: cb.Namespace,
		},
	}
	require.NoError(t, controllerutil.SetControllerReference(otherOwner, conflictingBuild, testScheme(t)))

	r, c := newReconciler(t, cb, cappConfig, clusterBuildStrategy, conflictingBuild)

	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: cb.Name, Namespace: cb.Namespace}})
	require.NoError(t, err)
	require.Equal(t, time.Duration(0), res.RequeueAfter)

	latest := &rcs.CappBuild{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(cb), latest))
	requireCondition(t, latest.Status.Conditions, TypeReady, metav1.ConditionFalse, ReasonBuildConflict)
}

func TestReconcileCreatesBuild(t *testing.T) {
	ctx := context.Background()

	cappConfig := newCappConfig()
	cb := newCappBuild("cb-"+t.Name(), "ns-"+t.Name())

	clusterBuildStrategy := &shipwright.ClusterBuildStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: absentStrategy},
	}

	r, c := newReconciler(t, cb, cappConfig, clusterBuildStrategy)
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: cb.Name, Namespace: cb.Namespace}})
	require.NoError(t, err)

	latest := &rcs.CappBuild{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(cb), latest))
	require.Equal(t, latest.Generation, latest.Status.ObservedGeneration)
	require.Equal(t, cb.Namespace+"/"+buildNameFor(cb), latest.Status.BuildRef)

	build := &shipwright.Build{}
	require.NoError(t, c.Get(ctx, types.NamespacedName{Name: buildNameFor(cb), Namespace: cb.Namespace}, build))
	require.Equal(t, buildNameFor(cb), build.Name)
	require.Equal(t, cb.Namespace, build.Namespace)
	require.True(t, metav1.IsControlledBy(build, latest), "Build should be controller-owned by CappBuild")
	require.Equal(t, absentStrategy, build.Spec.Strategy.Name)
}

func TestReconcileUpdatesBuild(t *testing.T) {
	ctx := context.Background()

	cappConfig := newCappConfig()
	cb := newCappBuild("cb-"+t.Name(), "ns-"+t.Name())

	clusterBuildStrategy := &shipwright.ClusterBuildStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: absentStrategy},
	}

	r, c := newReconciler(t, cb, cappConfig, clusterBuildStrategy)

	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: cb.Name, Namespace: cb.Namespace}})
	require.NoError(t, err)

	latest := &rcs.CappBuild{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(cb), latest))

	build := &shipwright.Build{}
	require.NoError(t, c.Get(ctx, types.NamespacedName{Name: buildNameFor(cb), Namespace: cb.Namespace}, build))
	require.Equal(t, cb.Spec.Source.Git.URL, build.Spec.Source.Git.URL)

	build.Spec.Source.Git.URL = "https://drifted-url.com"
	require.NoError(t, c.Update(ctx, build))

	_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: cb.Name, Namespace: cb.Namespace}})
	require.NoError(t, err)

	require.NoError(t, c.Get(ctx, types.NamespacedName{Name: buildNameFor(cb), Namespace: cb.Namespace}, build))
	require.Equal(t, cb.Spec.Source.Git.URL, build.Spec.Source.Git.URL)
	require.Equal(t, absentStrategy, build.Spec.Strategy.Name)
}
