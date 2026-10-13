package status

import (
	"context"
	"sort"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// revisionConditionTypes are checked in order to explain why a revision is not ready.
var revisionConditionTypes = []kapis.ConditionType{
	knativev1.RevisionConditionContainerHealthy,
	knativev1.RevisionConditionResourcesAvailable,
	knativev1.RevisionConditionReady,
}

// checkKnative returns why the Knative Service is not ready, or nil when it is.
// If the latest created revision is not ready, it is reported instead of the service.
func checkKnative(ctx context.Context, k8sClient client.Client, capp cappv1alpha1.Capp, _ *cappv1alpha1.CappConfig) (*metav1.Condition, error) {
	ksvc := &knativev1.Service{}
	key := types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}
	if cond, err := getChildResource(ctx, k8sClient, rmanagers.KnativeService, key, ksvc); cond != nil || err != nil {
		return cond, err
	}

	ready := ksvc.Status.GetCondition(kapis.ConditionReady)
	if ready == nil || ksvc.Status.ObservedGeneration < ksvc.Generation {
		return newPendingCondition(rmanagers.KnativeService, ksvc.Name), nil
	}

	if latest := ksvc.Status.LatestCreatedRevisionName; latest != "" && latest != ksvc.Status.LatestReadyRevisionName {
		cond, err := checkRevision(ctx, k8sClient, types.NamespacedName{Namespace: capp.Namespace, Name: latest})
		if cond != nil || err != nil {
			return cond, err
		}
	}

	return formatChildCondition(rmanagers.KnativeService, ksvc.Name, metav1.ConditionStatus(ready.Status), ready.Reason, ready.Message), nil
}

// checkRevision returns the first non-True condition of a revision, or nil when there is none.
func checkRevision(ctx context.Context, k8sClient client.Client, key types.NamespacedName) (*metav1.Condition, error) {
	rev := &knativev1.Revision{}
	if cond, err := getChildResource(ctx, k8sClient, kindRevision, key, rev); cond != nil || err != nil {
		return cond, err
	}
	if rev.Status.ObservedGeneration < rev.Generation {
		return newPendingCondition(kindRevision, rev.Name), nil
	}

	for _, conditionType := range revisionConditionTypes {
		if rc := rev.Status.GetCondition(conditionType); rc != nil && !rc.IsTrue() {
			return formatChildCondition(kindRevision, rev.Name, metav1.ConditionStatus(rc.Status), rc.Reason, rc.Message), nil
		}
	}
	return nil, nil
}
