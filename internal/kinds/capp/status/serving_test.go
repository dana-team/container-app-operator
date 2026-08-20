package status

import (
	"context"
	"testing"
	"time"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	kapis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newKnativeScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(cappv1alpha1.AddToScheme(s))
	utilruntime.Must(knativev1.AddToScheme(s))
	return s
}

func newRevision(name string, ts metav1.Time) *knativev1.Revision {
	return &knativev1.Revision{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         cappNamespace,
			CreationTimestamp: ts,
			Labels:            map[string]string{KnativeLabelKey: cappName},
		},
	}
}

func TestBuildKnativeStatus(t *testing.T) {
	ctx := context.Background()
	capp := newCapp()

	t.Run("returns empty when not required", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newKnativeScheme()).Build()

		svcStatus, revisions, err := buildKnativeStatus(ctx, fakeClient, capp, false)
		require.NoError(t, err)
		assert.Empty(t, svcStatus.Conditions)
		assert.Empty(t, revisions)
	})

	t.Run("returns empty when service not found", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newKnativeScheme()).Build()

		svcStatus, revisions, err := buildKnativeStatus(ctx, fakeClient, capp, true)
		require.NoError(t, err)
		assert.Empty(t, svcStatus.Conditions)
		assert.Empty(t, revisions)
	})

	t.Run("returns service status and revisions when service exists", func(t *testing.T) {
		ksvc := &knativev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cappName,
				Namespace: cappNamespace,
			},
			Status: knativev1.ServiceStatus{
				ConfigurationStatusFields: knativev1.ConfigurationStatusFields{
					LatestReadyRevisionName: "rev-ready",
				},
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(newKnativeScheme()).
			WithObjects(ksvc, newRevision(cappName+"-001", metav1.Now())).Build()

		svcStatus, revisions, err := buildKnativeStatus(ctx, fakeClient, capp, true)
		require.NoError(t, err)
		assert.Equal(t, "rev-ready", svcStatus.LatestReadyRevisionName)
		require.Len(t, revisions, 1)
		assert.Equal(t, cappName+"-001", revisions[0].RevisionName)
	})
}

func TestBuildRevisionsStatus(t *testing.T) {
	ctx := context.Background()
	capp := newCapp()

	t.Run("returns empty when no revisions exist", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newKnativeScheme()).Build()

		revisions, err := buildRevisionsStatus(ctx, capp, fakeClient)
		require.NoError(t, err)
		assert.Empty(t, revisions)
	})

	t.Run("sorts revisions by creation timestamp then name", func(t *testing.T) {
		now := metav1.Now()
		earlier := metav1.NewTime(now.Add(-time.Hour))

		fakeClient := fake.NewClientBuilder().WithScheme(newKnativeScheme()).
			WithObjects(
				newRevision(cappName+"-b", now),
				newRevision(cappName+"-a", earlier),
				newRevision(cappName+"-c", now),
			).Build()

		revisions, err := buildRevisionsStatus(ctx, capp, fakeClient)
		require.NoError(t, err)
		require.Len(t, revisions, 3)
		assert.Equal(t, cappName+"-a", revisions[0].RevisionName)
		assert.Equal(t, cappName+"-b", revisions[1].RevisionName)
		assert.Equal(t, cappName+"-c", revisions[2].RevisionName)
	})

	t.Run("does not include revisions from other capps", func(t *testing.T) {
		otherRev := newRevision("other-capp-001", metav1.Now())
		otherRev.Labels[KnativeLabelKey] = "other-capp"

		fakeClient := fake.NewClientBuilder().WithScheme(newKnativeScheme()).
			WithObjects(otherRev).Build()

		revisions, err := buildRevisionsStatus(ctx, capp, fakeClient)
		require.NoError(t, err)
		assert.Empty(t, revisions)
	})
}

func TestKnativeNotReady(t *testing.T) {
	tests := []struct {
		name       string
		status     knativev1.ServiceStatus
		wantReason string
		wantMsg    string
		wantOK     bool
	}{
		{
			name:       "not ready when no conditions exist",
			status:     knativev1.ServiceStatus{},
			wantReason: cappv1alpha1.CappReadyReasonKnativeNotReady,
			wantMsg:    "Knative Service has no status yet",
		},
		{
			name: "not ready when knative Ready condition is false",
			status: knativev1.ServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{Type: kapis.ConditionReady, Status: corev1.ConditionFalse, Message: "image pull failed"},
					},
				},
				ConfigurationStatusFields: knativev1.ConfigurationStatusFields{
					LatestCreatedRevisionName: readyRevision,
					LatestReadyRevisionName:   readyRevision,
				},
			},
			wantReason: cappv1alpha1.CappReadyReasonKnativeNotReady,
			wantMsg:    "image pull failed",
		},
		{
			name: "not ready when latest revision differs from latest ready",
			status: knativev1.ServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{Type: kapis.ConditionReady, Status: corev1.ConditionTrue},
					},
				},
				ConfigurationStatusFields: knativev1.ConfigurationStatusFields{
					LatestCreatedRevisionName: pendingRevision,
					LatestReadyRevisionName:   readyRevision,
				},
			},
			wantReason: cappv1alpha1.CappReadyReasonKnativeNotReady,
			wantMsg:    "latest revision " + pendingRevision + " is not ready",
		},
		{
			name: "not ready when Ready condition is missing from non-empty conditions",
			status: knativev1.ServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{Type: "ConfigurationsReady", Status: corev1.ConditionTrue},
					},
				},
				ConfigurationStatusFields: knativev1.ConfigurationStatusFields{
					LatestCreatedRevisionName: readyRevision,
					LatestReadyRevisionName:   readyRevision,
				},
			},
			wantReason: cappv1alpha1.CappReadyReasonKnativeNotReady,
			wantMsg:    "Knative Service Ready condition not found",
		},
		{
			name: "ready when Ready condition is true and revisions match",
			status: knativev1.ServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{Type: kapis.ConditionReady, Status: corev1.ConditionTrue},
					},
				},
				ConfigurationStatusFields: knativev1.ConfigurationStatusFields{
					LatestCreatedRevisionName: readyRevision,
					LatestReadyRevisionName:   readyRevision,
				},
			},
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, msg, ok := knativeNotReady(tt.status)
			assert.Equal(t, tt.wantOK, ok)
			if !ok {
				assert.Equal(t, tt.wantReason, reason)
				assert.Equal(t, tt.wantMsg, msg)
			}
		})
	}
}
