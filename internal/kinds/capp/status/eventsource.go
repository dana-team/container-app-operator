package status

import (
	"context"
	"slices"
	"strings"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"k8s.io/apimachinery/pkg/labels"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	knativeapis "knative.dev/pkg/apis"
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
	listOptions.Namespace = capp.Namespace
	if err := r.List(ctx, &list, &listOptions); err != nil {
		return eventingStatus, err
	}
	slices.SortFunc(list.Items, func(a, b sourcesv1.PingSource) int {
		return strings.Compare(a.Name, b.Name)
	})
	for _, ps := range list.Items {
		ready := ps.Status.IsReady()
		var message string
		if !ready {
			if cond := ps.Status.GetCondition(knativeapis.ConditionReady); cond != nil {
				message = cond.Message
			}
		}
		eventingStatus.EventSources = append(eventingStatus.EventSources, cappv1alpha1.EventSourceStatus{
			Name:    ps.Name,
			Ready:   ready,
			Message: message,
		})
	}

	return eventingStatus, nil
}
