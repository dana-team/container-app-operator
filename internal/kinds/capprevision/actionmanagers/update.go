package actionmanagers

import (
	"context"
	"sort"

	"k8s.io/apimachinery/pkg/api/equality"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capprevision/adapters"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const revisionsToKeep = 10

// splitRevisionsAtIndex splits a slice of CappRevisions into two slices:
// one containing the elements before the specified index (exclusive),
// and one containing the elements after the specified index (inclusive).
// If the index is out of bounds, it adjusts to return appropriate slices.
func splitRevisionsAtIndex(revisions []cappv1alpha1.CappRevision, index int) ([]cappv1alpha1.CappRevision, []cappv1alpha1.CappRevision) {
	if index <= 0 {
		return []cappv1alpha1.CappRevision{}, revisions
	}
	if index >= len(revisions) {
		return revisions, []cappv1alpha1.CappRevision{}
	}
	return revisions[:index], revisions[index:]
}

// sortByCreationTime sorts a slice of CappRevision by the CreatedAt field.
func sortByCreationTime(cappRevisions []cappv1alpha1.CappRevision) {
	sort.Slice(cappRevisions, func(i, j int) bool {
		return cappRevisions[j].CreationTimestamp.Time.Before(cappRevisions[i].CreationTimestamp.Time)
	})
}

// equalAnnotations returns a boolean indicating whether two Capp specs are equal.
func equalSpec(cappSpec, revisionCappSpec cappv1alpha1.CappSpec) bool {
	return equality.Semantic.DeepEqual(cappSpec, revisionCappSpec)
}

// equalAnnotations returns a boolean indicating whether two annotation maps are equal.
func equalAnnotations(cappAnnotations, revisionCappAnnotations map[string]string) bool {
	return equality.Semantic.DeepEqual(cappAnnotations, revisionCappAnnotations)
}

// equalLabels returns a boolean indicating whether two label maps are equal.
func equalLabels(cappLabels, revisionCappLabels map[string]string) bool {
	return equality.Semantic.DeepEqual(cappLabels, revisionCappLabels)
}

// isEqual returns a boolean indicating whether a Capp instance is equal to a Capp Revision instance.
// The comparison concerts the Capp Spec of the two instances, the Capp annotations and Capp labels.
func isEqual(capp cappv1alpha1.Capp, revision cappv1alpha1.CappRevision) bool {
	return equalSpec(capp.Spec, revision.Spec.CappTemplate.Spec) &&
		equalAnnotations(capp.Annotations, revision.Spec.CappTemplate.Annotations) &&
		equalLabels(capp.Labels, revision.Spec.CappTemplate.Labels)
}

// HandleCappUpdate manages the flow of CappRevision when a Capp is updated. It ensures that a CappRevision is created for every update.
// It also maintains a limit of only revisionsToKeep CappRevisions in the same namespace as the Capp.
func HandleCappUpdate(ctx context.Context, k8sClient client.Client, capp cappv1alpha1.Capp, logger logr.Logger, cappRevisions []cappv1alpha1.CappRevision) error {
	sortByCreationTime(cappRevisions)
	numOfRevisions := len(cappRevisions)

	latestRevision := cappRevisions[0]
	if isEqual(capp, latestRevision) {
		return nil
	}

	if numOfRevisions < revisionsToKeep {
		return adapters.CreateCappRevision(ctx, k8sClient, logger, capp, cappRevisions[0].Spec.RevisionNumber+1)
	}
	relevantRevision, revisionsToDelete := splitRevisionsAtIndex(cappRevisions, revisionsToKeep-1)

	for _, revision := range revisionsToDelete {
		if err := adapters.DeleteCappRevision(ctx, k8sClient, logger, &revision); err != nil {
			return err
		}
	}

	return adapters.CreateCappRevision(ctx, k8sClient, logger, capp, relevantRevision[0].Spec.RevisionNumber+1)
}
