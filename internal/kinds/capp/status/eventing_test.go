package status

import (
	"context"
	"fmt"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	kafkasourcev1 "knative.dev/eventing-kafka-broker/control-plane/pkg/apis/sources/v1"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	kapis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	cappName      = "my-capp"
	cappNamespace = "my-ns"
)

func newEventingScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(cappv1alpha1.AddToScheme(s))
	utilruntime.Must(sourcesv1.AddToScheme(s))
	utilruntime.Must(kafkasourcev1.AddToScheme(s))
	return s
}

func newCapp() cappv1alpha1.Capp {
	return cappv1alpha1.Capp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cappName,
			Namespace: cappNamespace,
		},
	}
}

func newPingSource(name string, ready corev1.ConditionStatus) *sourcesv1.PingSource {
	ps := &sourcesv1.PingSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cappNamespace,
			Labels:    utils.ManagedResourceLabels(cappName),
		},
	}
	ps.Status.Conditions = duckv1.Conditions{{Type: kapis.ConditionReady, Status: ready}}
	return ps
}

func newKafkaSource(name string, ready corev1.ConditionStatus) *kafkasourcev1.KafkaSource {
	ks := &kafkasourcev1.KafkaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cappNamespace,
			Labels:    utils.ManagedResourceLabels(cappName),
		},
	}
	ks.Status.Conditions = duckv1.Conditions{{Type: kafkasourcev1.KafkaConditionReady, Status: ready}}
	return ks
}

func TestBuildEventingStatus(t *testing.T) {
	ctx := context.Background()
	capp := newCapp()

	t.Run("returns empty when no owned sources exist", func(t *testing.T) {
		result, err := buildEventingStatus(ctx, fake.NewClientBuilder().WithScheme(newEventingScheme()).Build(), capp)
		require.NoError(t, err)
		require.Empty(t, result.EventSources)
	})

	t.Run("maps ping sources", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newEventingScheme()).WithObjects(
			newPingSource(fmt.Sprintf("%s-orders", cappName), corev1.ConditionTrue),
		).Build()

		result, err := buildEventingStatus(ctx, fakeClient, capp)
		require.NoError(t, err)
		require.Len(t, result.EventSources, 1)
		require.Equal(t, fmt.Sprintf("%s-orders", cappName), result.EventSources[0].Name)
		require.Equal(t, corev1.ConditionTrue, result.EventSources[0].Condition.Status)
	})

	t.Run("maps kafka sources", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newEventingScheme()).WithObjects(
			newKafkaSource(fmt.Sprintf("%s-orders", cappName), corev1.ConditionFalse),
		).Build()

		result, err := buildEventingStatus(ctx, fakeClient, capp)
		require.NoError(t, err)
		require.Len(t, result.EventSources, 1)
		require.Equal(t, fmt.Sprintf("%s-orders", cappName), result.EventSources[0].Name)
		require.Equal(t, corev1.ConditionFalse, result.EventSources[0].Condition.Status)
	})

	t.Run("merges ping and kafka sources sorted by name", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newEventingScheme()).WithObjects(
			newPingSource(fmt.Sprintf("%s-a", cappName), corev1.ConditionTrue),
			newKafkaSource(fmt.Sprintf("%s-b", cappName), corev1.ConditionTrue),
			newPingSource(fmt.Sprintf("%s-c", cappName), corev1.ConditionUnknown),
		).Build()

		result, err := buildEventingStatus(ctx, fakeClient, capp)
		require.NoError(t, err)
		require.Equal(t, []string{
			fmt.Sprintf("%s-a", cappName),
			fmt.Sprintf("%s-b", cappName),
			fmt.Sprintf("%s-c", cappName),
		}, []string{
			result.EventSources[0].Name,
			result.EventSources[1].Name,
			result.EventSources[2].Name,
		})
	})
}

func TestNewEventSourceStatus(t *testing.T) {
	const readyReason = "Ready"
	ready := &kapis.Condition{
		Type:    kapis.ConditionReady,
		Status:  corev1.ConditionTrue,
		Message: "ready to consume",
		Reason:  readyReason,
	}

	tests := []struct {
		name           string
		ready          *kapis.Condition
		wantStatus     corev1.ConditionStatus
		wantMessage    string
		wantReason     string
		checkCondition bool
	}{
		{
			name:           "uses unknown readiness when condition is nil",
			wantStatus:     corev1.ConditionUnknown,
			wantMessage:    "Source readiness not known",
			checkCondition: true,
		},
		{
			name:           "copies ready condition",
			ready:          ready,
			wantStatus:     corev1.ConditionTrue,
			wantMessage:    "ready to consume",
			wantReason:     readyReason,
			checkCondition: true,
		},
	}

	const sourceName = "src-a"
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := newEventSourceStatus(sourceName, tt.ready)
			require.Equal(t, sourceName, result.Name)
			if !tt.checkCondition {
				return
			}
			require.Equal(t, tt.wantStatus, result.Condition.Status)
			require.Equal(t, tt.wantMessage, result.Condition.Message)
			if tt.wantReason != "" {
				require.Equal(t, tt.wantReason, result.Condition.Reason)
			}
		})
	}
}
