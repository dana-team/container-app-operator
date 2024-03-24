package status

import (
	"context"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// buildRouteStatus constructs the Route Status of the Capp object in accordance to the
// status of the corresponding DomainMapping object if such exists
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

	routeStatus.DomainMappingObjectStatus = domainMapping.Status

	return routeStatus, nil
}
