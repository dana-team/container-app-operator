package status

import (
	"context"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// buildEventingStatus builds EventingStatus by listing all PingSources owned by the Capp.
// Returns empty status when no sources are required.
func buildEventingStatus(ctx context.Context, capp cappv1alpha1.Capp, r client.Client, required bool) (cappv1alpha1.EventingStatus, error) {
	eventingStatus := cappv1alpha1.EventingStatus{}
	if !required {
		return eventingStatus, nil
	}

	list := sourcesv1.PingSourceList{}
	set := labels.Set{utils.CappResourceKey: capp.Name}
	listOptions := utils.GetListOptions(set)
	if err := r.List(ctx, &list, &listOptions); err != nil {
		return eventingStatus, err
	}

	for _, ps := range list.Items {
		eventingStatus.EventSources = append(eventingStatus.EventSources, cappv1alpha1.EventSourceStatus{
			Name:  ps.Name,
			Ready: ps.Status.IsReady(),
		})
	}

	return eventingStatus, nil
}
