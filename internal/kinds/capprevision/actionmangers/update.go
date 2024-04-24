package actionmangers

import (
	"context"
	"sort"

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

// HandleCappUpdate manages the flow of CappRevision when a Capp is updated. It ensures that a CappRevision is created for every update.
// It also maintains a limit of only revisionsToKeep CappRevisions in the same namespace as the Capp.
func HandleCappUpdate(ctx context.Context, k8sClient client.Client, capp cappv1alpha1.Capp, logger logr.Logger, cappRevisions []cappv1alpha1.CappRevision) error {
	sortByCreationTime(cappRevisions)
	numOfRevisions := len(cappRevisions)
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
