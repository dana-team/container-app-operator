package status

import (
	"context"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newLoggingScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(cappv1alpha1.AddToScheme(s))
	utilruntime.Must(loggingv1beta1.AddToScheme(s))
	return s
}

func newSyslogNGFlow(problems int) *loggingv1beta1.SyslogNGFlow {
	return &loggingv1beta1.SyslogNGFlow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cappName,
			Namespace: cappNamespace,
		},
		Status: loggingv1beta1.SyslogNGFlowStatus{
			ProblemsCount: problems,
		},
	}
}

func newSyslogNGOutput(problems int) *loggingv1beta1.SyslogNGOutput {
	return &loggingv1beta1.SyslogNGOutput{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cappName,
			Namespace: cappNamespace,
		},
		Status: loggingv1beta1.SyslogNGOutputStatus{
			ProblemsCount: problems,
		},
	}
}

func TestBuildLoggingStatus(t *testing.T) {
	ctx := context.Background()
	capp := newCapp()
	log := logr.Discard()
	existing := cappv1alpha1.LoggingStatus{}

	t.Run("returns empty when not required", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newLoggingScheme()).Build()

		result, err := buildLoggingStatus(ctx, capp, log, fakeClient, existing, false)
		require.NoError(t, err)
		assert.Empty(t, result.Conditions)
	})

	t.Run("returns empty when flow not found", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newLoggingScheme()).Build()

		result, err := buildLoggingStatus(ctx, capp, log, fakeClient, existing, true)
		require.NoError(t, err)
		assert.Empty(t, result.Conditions)
	})

	t.Run("returns empty when output not found", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newLoggingScheme()).
			WithObjects(newSyslogNGFlow(0)).Build()

		result, err := buildLoggingStatus(ctx, capp, log, fakeClient, existing, true)
		require.NoError(t, err)
		assert.Empty(t, result.Conditions)
	})

	t.Run("sets ready condition when no problems", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newLoggingScheme()).
			WithObjects(newSyslogNGFlow(0), newSyslogNGOutput(0)).Build()

		result, err := buildLoggingStatus(ctx, capp, log, fakeClient, existing, true)
		require.NoError(t, err)
		require.Len(t, result.Conditions, 1)
		assert.Equal(t, loggingReady, result.Conditions[0].Type)
		assert.Equal(t, metav1.ConditionTrue, result.Conditions[0].Status)
		assert.Equal(t, conditionReady, result.Conditions[0].Reason)
	})

	t.Run("sets not-ready condition when flow has problems", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newLoggingScheme()).
			WithObjects(newSyslogNGFlow(2), newSyslogNGOutput(0)).Build()

		result, err := buildLoggingStatus(ctx, capp, log, fakeClient, existing, true)
		require.NoError(t, err)
		require.Len(t, result.Conditions, 1)
		assert.Equal(t, loggingReady, result.Conditions[0].Type)
		assert.Equal(t, metav1.ConditionFalse, result.Conditions[0].Status)
		assert.Equal(t, loggingResourceInvalid, result.Conditions[0].Reason)
	})

	t.Run("sets not-ready condition when output has problems", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newLoggingScheme()).
			WithObjects(newSyslogNGFlow(0), newSyslogNGOutput(1)).Build()

		result, err := buildLoggingStatus(ctx, capp, log, fakeClient, existing, true)
		require.NoError(t, err)
		require.Len(t, result.Conditions, 1)
		assert.Equal(t, metav1.ConditionFalse, result.Conditions[0].Status)
		assert.Equal(t, loggingResourceInvalid, result.Conditions[0].Reason)
	})
}

func TestLoggingNotReady(t *testing.T) {
	tests := []struct {
		name       string
		status     cappv1alpha1.LoggingStatus
		wantReason string
		wantMsg    string
		wantOK     bool
	}{
		{
			name:   "ready when logging has no conditions",
			status: cappv1alpha1.LoggingStatus{},
			wantOK: true,
		},
		{
			name: "ready when all conditions are true",
			status: cappv1alpha1.LoggingStatus{
				Conditions: []metav1.Condition{
					{Type: loggingReady, Status: metav1.ConditionTrue, Reason: conditionReady},
				},
			},
			wantOK: true,
		},
		{
			name: "not ready when a condition is false",
			status: cappv1alpha1.LoggingStatus{
				Conditions: []metav1.Condition{
					{Type: loggingReady, Status: metav1.ConditionFalse, Reason: loggingResourceInvalid, Message: "syslogng flow invalid"},
				},
			},
			wantReason: cappv1alpha1.CappReadyReasonLoggingNotReady,
			wantMsg:    "syslogng flow invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, msg, ok := loggingNotReady(tt.status)
			assert.Equal(t, tt.wantOK, ok)
			if !ok {
				assert.Equal(t, tt.wantReason, reason)
				assert.Equal(t, tt.wantMsg, msg)
			}
		})
	}
}
