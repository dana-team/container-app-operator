package status

import (
	"context"
	"fmt"
	"sort"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
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
	set := labels.Set{cappmeta.CappResourceKey: capp.Name}
	listOptions := client.ListOptions{
		LabelSelector: labels.SelectorFromSet(set),
		Namespace:     capp.Namespace,
	}
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

// checkEventing returns why an event source is not ready, or nil when all are ready.
func checkEventing(ctx context.Context, k8sClient client.Client, capp cappv1alpha1.Capp, _ *cappv1alpha1.CappConfig) (*metav1.Condition, error) {
	for _, source := range capp.Spec.EventSourcesSpec.Sources {
		key := types.NamespacedName{Namespace: capp.Namespace, Name: rmanagers.GenerateEventSourceName(capp.Name, source.Name)}

		var cond *metav1.Condition
		var err error
		switch {
		case source.PingSourceConfiguration != nil:
			cond, err = checkPingSource(ctx, k8sClient, key)
		case source.KafkaSourceConfiguration != nil:
			cond, err = checkKafkaSource(ctx, k8sClient, key)
		}
		if cond != nil || err != nil {
			return cond, err
		}
	}
	return nil, nil
}

// checkPingSource returns why the PingSource is not ready, or nil when it is.
func checkPingSource(ctx context.Context, k8sClient client.Client, key types.NamespacedName) (*metav1.Condition, error) {
	source := &sourcesv1.PingSource{}
	if cond, err := getChildResource(ctx, k8sClient, rmanagers.PingSource, key, source); cond != nil || err != nil {
		return cond, err
	}

	ready := source.Status.GetCondition(sourcesv1.PingSourceConditionReady)
	if ready == nil || source.Status.ObservedGeneration < source.Generation {
		return newPendingCondition(rmanagers.PingSource, source.Name), nil
	}
	return formatChildCondition(rmanagers.PingSource, source.Name, metav1.ConditionStatus(ready.Status), ready.Reason, ready.Message), nil
}

// checkKafkaSource returns why the KafkaSource is not ready, or nil when it is.
func checkKafkaSource(ctx context.Context, k8sClient client.Client, key types.NamespacedName) (*metav1.Condition, error) {
	source := &kafkasourcev1.KafkaSource{}
	if cond, err := getChildResource(ctx, k8sClient, rmanagers.KafkaSource, key, source); cond != nil || err != nil {
		return cond, err
	}

	ready := source.Status.GetCondition(kafkasourcev1.KafkaConditionReady)
	if ready == nil || source.Status.ObservedGeneration < source.Generation {
		return newPendingCondition(rmanagers.KafkaSource, source.Name), nil
	}
	return formatChildCondition(rmanagers.KafkaSource, source.Name, metav1.ConditionStatus(ready.Status), ready.Reason, ready.Message), nil
}
