package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestTriggerNaming(t *testing.T) {
	cb := newCappBuild("cb", "ns")
	cb.Spec.Rebuild = &rcs.CappBuildRebuild{Mode: rcs.CappBuildRebuildModeOnCommit}
	cb.Status.OnCommit = &rcs.CappBuildOnCommitStatus{
		Pending: &rcs.CappBuildOnCommitEvent{Ref: "refs/heads/main", CommitSHA: "abc"},
	}

	cfg := newCappConfig()
	r, _ := newReconciler(t, cfg, cb)

	br, requeue, err := r.triggerBuildRun(context.Background(), cb)
	require.NoError(t, err)
	require.Nil(t, requeue)
	require.NotNil(t, br)
	require.Equal(t, fmt.Sprintf("%s-buildrun-oncommit-1", cb.Name), br.Name)
	require.Equal(t, "oncommit", br.Labels["rcs.dana.io/build-trigger"])
}

func TestTriggerActiveBuild(t *testing.T) {
	cb := newCappBuild("cb", "ns")
	cb.Spec.Rebuild = &rcs.CappBuildRebuild{Mode: rcs.CappBuildRebuildModeOnCommit}
	cb.Status.LastBuildRunRef = "active-br"
	cb.Status.OnCommit = &rcs.CappBuildOnCommitStatus{
		Pending: &rcs.CappBuildOnCommitEvent{Ref: "refs/heads/main", CommitSHA: "abc"},
	}

	activeBR := &shipwright.BuildRun{
		ObjectMeta: metav1.ObjectMeta{Name: "active-br", Namespace: cb.Namespace},
	}
	require.NoError(t, controllerutil.SetControllerReference(cb, activeBR, testScheme(t)))

	cfg := newCappConfig()
	r, _ := newReconciler(t, cfg, cb, activeBR)

	br, requeue, err := r.triggerBuildRun(context.Background(), cb)
	require.NoError(t, err)
	require.Nil(t, requeue)
	require.NotNil(t, br, "should return the active BuildRun for status mapping")
	require.Equal(t, activeBR.Name, br.Name)
}

func TestTriggerDebounce(t *testing.T) {
	now := time.Now()
	cb := newCappBuild("cb", "ns")
	cb.Spec.Rebuild = &rcs.CappBuildRebuild{Mode: rcs.CappBuildRebuildModeOnCommit}
	cb.Status.OnCommit = &rcs.CappBuildOnCommitStatus{
		Pending: &rcs.CappBuildOnCommitEvent{
			Ref:        "refs/heads/main",
			CommitSHA:  "abc",
			ReceivedAt: metav1.NewTime(now),
		},
	}

	cfg := newCappConfig()
	r, _ := newReconciler(t, cfg, cb)

	br, requeue, err := r.triggerBuildRun(context.Background(), cb)
	require.NoError(t, err)
	require.NotNil(t, requeue, "should requeue for debounce")
	require.Nil(t, br)
	require.True(t, *requeue > 0)
}
