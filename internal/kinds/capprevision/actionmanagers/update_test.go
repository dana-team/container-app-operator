package actionmanagers

import (
	"context"
	"testing"
	"time"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSplitRevisionsAtIndex(t *testing.T) {
	revisions := []cappv1alpha1.CappRevision{
		{ObjectMeta: metav1.ObjectMeta{Name: "rev-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "rev-2"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "rev-3"}},
	}

	tests := []struct {
		name          string
		index         int
		wantBeforeLen int
		wantAfterLen  int
	}{
		{
			name:          "split in the middle",
			index:         2,
			wantBeforeLen: 2,
			wantAfterLen:  1,
		},
		{
			name:          "split at zero returns all in after",
			index:         0,
			wantBeforeLen: 0,
			wantAfterLen:  3,
		},
		{
			name:          "split at negative returns all in after",
			index:         -1,
			wantBeforeLen: 0,
			wantAfterLen:  3,
		},
		{
			name:          "split beyond length returns all in before",
			index:         5,
			wantBeforeLen: 3,
			wantAfterLen:  0,
		},
		{
			name:          "split at exact length returns all in before",
			index:         3,
			wantBeforeLen: 3,
			wantAfterLen:  0,
		},
		{
			name:          "split at one",
			index:         1,
			wantBeforeLen: 1,
			wantAfterLen:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, after := splitRevisionsAtIndex(revisions, tt.index)
			require.Len(t, before, tt.wantBeforeLen)
			require.Len(t, after, tt.wantAfterLen)
		})
	}
}

func TestSortByCreationTime(t *testing.T) {
	now := time.Now()
	revisions := []cappv1alpha1.CappRevision{
		{ObjectMeta: metav1.ObjectMeta{Name: "oldest", CreationTimestamp: metav1.NewTime(now.Add(-2 * time.Hour))}},
		{ObjectMeta: metav1.ObjectMeta{Name: "newest", CreationTimestamp: metav1.NewTime(now)}},
		{ObjectMeta: metav1.ObjectMeta{Name: "middle", CreationTimestamp: metav1.NewTime(now.Add(-1 * time.Hour))}},
	}

	sortByCreationTime(revisions)

	require.Equal(t, "newest", revisions[0].Name)
	require.Equal(t, "middle", revisions[1].Name)
	require.Equal(t, "oldest", revisions[2].Name)
}

func TestEqualSpec(t *testing.T) {
	capp := newBaseCapp()

	t.Run("returns true for identical specs", func(t *testing.T) {
		revisionSpec := *capp.Spec.DeepCopy()
		require.True(t, equalSpec(capp.Spec, revisionSpec))
	})

	t.Run("returns false when spec differs", func(t *testing.T) {
		revisionSpec := *capp.Spec.DeepCopy()
		revisionSpec.State = stateDisabled
		require.False(t, equalSpec(capp.Spec, revisionSpec))
	})
}

func TestEqualAnnotations(t *testing.T) {
	t.Run("returns true for identical annotations", func(t *testing.T) {
		a := map[string]string{annotationKeyKey: annotationKeyVal}
		b := map[string]string{annotationKeyKey: annotationKeyVal}
		require.True(t, equalAnnotations(a, b, annotationToIgnore))
	})

	t.Run("returns false when annotations differ", func(t *testing.T) {
		a := map[string]string{annotationKeyKey: annotationKeyValA}
		b := map[string]string{annotationKeyKey: annotationKeyValB}
		require.False(t, equalAnnotations(a, b, annotationToIgnore))
	})

	t.Run("ignores the last-updated-by annotation", func(t *testing.T) {
		a := map[string]string{annotationToIgnore: annotationUserA, annotationOtherKey: annotationSameVal}
		b := map[string]string{annotationToIgnore: annotationUserB, annotationOtherKey: annotationSameVal}
		require.True(t, equalAnnotations(a, b, annotationToIgnore))
	})

	t.Run("returns true for both nil", func(t *testing.T) {
		require.True(t, equalAnnotations(nil, nil, annotationToIgnore))
	})

	t.Run("returns true when only ignored key present on one side", func(t *testing.T) {
		a := map[string]string{annotationToIgnore: annotationUserA}
		b := map[string]string{}
		require.True(t, equalAnnotations(a, b, annotationToIgnore))
	})
}

func TestEqualLabels(t *testing.T) {
	t.Run("returns true for identical labels", func(t *testing.T) {
		a := map[string]string{labelTeamKey: labelTeamPlatform}
		b := map[string]string{labelTeamKey: labelTeamPlatform}
		require.True(t, equalLabels(a, b))
	})

	t.Run("returns false when labels differ", func(t *testing.T) {
		a := map[string]string{labelTeamKey: labelTeamPlatform}
		b := map[string]string{labelTeamKey: labelTeamInfra}
		require.False(t, equalLabels(a, b))
	})

	t.Run("returns true for both nil", func(t *testing.T) {
		require.True(t, equalLabels(nil, nil))
	})
}

func TestIsEqual(t *testing.T) {
	capp := newBaseCapp()
	capp.Labels = map[string]string{labelTeamKey: labelTeamPlatform}
	capp.Annotations = map[string]string{annotationNoteKey: annotationNoteHi}

	t.Run("returns true when capp matches revision", func(t *testing.T) {
		rev := *newCappRevision("rev-1", 1, capp, time.Now())
		require.True(t, isEqual(capp, rev))
	})

	t.Run("returns false when spec differs", func(t *testing.T) {
		rev := *newCappRevision("rev-1", 1, capp, time.Now())
		rev.Spec.CappTemplate.Spec.State = stateDisabled
		require.False(t, isEqual(capp, rev))
	})

	t.Run("returns false when labels differ", func(t *testing.T) {
		rev := *newCappRevision("rev-1", 1, capp, time.Now())
		rev.Spec.CappTemplate.Labels = map[string]string{labelTeamKey: labelTeamInfra}
		require.False(t, isEqual(capp, rev))
	})

	t.Run("returns false when annotations differ", func(t *testing.T) {
		rev := *newCappRevision("rev-1", 1, capp, time.Now())
		rev.Spec.CappTemplate.Annotations = map[string]string{annotationNoteKey: "changed"}
		require.False(t, isEqual(capp, rev))
	})

	t.Run("ignores last-updated-by annotation difference", func(t *testing.T) {
		cappWithUpdatedBy := capp.DeepCopy()
		cappWithUpdatedBy.Annotations = map[string]string{annotationNoteKey: annotationNoteHi, annotationToIgnore: annotationUserA}

		rev := *newCappRevision("rev-1", 1, *cappWithUpdatedBy, time.Now())
		rev.Spec.CappTemplate.Annotations[annotationToIgnore] = annotationUserB
		require.True(t, isEqual(*cappWithUpdatedBy, rev))
	})
}

func TestHandleCappUpdate(t *testing.T) {
	ctx := context.Background()
	logger := logr.Discard()
	now := time.Now()

	t.Run("no-op when capp equals latest revision", func(t *testing.T) {
		capp := newBaseCapp()
		rev := newCappRevision("rev-00001", 1, capp, now)

		k8sClient := newFakeClient(newCappConfig(10), rev)
		revisions := []cappv1alpha1.CappRevision{*rev}

		require.NoError(t, HandleCappUpdate(ctx, k8sClient, capp, logger, revisions))

		revList := &cappv1alpha1.CappRevisionList{}
		require.NoError(t, k8sClient.List(ctx, revList))
		require.Len(t, revList.Items, 1)
	})

	t.Run("creates new revision when spec changes and under limit", func(t *testing.T) {
		capp := newBaseCapp()
		rev := newCappRevision("rev-00001", 1, capp, now)

		k8sClient := newFakeClient(newCappConfig(10), rev)
		revisions := []cappv1alpha1.CappRevision{*rev}

		capp.Spec.State = stateDisabled
		require.NoError(t, HandleCappUpdate(ctx, k8sClient, capp, logger, revisions))

		revList := &cappv1alpha1.CappRevisionList{}
		require.NoError(t, k8sClient.List(ctx, revList))
		require.Len(t, revList.Items, 2)
	})

	t.Run("creates new revision when labels change", func(t *testing.T) {
		capp := newBaseCapp()
		capp.Labels = map[string]string{labelTeamKey: labelTeamPlatform}
		rev := newCappRevision("rev-00001", 1, capp, now)

		k8sClient := newFakeClient(newCappConfig(10), rev)
		revisions := []cappv1alpha1.CappRevision{*rev}

		capp.Labels = map[string]string{labelTeamKey: labelTeamInfra}
		require.NoError(t, HandleCappUpdate(ctx, k8sClient, capp, logger, revisions))

		revList := &cappv1alpha1.CappRevisionList{}
		require.NoError(t, k8sClient.List(ctx, revList))
		require.Len(t, revList.Items, 2)
	})

	t.Run("creates new revision when annotations change", func(t *testing.T) {
		capp := newBaseCapp()
		capp.Annotations = map[string]string{annotationNoteKey: "v1"}
		rev := newCappRevision("rev-00001", 1, capp, now)

		k8sClient := newFakeClient(newCappConfig(10), rev)
		revisions := []cappv1alpha1.CappRevision{*rev}

		capp.Annotations = map[string]string{annotationNoteKey: "v2"}
		require.NoError(t, HandleCappUpdate(ctx, k8sClient, capp, logger, revisions))

		revList := &cappv1alpha1.CappRevisionList{}
		require.NoError(t, k8sClient.List(ctx, revList))
		require.Len(t, revList.Items, 2)
	})

	t.Run("prunes oldest revisions when at limit", func(t *testing.T) {
		capp := newBaseCapp()
		limit := 3

		rev1 := newCappRevision("rev-00001", 1, capp, now.Add(-2*time.Hour))
		rev2 := newCappRevision("rev-00002", 2, capp, now.Add(-1*time.Hour))
		rev3 := newCappRevision("rev-00003", 3, capp, now)

		k8sClient := newFakeClient(newCappConfig(limit), rev1, rev2, rev3)
		revisions := []cappv1alpha1.CappRevision{*rev1, *rev2, *rev3}

		capp.Spec.State = stateDisabled
		require.NoError(t, HandleCappUpdate(ctx, k8sClient, capp, logger, revisions))

		revList := &cappv1alpha1.CappRevisionList{}
		require.NoError(t, k8sClient.List(ctx, revList))
		require.Len(t, revList.Items, limit)
	})

	t.Run("prunes excess revisions when over limit", func(t *testing.T) {
		capp := newBaseCapp()
		limit := 2

		rev1 := newCappRevision("rev-00001", 1, capp, now.Add(-3*time.Hour))
		rev2 := newCappRevision("rev-00002", 2, capp, now.Add(-2*time.Hour))
		rev3 := newCappRevision("rev-00003", 3, capp, now.Add(-1*time.Hour))
		rev4 := newCappRevision("rev-00004", 4, capp, now)

		k8sClient := newFakeClient(newCappConfig(limit), rev1, rev2, rev3, rev4)
		revisions := []cappv1alpha1.CappRevision{*rev1, *rev2, *rev3, *rev4}

		capp.Spec.State = stateDisabled
		require.NoError(t, HandleCappUpdate(ctx, k8sClient, capp, logger, revisions))

		revList := &cappv1alpha1.CappRevisionList{}
		require.NoError(t, k8sClient.List(ctx, revList))
		require.Len(t, revList.Items, limit)
	})

	t.Run("new revision number is based on latest kept revision", func(t *testing.T) {
		capp := newBaseCapp()
		limit := 2

		rev1 := newCappRevision("rev-00001", 1, capp, now.Add(-1*time.Hour))
		rev2 := newCappRevision("rev-00002", 2, capp, now)

		k8sClient := newFakeClient(newCappConfig(limit), rev1, rev2)
		revisions := []cappv1alpha1.CappRevision{*rev1, *rev2}

		capp.Spec.State = stateDisabled
		require.NoError(t, HandleCappUpdate(ctx, k8sClient, capp, logger, revisions))

		revList := &cappv1alpha1.CappRevisionList{}
		require.NoError(t, k8sClient.List(ctx, revList))

		var maxRevNum int
		for _, r := range revList.Items {
			if r.Spec.RevisionNumber > maxRevNum {
				maxRevNum = r.Spec.RevisionNumber
			}
		}
		require.Equal(t, 3, maxRevNum)
	})

	t.Run("returns error when capp config is missing", func(t *testing.T) {
		capp := newBaseCapp()
		rev := newCappRevision("rev-00001", 1, capp, now)

		k8sClient := newFakeClient(rev)
		revisions := []cappv1alpha1.CappRevision{*rev}

		capp.Spec.State = stateDisabled
		require.Error(t, HandleCappUpdate(ctx, k8sClient, capp, logger, revisions))
	})
}
