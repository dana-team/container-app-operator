package status_utils

import (
	"fmt"
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

const (
	OpenShiftConsoleNamespace = "openshift-console"
	ConsoleName               = "console"
)

// This function builds the ApplicationLinks status of the Capp  by getting the console route and the cluster segment.
// It returns a pointer to the ApplicationLinks struct.
func buildApplicationLinks(ctx context.Context, log logr.Logger,
	r client.Client, onOpenshift bool) (*rcsv1alpha1.ApplicationLinks, error) {
	segment, err := getClusterSegment(ctx, r)
	if err != nil {
		return nil, err
	}
	applicationLinks := rcsv1alpha1.ApplicationLinks{
		ClusterSegment: segment,
	}
	if onOpenshift {
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

// This function gets the cluster segment from the list of host subnets.
func getClusterSegment(ctx context.Context, r client.Client) (string, error) {
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
