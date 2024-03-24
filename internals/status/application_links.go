package status

import (
	"strings"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	OpenShiftConsoleNamespace = "openshift-console"
	ConsoleName               = "console"
)

// This function builds the ApplicationLinks status of the Capp  by getting the console route.
// It returns a pointer to the ApplicationLinks struct.
func buildApplicationLinks(ctx context.Context, log logr.Logger, r client.Client, isRequired bool) (*cappv1alpha1.ApplicationLinks, error) {
	applicationLinks := cappv1alpha1.ApplicationLinks{}
	if isRequired {
		console, err := getClusterConsole(ctx, log, r)
		if err != nil {
			return nil, err
		}
		applicationLinks.ConsoleLink = console
		applicationLinks.Site = strings.Split(console, ".")[2]

	}
	return &applicationLinks, nil
}

// getClusterConsole responsible to return the cluster console url
func getClusterConsole(ctx context.Context, log logr.Logger, r client.Client) (string, error) {
	openshiftConsoleKey := types.NamespacedName{Namespace: OpenShiftConsoleNamespace, Name: ConsoleName}
	consoleRoute := routev1.Route{}
	if err := r.Get(ctx, openshiftConsoleKey, &consoleRoute); err != nil {
		log.Error(err, "can't get console route")
		return "", err
	}
	return consoleRoute.Spec.Host, nil
}
