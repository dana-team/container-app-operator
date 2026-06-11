package status

import (
	"context"
	"fmt"
	"sort"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kafkasourcev1 "knative.dev/eventing-kafka-broker/control-plane/pkg/apis/sources/v1"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	kapis "knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func buildEventingStatus(ctx context.Context, r client.Client, capp cappv1alpha1.Capp) (cappv1alpha1.EventingStatus, error) {
	pingSources := sourcesv1.PingSourceList{}
	if err := listOwnedEventSources(ctx, r, capp, &pingSources); err != nil {
		return cappv1alpha1.EventingStatus{}, fmt.Errorf("list PingSources for Capp %q: %w", capp.Name, err)
	}

	kafkaSources := kafkasourcev1.KafkaSourceList{}
	if err := listOwnedEventSources(ctx, r, capp, &kafkaSources); err != nil {
		return cappv1alpha1.EventingStatus{}, fmt.Errorf("list KafkaSources for Capp %q: %w", capp.Name, err)
	}

	statuses := make([]cappv1alpha1.EventSourceStatus, 0, len(pingSources.Items)+len(kafkaSources.Items))
	for i := range pingSources.Items {
		ps := &pingSources.Items[i]
		statuses = append(statuses, newEventSourceStatus(ps.Name, ps.Status.GetCondition(kapis.ConditionReady)))
	}
	for i := range kafkaSources.Items {
		ks := &kafkaSources.Items[i]
		statuses = append(statuses, newEventSourceStatus(ks.Name, ks.Status.GetCondition(kapis.ConditionReady)))
	}

	if len(statuses) == 0 {
		return cappv1alpha1.EventingStatus{}, nil
	}

	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Name < statuses[j].Name })
	return cappv1alpha1.EventingStatus{EventSources: statuses}, nil
}

func listOwnedEventSources(ctx context.Context, r client.Client, capp cappv1alpha1.Capp, list client.ObjectList) error {
	set := labels.Set{utils.CappResourceKey: capp.Name}
	listOptions := utils.GetListOptions(set)
	listOptions.Namespace = capp.Namespace
	return r.List(ctx, list, &listOptions)
}

func newEventSourceStatus(name string, ready *kapis.Condition) cappv1alpha1.EventSourceStatus {
	condition := kapis.Condition{
		Type:               kapis.ConditionReady,
		Status:             corev1.ConditionUnknown,
		Message:            "Source readiness not known",
		LastTransitionTime: kapis.VolatileTime{Inner: metav1.Now()},
	}
	if ready != nil {
		condition.Status = ready.Status
		condition.Message = ready.Message
		if ready.Reason != "" {
			condition.Reason = ready.Reason
		}
		condition.LastTransitionTime = ready.LastTransitionTime
	}
	return cappv1alpha1.EventSourceStatus{
		Name:      name,
		Condition: condition,
	}
}
