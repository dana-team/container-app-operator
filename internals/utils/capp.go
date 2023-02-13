package utils

import (
	"context"
	"reflect"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var openshiftConsoleKey = types.NamespacedName{Namespace: "openshift-console", Name: "console"}

var clusters = map[string]string{
	"ocp-nikola": "10.0.128.0/23",
}

func buildApplicationLinks(ctx context.Context, capp rcsv1alpha1.Capp, log logr.Logger, r client.Client) (*rcsv1alpha1.ApplicationLinks, error) {
	applicationLinks := rcsv1alpha1.ApplicationLinks{}
	consoleRoute := routev1.Route{}
	if err := r.Get(ctx, openshiftConsoleKey, &consoleRoute); err != nil {
		return nil, err
	}
	applicationLinks.ConsoleLink = consoleRoute.Spec.Host
	applicationLinks.Site = capp.Status.ApplicationLinks.Site
	applicationLinks.ClusterSegment = clusters[applicationLinks.Site]
	return &applicationLinks, nil
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
	capp.Status.ApplicationLinks = *applicationLinks
	if !reflect.DeepEqual(capp.Status, cappObject.Status) {
		if err := r.Status().Update(ctx, &cappObject); err != nil {
			return err
		}
	}
	return nil
}
