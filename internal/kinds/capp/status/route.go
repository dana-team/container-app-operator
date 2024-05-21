package status

import (
	"context"

	dnsv1alpha1 "sigs.k8s.io/external-dns/endpoint"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// buildRouteStatus constructs the Route Status of the Capp object in accordance to the
// status of the corresponding DomainMapping and DNSEndpoint objects if such exist.
func buildRouteStatus(ctx context.Context, kubeClient client.Client, capp cappv1alpha1.Capp, isRequired bool) (cappv1alpha1.RouteStatus, error) {
	routeStatus := cappv1alpha1.RouteStatus{}

	if !isRequired {
		return routeStatus, nil
	}

	domainMapping := &knativev1beta1.DomainMapping{}
	domainMappingName := capp.Spec.RouteSpec.Hostname
	if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: domainMappingName}, domainMapping); err != nil {
		return routeStatus, err
	}

	dnsEndpoint := &dnsv1alpha1.DNSEndpoint{}
	if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, dnsEndpoint); err != nil {
		return routeStatus, err
	}

	routeStatus.DomainMappingObjectStatus = domainMapping.Status
	routeStatus.DNSEndpointObjectStatus = dnsEndpoint.Status

	return routeStatus, nil
}
