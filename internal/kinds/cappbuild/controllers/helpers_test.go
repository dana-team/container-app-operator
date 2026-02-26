package controllers

import (
	"testing"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	capputils "github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
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
			BuildFile: rcs.CappBuildFile{Mode: rcs.CappBuildFileModeAbsent},
			Source: rcs.CappBuildSource{
				Type: rcs.CappBuildSourceTypeGit,
				Git:  rcs.CappBuildGitSource{URL: "https://example.invalid/repo.git"},
			},
			Output: rcs.CappBuildOutput{Image: "registry.example.com/team/app"},
		},
	}
}

func requireCondition(
	t *testing.T,
	conditions []metav1.Condition,
	condType string,
	status metav1.ConditionStatus,
	reason string,
) {
	t.Helper()

	cond := meta.FindStatusCondition(conditions, condType)
	require.NotNil(t, cond, "%s condition should be set", condType)
	require.Equal(t, status, cond.Status)
	require.Equal(t, reason, cond.Reason)
}
