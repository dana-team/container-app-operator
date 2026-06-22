package actionmanagers

import (
	"context"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
)

func TestHandleCappCreation(t *testing.T) {
	ctx := context.Background()
	logger := logr.Discard()

	t.Run("creates first revision with revision number 1", func(t *testing.T) {
		capp := newBaseCapp()
		k8sClient := newFakeClient()

		require.NoError(t, HandleCappCreation(ctx, k8sClient, capp, logger))

		revList := &cappv1alpha1.CappRevisionList{}
		require.NoError(t, k8sClient.List(ctx, revList))
		require.Len(t, revList.Items, 1)
		require.Equal(t, 1, revList.Items[0].Spec.RevisionNumber)
	})

	t.Run("revision captures capp spec", func(t *testing.T) {
		capp := newBaseCapp()
		capp.Spec.State = stateDisabled
		k8sClient := newFakeClient()

		require.NoError(t, HandleCappCreation(ctx, k8sClient, capp, logger))

		revList := &cappv1alpha1.CappRevisionList{}
		require.NoError(t, k8sClient.List(ctx, revList))
		require.Equal(t, capp.Spec, revList.Items[0].Spec.CappTemplate.Spec)
	})

	t.Run("revision captures capp labels and annotations", func(t *testing.T) {
		capp := newBaseCapp()
		capp.Labels = map[string]string{labelTeamKey: labelTeamPlatform}
		capp.Annotations = map[string]string{annotationNoteKey: annotationNoteHi}
		k8sClient := newFakeClient()

		require.NoError(t, HandleCappCreation(ctx, k8sClient, capp, logger))

		revList := &cappv1alpha1.CappRevisionList{}
		require.NoError(t, k8sClient.List(ctx, revList))
		require.Equal(t, capp.Labels, revList.Items[0].Spec.CappTemplate.Labels)
		require.Equal(t, capp.Annotations, revList.Items[0].Spec.CappTemplate.Annotations)
	})

	t.Run("revision has owner reference to capp", func(t *testing.T) {
		capp := newBaseCapp()
		k8sClient := newFakeClient()

		require.NoError(t, HandleCappCreation(ctx, k8sClient, capp, logger))

		revList := &cappv1alpha1.CappRevisionList{}
		require.NoError(t, k8sClient.List(ctx, revList))
		require.Len(t, revList.Items[0].OwnerReferences, 1)
		require.Equal(t, cappName, revList.Items[0].OwnerReferences[0].Name)
	})

	t.Run("revision has capp name label", func(t *testing.T) {
		capp := newBaseCapp()
		k8sClient := newFakeClient()

		require.NoError(t, HandleCappCreation(ctx, k8sClient, capp, logger))

		revList := &cappv1alpha1.CappRevisionList{}
		require.NoError(t, k8sClient.List(ctx, revList))
		labelKey := cappv1alpha1.GroupVersion.Group + "/cappName"
		require.Equal(t, cappName, revList.Items[0].Labels[labelKey])
	})
}
