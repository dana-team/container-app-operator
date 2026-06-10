package resourcemanagers

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	kapis "knative.dev/pkg/apis"
)

const eventSourceStatusName = "src-a"

func TestNewEventSourceStatus(t *testing.T) {
	ready := &kapis.Condition{
		Type:    kapis.ConditionReady,
		Status:  corev1.ConditionTrue,
		Message: "ready to consume",
		Reason:  "Ready",
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
			wantReason:     "Ready",
			checkCondition: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := newEventSourceStatus(eventSourceStatusName, tt.ready)
			require.Equal(t, eventSourceStatusName, result.Name)
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
