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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const readyRevision = "rev-1"

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

func newKsvc(latestCreated, latestReady string, conditions ...kapis.Condition) *knativev1.Service {
	ksvc := &knativev1.Service{ObjectMeta: metav1.ObjectMeta{Name: cappName, Namespace: cappNamespace}}
	ksvc.Status.Conditions = conditions
	ksvc.Status.LatestCreatedRevisionName = latestCreated
	ksvc.Status.LatestReadyRevisionName = latestReady
	return ksvc
}

func TestCheckKnative(t *testing.T) {
	const latestRevision = cappName + "-00002"
	const imageError = `Unable to fetch image "bad": failed to resolve image to digest`

	ready := kapis.Condition{Type: kapis.ConditionReady, Status: corev1.ConditionTrue}
	unknown := kapis.Condition{Type: kapis.ConditionReady, Status: corev1.ConditionUnknown}

	staleKsvc := newKsvc(readyRevision, readyRevision, ready)
	staleKsvc.Generation = 2
	staleKsvc.Status.ObservedGeneration = 1

	failedRevision := newRevision(latestRevision, metav1.Now())
	failedRevision.Status.Conditions = duckv1.Conditions{
		{Type: knativev1.RevisionConditionContainerHealthy, Status: corev1.ConditionFalse, Reason: knativev1.ReasonContainerMissing, Message: imageError},
	}

	deployingRevision := newRevision(latestRevision, metav1.Now())
	deployingRevision.Status.Conditions = duckv1.Conditions{
		{Type: knativev1.RevisionConditionContainerHealthy, Status: corev1.ConditionTrue},
		{Type: knativev1.RevisionConditionResourcesAvailable, Status: corev1.ConditionUnknown, Reason: knativev1.ReasonDeploying},
	}

	pendingKsvc := &metav1.Condition{
		Status:  metav1.ConditionUnknown,
		Reason:  cappv1alpha1.CapabilityReasonPending,
		Message: "KnativeService " + cappName + " not observed yet",
	}

	tests := []struct {
		name    string
		objects []client.Object
		want    *metav1.Condition
	}{
		{
			name:    "pending when the service status is stale",
			objects: []client.Object{staleKsvc},
			want:    pendingKsvc,
		},
		{
			name:    "nil when the service is ready",
			objects: []client.Object{newKsvc(readyRevision, readyRevision, ready)},
		},
		{
			name: "reports the service condition when the latest revision is ready",
			objects: []client.Object{newKsvc(readyRevision, readyRevision,
				kapis.Condition{Type: kapis.ConditionReady, Status: corev1.ConditionFalse, Reason: "RevisionMissing", Message: "no ready revision"})},
			want: &metav1.Condition{
				Status:  metav1.ConditionFalse,
				Reason:  "RevisionMissing",
				Message: "KnativeService " + cappName + ": no ready revision",
			},
		},
		{
			name: "reports the latest revision when it failed",
			objects: []client.Object{
				newKsvc(latestRevision, "", kapis.Condition{Type: kapis.ConditionReady, Status: corev1.ConditionFalse, Reason: "RevisionFailed"}),
				failedRevision,
			},
			want: &metav1.Condition{
				Status:  metav1.ConditionFalse,
				Reason:  knativev1.ReasonContainerMissing,
				Message: "Revision " + latestRevision + ": " + imageError,
			},
		},
		{
			name:    "reports the latest revision as unknown while it deploys",
			objects: []client.Object{newKsvc(latestRevision, readyRevision, unknown), deployingRevision},
			want: &metav1.Condition{
				Status:  metav1.ConditionUnknown,
				Reason:  knativev1.ReasonDeploying,
				Message: "Revision " + latestRevision,
			},
		},
		{
			name:    "pending when the latest revision is missing",
			objects: []client.Object{newKsvc(latestRevision, readyRevision, unknown)},
			want: &metav1.Condition{
				Status:  metav1.ConditionUnknown,
				Reason:  cappv1alpha1.CapabilityReasonPending,
				Message: "Revision " + latestRevision + " not observed yet",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(newKnativeScheme()).WithObjects(tt.objects...).Build()

			got, err := checkKnative(context.Background(), fakeClient, newCapp(), nil)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
