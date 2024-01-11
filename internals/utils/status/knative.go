package status_utils

import (
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internals/utils"
	"github.com/go-logr/logr"
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
func buildRevisionsStatus(ctx context.Context, capp rcsv1alpha1.Capp,
	r client.Client) ([]rcsv1alpha1.RevisionInfo, error) {
	knativeRevisions := knativev1.RevisionList{}
	revisionsInfo := []rcsv1alpha1.RevisionInfo{}
	requirement, err := labels.NewRequirement(KnativeLabelKey, selection.Equals, []string{capp.Name})
	if err != nil {
		return revisionsInfo, err
	}
	labelSelector := labels.NewSelector().Add(*requirement)
	listOptions := client.ListOptions{
		LabelSelector: labelSelector,
		Limit:         ClientListLimit,
	}
	if err := r.List(ctx, &knativeRevisions, &listOptions); err != nil {
		return revisionsInfo, err
	}
	for _, revision := range knativeRevisions.Items {
		revisionsInfo = append(revisionsInfo, rcsv1alpha1.RevisionInfo{
			RevisionName:   revision.Name,
			RevisionStatus: revision.Status,
		})
	}
	return revisionsInfo, nil
}

// buildKnativeStatus responsible all the status related to the knative.
// The funciton returns
func buildKnativeStatus(ctx context.Context, kubeClient client.Client, logger logr.Logger,
	capp rcsv1alpha1.Capp) (knativev1.ServiceStatus, []rcsv1alpha1.RevisionInfo, error) {
	KnativeObjectStatus := knativev1.ServiceStatus{}
	RevisionInfo := []rcsv1alpha1.RevisionInfo{}
	if !utils.DoesHaltAnnotationExist(capp.Annotations) {
		kservice := &knativev1.Service{}
		if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, kservice); err != nil {
			return KnativeObjectStatus, RevisionInfo, err
		}
		RevisionsStatus, err := buildRevisionsStatus(ctx, capp, kubeClient)
		if err != nil {
			return KnativeObjectStatus, RevisionInfo, err
		}
		KnativeObjectStatus = kservice.Status
		RevisionInfo = RevisionsStatus
	}
	return KnativeObjectStatus, RevisionInfo, nil
}
