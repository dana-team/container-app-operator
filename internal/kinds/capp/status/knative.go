package status

import (
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	KnativeLabelKey = "serving.knative.dev/configuration"
	ClientListLimit = 100
)

// This function builds the RevisionInfo status of the Capp CRD by getting the list of revisions associated with the Knative service.
// It returns a slice of RevisionInfo structs.
func buildRevisionsStatus(ctx context.Context, capp cappv1alpha1.Capp, r client.Client) ([]cappv1alpha1.RevisionInfo, error) {
	knativeRevisions := knativev1.RevisionList{}
	//nolint:prealloc
	var revisionsInfo []cappv1alpha1.RevisionInfo

	requirement, err := labels.NewRequirement(KnativeLabelKey, selection.Equals, []string{capp.Name})
	if err != nil {
		return revisionsInfo, err
	}

	labelSelector := labels.NewSelector().Add(*requirement)
	listOptions := client.ListOptions{
		Namespace:     capp.Namespace,
		LabelSelector: labelSelector,
		Limit:         ClientListLimit,
	}

	if err := r.List(ctx, &knativeRevisions, &listOptions); err != nil {
		return revisionsInfo, err
	}
	for _, revision := range knativeRevisions.Items {
		revisionsInfo = append(revisionsInfo, cappv1alpha1.RevisionInfo{
			RevisionName:   revision.Name,
			RevisionStatus: revision.Status,
		})
	}

	return revisionsInfo, nil
}

// buildKnativeStatus responsible all the status related to Knative.
func buildKnativeStatus(ctx context.Context, kubeClient client.Client, capp cappv1alpha1.Capp, isRequired bool) (knativev1.ServiceStatus, []cappv1alpha1.RevisionInfo, error) {
	knativeObjectStatus := knativev1.ServiceStatus{}
	var revisionInfo []cappv1alpha1.RevisionInfo

	if isRequired {
		kservice := &knativev1.Service{}
		if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, kservice); err != nil {
			return knativeObjectStatus, revisionInfo, err
		}

		revisionsStatus, err := buildRevisionsStatus(ctx, capp, kubeClient)
		if err != nil {
			return knativeObjectStatus, revisionInfo, err
		}

		knativeObjectStatus = kservice.Status
		revisionInfo = revisionsStatus
	}

	return knativeObjectStatus, revisionInfo, nil
}
