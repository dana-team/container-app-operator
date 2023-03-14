package status_utils

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	networkingv1 "github.com/openshift/api/network/v1"
	routev1 "github.com/openshift/api/route/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var openshiftConsoleKey = types.NamespacedName{Namespace: "openshift-console", Name: "console"}

func buildApplicationLinks(ctx context.Context, capp rcsv1alpha1.Capp, log logr.Logger, r client.Client) (*rcsv1alpha1.ApplicationLinks, error) {
	consoleRoute := routev1.Route{}
	if err := r.Get(ctx, openshiftConsoleKey, &consoleRoute); err != nil {
		log.Error(err, "Cant get console route")
		return nil, err
	}
	segment, err := getClusterSegment(ctx, capp, log, r)
	if err != nil {
		return nil, err
	}
	applicationLinks := rcsv1alpha1.ApplicationLinks{
		ConsoleLink:    consoleRoute.Spec.Host,
		Site:           strings.Split(consoleRoute.Spec.Host, ".")[2],
		ClusterSegment: segment,
	}
	return &applicationLinks, nil
}

func getClusterSegment(ctx context.Context, capp rcsv1alpha1.Capp, log logr.Logger, r client.Client) (string, error) {
	hostsubnets := networkingv1.HostSubnetList{}
	if err := r.List(ctx, &hostsubnets); err != nil {
		return "", err
	}
	return hostsubnets.Items[0].Subnet, nil
}

func buildRevisionsStatus(ctx context.Context, capp rcsv1alpha1.Capp, knativeService knativev1.Service, log logr.Logger, r client.Client) ([]rcsv1alpha1.RevisionInfo, error) {
	knativeRevisions := knativev1.RevisionList{}
	revisionsInfo := []rcsv1alpha1.RevisionInfo{}
	requirement, err := labels.NewRequirement("serving.knative.dev/configuration", selection.Equals, []string{capp.Name})
	if err != nil {
		fmt.Print("sahar \n", requirement)
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

func SyncStatus(ctx context.Context, capp rcsv1alpha1.Capp, log logr.Logger, r client.Client) error {
	cappObject := rcsv1alpha1.Capp{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, &cappObject); err != nil {
		return err
	}

	applicationLinks, err := buildApplicationLinks(ctx, capp, log, r)
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
	if !reflect.DeepEqual(applicationLinks, cappObject.Status.ApplicationLinks) {
		cappObject.Status.ApplicationLinks = *applicationLinks
		if err := r.Status().Update(ctx, &cappObject); err != nil {
			log.Error(err, "Cant update capp status")
			return err
		}
	}
	return nil
}
