package status

import (
	"context"
	"sort"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	kapis "knative.dev/pkg/apis"
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

	sort.Slice(knativeRevisions.Items, func(i, j int) bool {
		rev1, rev2 := knativeRevisions.Items[i], knativeRevisions.Items[j]
		t1, t2 := &rev1.CreationTimestamp, &rev2.CreationTimestamp
		if !t1.Equal(t2) {
			return t1.Before(t2)
		}
		return rev1.Name < rev2.Name
	})

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
			if apierrors.IsNotFound(err) {
				return knativeObjectStatus, revisionInfo, nil
			}
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

func knativeNotReady(ks knativev1.ServiceStatus) (string, string, bool) {
	if len(ks.Conditions) == 0 {
		return cappv1alpha1.CappReadyReasonKnativeNotReady, "Knative Service has no status yet", false
	}

	if ks.LatestCreatedRevisionName != "" &&
		ks.LatestCreatedRevisionName != ks.LatestReadyRevisionName {
		return cappv1alpha1.CappReadyReasonKnativeNotReady,
			"latest revision " + ks.LatestCreatedRevisionName + " is not ready", false
	}

	for _, c := range ks.Conditions {
		if c.Type == kapis.ConditionReady {
			if c.Status == corev1.ConditionTrue {
				return "", "", true
			}
			return cappv1alpha1.CappReadyReasonKnativeNotReady, c.Message, false
		}
	}
	return cappv1alpha1.CappReadyReasonKnativeNotReady, "Knative Service Ready condition not found", false
}
