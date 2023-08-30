// This is a Go package that contains functions for synchronizing the status of a custom resource definition (CRD) called Capp with the status of the Knative service and revisions associated with it.
// The SyncStatus function is the main function that orchestrates the synchronization process.
package status_utils

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var openshiftConsoleKey = types.NamespacedName{Namespace: "openshift-console", Name: "console"}

const (
	KnativeLabelKey = "serving.knative.dev/configuration"
)

// This function builds the ApplicationLinks status of the Capp  by getting the console route and the cluster segment. It returns a pointer to the ApplicationLinks struct.
func buildApplicationLinks(ctx context.Context, capp rcsv1alpha1.Capp, log logr.Logger, r client.Client, onOpenshift bool) (*rcsv1alpha1.ApplicationLinks, error) {
	segment, err := getClusterSegment(ctx, capp, log, r)
	if err != nil {
		return nil, err
	}
	applicationLinks := rcsv1alpha1.ApplicationLinks{
		ClusterSegment: segment,
	}
	if onOpenshift {
		console, err := getClusterConsole(ctx, capp, log, r)
		if err != nil {
			return nil, err
		}
		applicationLinks.ConsoleLink = console
		applicationLinks.Site = strings.Split(console, ".")[2]

	}
	return &applicationLinks, nil
}

func getClusterConsole(ctx context.Context, capp rcsv1alpha1.Capp, log logr.Logger, r client.Client) (string, error) {
	consoleRoute := routev1.Route{}
	if err := r.Get(ctx, openshiftConsoleKey, &consoleRoute); err != nil {
		log.Error(err, "can't get console route")
		return "", err
	}
	return consoleRoute.Spec.Host, nil
}

// This function gets the cluster segment from the list of host subnets.
func getClusterSegment(ctx context.Context, capp rcsv1alpha1.Capp, log logr.Logger, r client.Client) (string, error) {
	nodes := corev1.NodeList{}
	if err := r.List(ctx, &nodes); err != nil {
		return "", err
	}
	for _, address := range nodes.Items[0].Status.Addresses {
		if address.Type == "InternalIP" {
			ipSegments := strings.Split(address.Address, ".")
			ip := fmt.Sprintf("%s.%s.%s.0/24", ipSegments[0], ipSegments[1], ipSegments[2])
			return ip, nil
		}
	}
	return "", nil
}

// This function builds the RevisionInfo status of the Capp CRD by getting the list of revisions associated with the Knative service. It returns a slice of RevisionInfo structs.
func buildRevisionsStatus(ctx context.Context, capp rcsv1alpha1.Capp, knativeService knativev1.Service, log logr.Logger, r client.Client) ([]rcsv1alpha1.RevisionInfo, error) {
	knativeRevisions := knativev1.RevisionList{}
	revisionsInfo := []rcsv1alpha1.RevisionInfo{}
	requirement, err := labels.NewRequirement(KnativeLabelKey, selection.Equals, []string{capp.Name})
	if err != nil {
		return revisionsInfo, err
	}
	labelSelector := labels.NewSelector().Add(*requirement)
	listOptions := client.ListOptions{
		LabelSelector: labelSelector,
		Limit:         100,
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

// This function builds the Logging status of the Capp CRD by getting the flow and output bundled to the Capp and adding their status. It also creates a condition in accordance with their situation. 
func buildLoggingStatus(ctx context.Context, capp rcsv1alpha1.Capp, knativeService knativev1.Service, log logr.Logger, r client.Client) (rcsv1alpha1.LoggingStatus, error) {
		loggingStatus := rcsv1alpha1.LoggingStatus{}
		flow := &loggingv1beta1.Flow{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name + "-flow"}, flow); err != nil {
			return loggingStatus, err
		}

		output := &loggingv1beta1.Output{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name + "-output"}, output); err != nil {
			return loggingStatus, err
		}
		loggingStatus.Flow = flow.Status
		loggingStatus.Output = output.Status
		problems := "True"
		reason := "Ready"
		if flow.Status.ProblemsCount != 0 || output.Status.ProblemsCount != 0 {
			reason = "LoggingResourceInvalid"
			problems = "False"
		}
		condition := metav1.Condition{
			Type:               "LoggingIsReady",
			Status:             metav1.ConditionStatus(problems),
			LastTransitionTime: metav1.Time{Time: time.Now()},
			Reason:             reason,
		}
		meta.SetStatusCondition(&loggingStatus.Conditions, condition)
		return loggingStatus, nil
	}

// This is the main function that synchronizes the status of the Capp CRD with the Knative service and revisions associated with it.
// It gets the Capp CRD, builds the ApplicationLinks and RevisionInfo statuses, and updates the status of the Capp CRD if it has changed.
func SyncStatus(ctx context.Context, capp rcsv1alpha1.Capp, log logr.Logger, r client.Client, onOpenshift bool) error {
	cappObject := rcsv1alpha1.Capp{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, &cappObject); err != nil {
		return err
	}

	applicationLinks, err := buildApplicationLinks(ctx, capp, log, r, onOpenshift)
	if err != nil {
		return err
	}
	kservice := &knativev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, kservice); err != nil {
		return err
	}
	RevisionsStatus, err := buildRevisionsStatus(ctx, capp, *kservice, log, r)
	if err != nil {
		return err
	}
	cappObject.Status.KnativeObjectStatus = kservice.Status
	cappObject.Status.RevisionInfo = RevisionsStatus
	if cappObject.Spec.LogSpec != (rcsv1alpha1.LogSpec{}) {
		loggingStatus, err := buildLoggingStatus(ctx, capp, *kservice, log, r)
		if err != nil {
			return err
		}
		cappObject.Status.LoggingStatus = loggingStatus
	}
	if !reflect.DeepEqual(applicationLinks, cappObject.Status.ApplicationLinks) {
		cappObject.Status.ApplicationLinks = *applicationLinks
		if err := r.Status().Update(ctx, &cappObject); err != nil {
			log.Error(err, "can't update capp status")
			return err
		}
	}
	return nil
}
