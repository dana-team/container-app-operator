package utils

import (
	"context"

	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetCappRevisions retrieves a list of CappRevision resources filtered by labels matching a specific Capp, returning the list and any error encountered.
func GetCappRevisions(ctx context.Context, k8sClient client.Client, capp cappv1alpha1.Capp) ([]cappv1alpha1.CappRevision, error) {
	cappRevisions := cappv1alpha1.CappRevisionList{}

	requirement, err := labels.NewRequirement(testconsts.CappNameLabelKey, selection.Equals, []string{capp.Name})
	if err != nil {
		return cappRevisions.Items, err
	}

	labelSelector := labels.NewSelector().Add(*requirement)
	listOptions := client.ListOptions{
		Namespace:     capp.Namespace,
		LabelSelector: labelSelector,
		Limit:         testconsts.ClientListLimit,
	}

	err = k8sClient.List(ctx, &cappRevisions, &listOptions)
	return cappRevisions.Items, err
}

// GetCappRevision retrieves a CappRevision by name.
func GetCappRevision(k8sClient client.Client, cappRevisionName, namespace string) *cappv1alpha1.CappRevision {
	cappRevision := &cappv1alpha1.CappRevision{}
	GetResource(k8sClient, cappRevision, cappRevisionName, namespace)
	return cappRevision
}
