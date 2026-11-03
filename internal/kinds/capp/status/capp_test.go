package status

import (
	"context"
	"errors"
	"testing"
	"time"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	dnsrecordv1alpha1 "github.com/dana-team/provider-dns-v2/apis/namespaced/record/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	kafkasourcev1 "knative.dev/eventing-kafka-broker/control-plane/pkg/apis/sources/v1"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	kapis "knative.dev/pkg/apis"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	reasonResolveFailed   = "ResolveFailed"
	reasonPvcPending      = "PvcPending"
	pendingVolumeMessage  = "NfsPvc vol-a not observed yet"
	pendingKsvcMessage    = "KnativeService my-capp not observed yet"
	pendingRolloutMessage = "rollout status for latest revision rev-1 is not recorded yet"
)

type stubManager struct {
	required bool
}

func (s stubManager) Manage(_ context.Context, _ cappv1alpha1.Capp) error  { return nil }
func (s stubManager) CleanUp(_ context.Context, _ cappv1alpha1.Capp) error { return nil }
func (s stubManager) IsRequired(_ cappv1alpha1.Capp) bool                  { return s.required }

// knativeStubManager is a stubManager for the Knative Service that checks its template with the real manager.
type knativeStubManager struct {
	stubManager
	manager rmanagers.KnativeServiceManager
}

func (s knativeStubManager) TemplateApplied(ctx context.Context, capp cappv1alpha1.Capp, ksvc *knativev1.Service) bool {
	return s.manager.TemplateApplied(ctx, capp, ksvc)
}

func buildManagers(required map[string]bool) map[string]rmanagers.ResourceManager {
	all := []string{
		rmanagers.KnativeService,
		rmanagers.SyslogNGFlow,
		rmanagers.SyslogNGOutput,
		rmanagers.DomainMapping,
		rmanagers.DNSRecord,
		rmanagers.Certificate,
		rmanagers.NfsPvc,
		rmanagers.PingSource,
		rmanagers.KafkaSource,
	}
	m := make(map[string]rmanagers.ResourceManager, len(all))
	for _, name := range all {
		m[name] = stubManager{required: required[name]}
	}
	m[rmanagers.KnativeService] = knativeStubManager{
		stubManager: stubManager{required: required[rmanagers.KnativeService]},
		manager:     rmanagers.KnativeServiceManager{CappConfig: newCappConfig()},
	}
	return m
}

func newStatusScheme() *runtime.Scheme {
	s := newRouteScheme()
	utilruntime.Must(knativev1.AddToScheme(s))
	utilruntime.Must(sourcesv1.AddToScheme(s))
	utilruntime.Must(kafkasourcev1.AddToScheme(s))
	return s
}

func TestComputeReadyCondition(t *testing.T) {
	tests := []struct {
		name       string
		state      string
		conditions []metav1.Condition
		want       metav1.Condition
	}{
		{
			name: "ready when all capabilities are ready",
			conditions: []metav1.Condition{
				{Type: cappv1alpha1.CappConditionServingReady, Status: metav1.ConditionTrue, Reason: cappv1alpha1.CapabilityReasonReady},
				{Type: cappv1alpha1.CappConditionRouteReady, Status: metav1.ConditionTrue, Reason: cappv1alpha1.CapabilityReasonReady},
			},
			want: metav1.Condition{
				Type:    cappv1alpha1.CappConditionReady,
				Status:  metav1.ConditionTrue,
				Reason:  cappv1alpha1.CappReadyReasonReady,
				Message: readyMessage,
			},
		},
		{
			name: "reports serving instead of the route symptom",
			conditions: []metav1.Condition{
				{Type: cappv1alpha1.CappConditionServingReady, Status: metav1.ConditionFalse, Reason: knativev1.ReasonContainerMissing, Message: "Revision my-capp-00001: Unable to fetch image"},
				{Type: cappv1alpha1.CappConditionRouteReady, Status: metav1.ConditionFalse, Reason: reasonResolveFailed, Message: "DomainMapping app.example.com: no address"},
			},
			want: metav1.Condition{
				Type:    cappv1alpha1.CappConditionReady,
				Status:  metav1.ConditionFalse,
				Reason:  cappv1alpha1.CappReadyReasonServingNotReady,
				Message: knativev1.ReasonContainerMissing + ": Revision my-capp-00001: Unable to fetch image",
			},
		},
		{
			name: "reports a False root cause before an Unknown one",
			conditions: []metav1.Condition{
				{Type: cappv1alpha1.CappConditionVolumesReady, Status: metav1.ConditionUnknown, Reason: cappv1alpha1.CapabilityReasonPending, Message: pendingVolumeMessage},
				{Type: cappv1alpha1.CappConditionLoggingReady, Status: metav1.ConditionFalse, Reason: loggingResourceInvalid, Message: "SyslogNGFlow my-capp: invalid host"},
			},
			want: metav1.Condition{
				Type:    cappv1alpha1.CappConditionReady,
				Status:  metav1.ConditionFalse,
				Reason:  cappv1alpha1.CappReadyReasonLoggingNotReady,
				Message: loggingResourceInvalid + ": SyslogNGFlow my-capp: invalid host",
			},
		},
		{
			name: "reports an Unknown root cause instead of its symptom",
			conditions: []metav1.Condition{
				{Type: cappv1alpha1.CappConditionVolumesReady, Status: metav1.ConditionUnknown, Reason: cappv1alpha1.CapabilityReasonPending, Message: pendingVolumeMessage},
				{Type: cappv1alpha1.CappConditionServingReady, Status: metav1.ConditionFalse, Reason: "RevisionFailed", Message: "KnativeService my-capp: volume not found"},
			},
			want: metav1.Condition{
				Type:    cappv1alpha1.CappConditionReady,
				Status:  metav1.ConditionUnknown,
				Reason:  cappv1alpha1.CappReadyReasonVolumesNotReady,
				Message: cappv1alpha1.CapabilityReasonPending + ": " + pendingVolumeMessage,
			},
		},
		{
			name: "reports a sync error before other root causes",
			conditions: []metav1.Condition{
				{Type: cappv1alpha1.CappConditionVolumesReady, Status: metav1.ConditionFalse, Reason: reasonPvcPending, Message: "NfsPvc vol-a: waiting for the PVC to bind"},
				{Type: cappv1alpha1.CappConditionRouteReady, Status: metav1.ConditionFalse, Reason: cappv1alpha1.CapabilityReasonResourceSyncFailed, Message: "DomainMapping: admission webhook denied the request"},
			},
			want: metav1.Condition{
				Type:    cappv1alpha1.CappConditionReady,
				Status:  metav1.ConditionFalse,
				Reason:  cappv1alpha1.CappReadyReasonRouteNotReady,
				Message: cappv1alpha1.CapabilityReasonResourceSyncFailed + ": DomainMapping: admission webhook denied the request",
			},
		},
		{
			name:  "ready and disabled when the Capp is disabled",
			state: cappv1alpha1.CappStateDisabled,
			conditions: []metav1.Condition{
				{Type: cappv1alpha1.CappConditionVolumesReady, Status: metav1.ConditionUnknown, Reason: cappv1alpha1.CapabilityReasonPending, Message: pendingVolumeMessage},
			},
			want: metav1.Condition{
				Type:    cappv1alpha1.CappConditionReady,
				Status:  metav1.ConditionTrue,
				Reason:  cappv1alpha1.CappReadyReasonDisabled,
				Message: disabledMessage,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capp := newCapp()
			capp.Spec.State = tt.state

			assert.Equal(t, tt.want, computeReadyCondition(capp, tt.conditions))
		})
	}
}

func TestSetStatusConditions(t *testing.T) {
	const generation = 3

	ready := kapis.Condition{Type: kapis.ConditionReady, Status: corev1.ConditionTrue}
	objects := []client.Object{
		newKsvc(readyRevision, readyRevision, ready),
		newCNAMERecord(
			xpv2.Condition{Type: xpv2.TypeSynced, Status: corev1.ConditionTrue},
			xpv2.Condition{Type: xpv2.TypeReady, Status: corev1.ConditionTrue},
		),
		newDomainMapping(ready),
	}
	routeManagers := map[string]bool{rmanagers.KnativeService: true, rmanagers.DNSRecord: true, rmanagers.DomainMapping: true}
	applyErr := errors.New("DomainMapping: admission webhook denied the request")
	cleanupErr := errors.New("NfsPvc: failed to delete")

	tests := []struct {
		name       string
		state      string
		required   map[string]bool
		syncErrors map[string]error
		want       []metav1.Condition
	}{
		{
			name:     "sets the configured capabilities and removes the others",
			required: routeManagers,
			want: []metav1.Condition{
				{Type: cappv1alpha1.CappConditionServingReady, Status: metav1.ConditionTrue, Reason: cappv1alpha1.CapabilityReasonReady, ObservedGeneration: generation},
				{Type: cappv1alpha1.CappConditionRouteReady, Status: metav1.ConditionTrue, Reason: cappv1alpha1.CapabilityReasonReady, ObservedGeneration: generation},
				{Type: cappv1alpha1.CappConditionAvailable, Status: metav1.ConditionUnknown, Reason: cappv1alpha1.CapabilityReasonPending, Message: pendingKsvcMessage, ObservedGeneration: generation},
				{Type: cappv1alpha1.CappConditionRolloutSucceeded, Status: metav1.ConditionUnknown, Reason: cappv1alpha1.CapabilityReasonPending, Message: pendingRolloutMessage, ObservedGeneration: generation},
				{Type: cappv1alpha1.CappConditionReady, Status: metav1.ConditionTrue, Reason: cappv1alpha1.CappReadyReasonReady, Message: readyMessage, ObservedGeneration: generation},
			},
		},
		{
			name:       "reports sync errors on their capabilities, even when not configured",
			required:   routeManagers,
			syncErrors: map[string]error{rmanagers.DomainMapping: applyErr, rmanagers.NfsPvc: cleanupErr},
			want: []metav1.Condition{
				{Type: cappv1alpha1.CappConditionVolumesReady, Status: metav1.ConditionFalse, Reason: cappv1alpha1.CapabilityReasonResourceSyncFailed, Message: cleanupErr.Error(), ObservedGeneration: generation},
				{Type: cappv1alpha1.CappConditionServingReady, Status: metav1.ConditionTrue, Reason: cappv1alpha1.CapabilityReasonReady, ObservedGeneration: generation},
				{Type: cappv1alpha1.CappConditionRouteReady, Status: metav1.ConditionFalse, Reason: cappv1alpha1.CapabilityReasonResourceSyncFailed, Message: applyErr.Error(), ObservedGeneration: generation},
				{Type: cappv1alpha1.CappConditionAvailable, Status: metav1.ConditionUnknown, Reason: cappv1alpha1.CapabilityReasonPending, Message: pendingKsvcMessage, ObservedGeneration: generation},
				{Type: cappv1alpha1.CappConditionRolloutSucceeded, Status: metav1.ConditionUnknown, Reason: cappv1alpha1.CapabilityReasonPending, Message: pendingRolloutMessage, ObservedGeneration: generation},
				{Type: cappv1alpha1.CappConditionReady, Status: metav1.ConditionFalse, Reason: cappv1alpha1.CappReadyReasonVolumesNotReady, Message: cappv1alpha1.CapabilityReasonResourceSyncFailed + ": " + cleanupErr.Error(), ObservedGeneration: generation},
			},
		},
		{
			name:     "removes the capabilities that depend on serving when disabled",
			state:    cappv1alpha1.CappStateDisabled,
			required: map[string]bool{rmanagers.DNSRecord: true, rmanagers.DomainMapping: true},
			want: []metav1.Condition{
				{Type: cappv1alpha1.CappConditionAvailable, Status: metav1.ConditionUnknown, Reason: cappv1alpha1.CapabilityReasonPending, Message: pendingKsvcMessage, ObservedGeneration: generation},
				{Type: cappv1alpha1.CappConditionRolloutSucceeded, Status: metav1.ConditionUnknown, Reason: cappv1alpha1.CapabilityReasonPending, Message: pendingRolloutMessage, ObservedGeneration: generation},
				{Type: cappv1alpha1.CappConditionReady, Status: metav1.ConditionTrue, Reason: cappv1alpha1.CappReadyReasonDisabled, Message: disabledMessage, ObservedGeneration: generation},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capp := routeCapp()
			capp.Generation = generation
			capp.Spec.State = tt.state
			status := cappv1alpha1.CappStatus{
				Conditions: []metav1.Condition{
					{Type: cappv1alpha1.CappConditionVolumesReady, Status: metav1.ConditionTrue, Reason: cappv1alpha1.CapabilityReasonReady},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(newStatusScheme()).WithObjects(objects...).Build()

			err := setStatusConditions(context.Background(), fakeClient, &status, capp, buildManagers(tt.required), newCappConfig(), tt.syncErrors)
			require.NoError(t, err)

			for i := range status.Conditions {
				status.Conditions[i].LastTransitionTime = metav1.Time{}
			}
			assert.Equal(t, tt.want, status.Conditions)
		})
	}
}

func TestSyncStatus(t *testing.T) {
	ctx := context.Background()
	capp := newCapp()
	capp.Generation = 2
	ksvc := newKsvc(readyRevision, readyRevision, kapis.Condition{Type: kapis.ConditionReady, Status: corev1.ConditionTrue})
	fakeClient := fake.NewClientBuilder().WithScheme(newStatusScheme()).
		WithObjects(&capp, ksvc).WithStatusSubresource(&cappv1alpha1.Capp{}).Build()

	err := SyncStatus(ctx, capp, logr.Discard(), fakeClient, buildManagers(map[string]bool{rmanagers.KnativeService: true}), newCappConfig(), nil)
	require.NoError(t, err)

	updated := cappv1alpha1.Capp{}
	require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Namespace: cappNamespace, Name: cappName}, &updated))
	assert.Equal(t, int64(2), updated.Status.ObservedGeneration)
	readyCondition := meta.FindStatusCondition(updated.Status.Conditions, cappv1alpha1.CappConditionReady)
	require.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionTrue, readyCondition.Status)
}

func TestCreateStateStatus(t *testing.T) {
	t.Run("sets state and timestamp when state is empty", func(t *testing.T) {
		status := &cappv1alpha1.StateStatus{}

		CreateStateStatus(status, cappv1alpha1.CappStateEnabled)

		assert.Equal(t, cappv1alpha1.CappStateEnabled, status.State)
		assert.False(t, status.LastChange.IsZero())
	})

	t.Run("updates state and timestamp on state change", func(t *testing.T) {
		original := metav1.Now()
		status := &cappv1alpha1.StateStatus{
			State:      cappv1alpha1.CappStateEnabled,
			LastChange: original,
		}

		CreateStateStatus(status, cappv1alpha1.CappStateDisabled)

		assert.Equal(t, cappv1alpha1.CappStateDisabled, status.State)
		assert.False(t, status.LastChange.Before(&original))
	})

	t.Run("does not change timestamp when state is unchanged", func(t *testing.T) {
		original := metav1.Now()
		status := &cappv1alpha1.StateStatus{
			State:      cappv1alpha1.CappStateEnabled,
			LastChange: original,
		}

		CreateStateStatus(status, cappv1alpha1.CappStateEnabled)

		assert.Equal(t, cappv1alpha1.CappStateEnabled, status.State)
		assert.Equal(t, original, status.LastChange)
	})
}

//nolint:staticcheck // reads deprecated fields
func TestStripVolatileStatusFields(t *testing.T) {
	ts := metav1.NewTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	ptrTs := &ts

	status := cappv1alpha1.CappStatus{
		Conditions: []metav1.Condition{
			{Type: cappv1alpha1.CappConditionReady, Status: metav1.ConditionTrue, LastTransitionTime: ts},
		},
		LoggingStatus: cappv1alpha1.LoggingStatus{
			Conditions: []metav1.Condition{
				{Type: loggingReady, Status: metav1.ConditionTrue, LastTransitionTime: ts},
			},
		},
		RouteStatus: cappv1alpha1.RouteStatus{
			CertificateObjectStatus: cmapi.CertificateStatus{
				Conditions: []cmapi.CertificateCondition{
					{Type: cmapi.CertificateConditionReady, Status: cmmeta.ConditionTrue, LastTransitionTime: ptrTs},
				},
			},
			DNSRecordObjectStatus: cappv1alpha1.DNSRecordObjectStatus{
				CNAMERecordObjectStatus: dnsrecordv1alpha1.CNAMERecordStatus{
					ManagedResourceStatus: xpv2.ManagedResourceStatus{
						ConditionedStatus: xpv2.ConditionedStatus{
							Conditions: []xpv2.Condition{
								{Type: xpv2.TypeReady, Status: corev1.ConditionTrue, LastTransitionTime: ts},
							},
						},
					},
				},
			},
		},
		EventingStatus: cappv1alpha1.EventingStatus{
			EventSources: []cappv1alpha1.EventSourceStatus{
				{
					Name: "src",
					Condition: kapis.Condition{
						Type:               kapis.ConditionReady,
						Status:             corev1.ConditionTrue,
						LastTransitionTime: kapis.VolatileTime{Inner: ts},
					},
				},
			},
		},
	}

	result := stripVolatileStatusFields(status)

	require.Len(t, result.Conditions, 1)
	assert.True(t, result.Conditions[0].LastTransitionTime.IsZero())

	require.Len(t, result.LoggingStatus.Conditions, 1)
	assert.True(t, result.LoggingStatus.Conditions[0].LastTransitionTime.IsZero())

	require.Len(t, result.RouteStatus.CertificateObjectStatus.Conditions, 1)
	assert.Nil(t, result.RouteStatus.CertificateObjectStatus.Conditions[0].LastTransitionTime)

	require.Len(t, result.RouteStatus.DNSRecordObjectStatus.CNAMERecordObjectStatus.Conditions, 1)
	assert.True(t, result.RouteStatus.DNSRecordObjectStatus.CNAMERecordObjectStatus.Conditions[0].LastTransitionTime.IsZero())

	require.Len(t, result.EventingStatus.EventSources, 1)
	assert.True(t, result.EventingStatus.EventSources[0].Condition.LastTransitionTime.Inner.IsZero())
}

func TestSetStatusConditionsRollout(t *testing.T) {
	managers := buildManagers(map[string]bool{rmanagers.KnativeService: true})
	const message = "candidate revision rev-2 rolled back"
	// routeRolloutCapp is newRolloutCapp with the hostname of routeCapp, so that the Knative Service of the fixtures matches the Capp.
	routeRolloutCapp := func() cappv1alpha1.Capp {
		capp := newRolloutCapp()
		capp.Spec.RouteSpec.Hostname = hostname
		return capp
	}

	newFakeClient := func(objects ...client.Object) client.Client {
		return fake.NewClientBuilder().WithScheme(newStatusScheme()).WithObjects(objects...).Build()
	}
	servingKsvc := newRolloutKsvc(stableRevision, corev1.ConditionTrue, trafficTo(stableRevision, 100))
	servingObjects := []client.Object{servingKsvc, newRevisionWithReady(stableRevision, corev1.ConditionTrue), newRevisionWithReady(candidateRevision, corev1.ConditionFalse)}

	t.Run("Ready is unchanged by a rolled back rollout, which reports RolloutSucceeded=False and Available=True", func(t *testing.T) {
		capp := routeRolloutCapp()
		capp.Spec.RouteSpec.RolloutStrategy = &cappv1alpha1.RolloutStrategy{}
		capp.Status.Rollout = &cappv1alpha1.RolloutStatus{StableRevision: stableRevision, Phase: cappv1alpha1.RolloutPhaseRolledBack, Message: message}
		status := cappv1alpha1.CappStatus{}

		require.NoError(t, setStatusConditions(context.Background(), newFakeClient(servingObjects...), &status, capp, managers, newCappConfig(), nil))

		readyCond := meta.FindStatusCondition(status.Conditions, cappv1alpha1.CappConditionReady)
		require.NotNil(t, readyCond)
		assert.Equal(t, metav1.ConditionTrue, readyCond.Status)
		assert.Equal(t, cappv1alpha1.CappReadyReasonReady, readyCond.Reason)

		succeeded := meta.FindStatusCondition(status.Conditions, cappv1alpha1.CappConditionRolloutSucceeded)
		require.NotNil(t, succeeded)
		assert.Equal(t, metav1.ConditionFalse, succeeded.Status)
		assert.Equal(t, string(cappv1alpha1.RolloutPhaseRolledBack), succeeded.Reason)
		assert.Equal(t, message, succeeded.Message)

		available := meta.FindStatusCondition(status.Conditions, cappv1alpha1.CappConditionAvailable)
		require.NotNil(t, available)
		assert.Equal(t, metav1.ConditionTrue, available.Status)
	})

	t.Run("a failed rollout sets RolloutSucceeded=False but leaves Ready True", func(t *testing.T) {
		capp := routeRolloutCapp()
		capp.Status.Rollout = &cappv1alpha1.RolloutStatus{StableRevision: stableRevision, Phase: cappv1alpha1.RolloutPhaseFailed, Message: message}
		status := cappv1alpha1.CappStatus{}

		require.NoError(t, setStatusConditions(context.Background(), newFakeClient(servingObjects...), &status, capp, managers, newCappConfig(), nil))

		succeeded := meta.FindStatusCondition(status.Conditions, cappv1alpha1.CappConditionRolloutSucceeded)
		require.NotNil(t, succeeded)
		assert.Equal(t, metav1.ConditionFalse, succeeded.Status)
		readyCond := meta.FindStatusCondition(status.Conditions, cappv1alpha1.CappConditionReady)
		require.NotNil(t, readyCond)
		assert.Equal(t, metav1.ConditionTrue, readyCond.Status)
	})

	t.Run("RolloutSucceeded is Unknown while promoting", func(t *testing.T) {
		capp := routeRolloutCapp()
		capp.Status.Rollout = &cappv1alpha1.RolloutStatus{StableRevision: stableRevision, CandidateRevision: candidateRevision, Phase: cappv1alpha1.RolloutPhasePromoting}
		status := cappv1alpha1.CappStatus{}

		require.NoError(t, setStatusConditions(context.Background(), newFakeClient(servingObjects...), &status, capp, managers, newCappConfig(), nil))

		succeeded := meta.FindStatusCondition(status.Conditions, cappv1alpha1.CappConditionRolloutSucceeded)
		require.NotNil(t, succeeded)
		assert.Equal(t, metav1.ConditionUnknown, succeeded.Status)
		assert.Equal(t, string(cappv1alpha1.RolloutPhasePromoting), succeeded.Reason)
	})

	t.Run("RolloutSucceeded is True once the stable revision is confirmed", func(t *testing.T) {
		capp := routeRolloutCapp()
		capp.Status.Rollout = &cappv1alpha1.RolloutStatus{StableRevision: stableRevision, Phase: cappv1alpha1.RolloutPhaseStable}
		status := cappv1alpha1.CappStatus{}

		require.NoError(t, setStatusConditions(context.Background(), newFakeClient(servingObjects...), &status, capp, managers, newCappConfig(), nil))

		succeeded := meta.FindStatusCondition(status.Conditions, cappv1alpha1.CappConditionRolloutSucceeded)
		require.NotNil(t, succeeded)
		assert.Equal(t, metav1.ConditionTrue, succeeded.Status)
	})

	t.Run("RolloutSucceeded is unknown before the rollout status is recorded, and Available is always set", func(t *testing.T) {
		capp := routeRolloutCapp()
		status := cappv1alpha1.CappStatus{}

		require.NoError(t, setStatusConditions(context.Background(), newFakeClient(servingObjects...), &status, capp, managers, newCappConfig(), nil))

		succeeded := meta.FindStatusCondition(status.Conditions, cappv1alpha1.CappConditionRolloutSucceeded)
		require.NotNil(t, succeeded)
		assert.Equal(t, metav1.ConditionUnknown, succeeded.Status)
		available := meta.FindStatusCondition(status.Conditions, cappv1alpha1.CappConditionAvailable)
		require.NotNil(t, available)
		assert.Equal(t, metav1.ConditionTrue, available.Status)
	})
}

func TestSetStatusConditionsRolloutSucceededSyncErrors(t *testing.T) {
	const generation = 2
	applyErr := errors.New("Knative Service: admission webhook denied the request")
	otherErr := errors.New("DomainMapping: admission webhook denied the request")

	tests := []struct {
		name       string
		syncErrors map[string]error
		want       metav1.ConditionStatus
		wantReason string
	}{
		{
			name:       "knative service sync error fails the rollout",
			syncErrors: map[string]error{rmanagers.KnativeService: applyErr},
			want:       metav1.ConditionFalse,
			wantReason: cappv1alpha1.CapabilityReasonResourceSyncFailed,
		},
		{
			name:       "other manager sync errors leave the rollout unchanged",
			syncErrors: map[string]error{rmanagers.DomainMapping: otherErr},
			want:       metav1.ConditionTrue,
			wantReason: string(cappv1alpha1.RolloutPhaseStable),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capp := newRolloutCapp()
			capp.Generation = generation
			capp.Status.Rollout = &cappv1alpha1.RolloutStatus{StableRevision: stableRevision, Phase: cappv1alpha1.RolloutPhaseStable}
			k8sClient := newRolloutClient(
				newRolloutKsvc(stableRevision, corev1.ConditionTrue, trafficTo(stableRevision, 100)),
				newRevisionWithReady(stableRevision, corev1.ConditionTrue),
			)

			var status cappv1alpha1.CappStatus
			err := setStatusConditions(context.Background(), k8sClient, &status, capp,
				buildManagers(map[string]bool{rmanagers.KnativeService: true}), newCappConfig(), tt.syncErrors)
			require.NoError(t, err)

			succeeded := meta.FindStatusCondition(status.Conditions, cappv1alpha1.CappConditionRolloutSucceeded)
			require.NotNil(t, succeeded)
			assert.Equal(t, tt.want, succeeded.Status)
			assert.Equal(t, tt.wantReason, succeeded.Reason)
			assert.Equal(t, int64(generation), succeeded.ObservedGeneration)
		})
	}
}
