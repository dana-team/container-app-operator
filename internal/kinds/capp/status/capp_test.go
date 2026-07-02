package status

import (
	"context"
	"testing"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	nfspvcv1alpha1 "github.com/dana-team/nfspvc-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kapis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
)

const (
	readyRevision    = "rev-1"
	pendingRevision  = "rev-2"
	pingEventSource  = "ping-src"
	kafkaEventSource = "kafka-src"
)

type stubManager struct {
	required bool
}

func (s stubManager) Manage(_ context.Context, _ cappv1alpha1.Capp) error  { return nil }
func (s stubManager) CleanUp(_ context.Context, _ cappv1alpha1.Capp) error { return nil }
func (s stubManager) IsRequired(_ cappv1alpha1.Capp) bool                  { return s.required }

func buildManagers(enabled map[string]bool) map[string]rmanagers.ResourceManager {
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
		m[name] = stubManager{required: enabled[name]}
	}
	return m
}

func readyCondition(status *cappv1alpha1.CappStatus) *metav1.Condition {
	for i := range status.Conditions {
		if status.Conditions[i].Type == cappv1alpha1.CappConditionReady {
			return &status.Conditions[i]
		}
	}
	return nil
}

func knativeServiceReady(ready corev1.ConditionStatus) knativev1.ServiceStatus {
	return knativev1.ServiceStatus{
		Status: duckv1.Status{
			Conditions: duckv1.Conditions{
				{Type: kapis.ConditionReady, Status: ready},
			},
		},
		ConfigurationStatusFields: knativev1.ConfigurationStatusFields{
			LatestCreatedRevisionName: readyRevision,
			LatestReadyRevisionName:   readyRevision,
		},
	}
}

func domainMappingStatus(ready corev1.ConditionStatus) knativev1beta1.DomainMappingStatus {
	return knativev1beta1.DomainMappingStatus{
		Status: duckv1.Status{
			Conditions: duckv1.Conditions{
				{Type: kapis.ConditionReady, Status: ready},
			},
		},
	}
}

func certificateStatus(ready cmmeta.ConditionStatus) cmapi.CertificateStatus {
	return cmapi.CertificateStatus{
		Conditions: []cmapi.CertificateCondition{
			{Type: cmapi.CertificateConditionReady, Status: ready},
		},
	}
}

func nfsVolumesBound(names ...string) cappv1alpha1.VolumesStatus {
	vs := cappv1alpha1.VolumesStatus{}
	for _, n := range names {
		vs.NFSVolumesStatus = append(vs.NFSVolumesStatus, cappv1alpha1.NFSVolumeStatus{
			VolumeName: n,
			NFSPVCStatus: nfspvcv1alpha1.NfsPvcStatus{
				PvPhase:  string(corev1.VolumeBound),
				PvcPhase: string(corev1.ClaimBound),
			},
		})
	}
	return vs
}

func nfsVolumesUnbound(name string) cappv1alpha1.VolumesStatus {
	return cappv1alpha1.VolumesStatus{
		NFSVolumesStatus: []cappv1alpha1.NFSVolumeStatus{
			{
				VolumeName: name,
				NFSPVCStatus: nfspvcv1alpha1.NfsPvcStatus{
					PvPhase:  string(corev1.VolumePending),
					PvcPhase: string(corev1.ClaimPending),
				},
			},
		},
	}
}

func TestBuildCappConditions(t *testing.T) {
	capp := cappv1alpha1.Capp{}

	tests := []struct {
		name           string
		status         cappv1alpha1.CappStatus
		enabled        map[string]bool
		expectedStatus metav1.ConditionStatus
		expectedReason string
	}{
		{
			name: "ready when knative is ready and no optional features enabled",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativeServiceReady(corev1.ConditionTrue),
			},
			enabled:        map[string]bool{},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: cappv1alpha1.CappReadyReasonReady,
		},
		{
			name: "not ready when knative has no conditions",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativev1.ServiceStatus{},
			},
			enabled:        map[string]bool{},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: cappv1alpha1.CappReadyReasonKnativeNotReady,
		},
		{
			name: "not ready when knative is not ready",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativeServiceReady(corev1.ConditionFalse),
			},
			enabled:        map[string]bool{},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: cappv1alpha1.CappReadyReasonKnativeNotReady,
		},
		{
			name: "not ready when latest revision differs from latest ready",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativev1.ServiceStatus{
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
			},
			enabled:        map[string]bool{},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: cappv1alpha1.CappReadyReasonKnativeNotReady,
		},

		// --- Logging ---
		{
			name: "not ready when logging is enabled and has problems",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativeServiceReady(corev1.ConditionTrue),
				LoggingStatus: cappv1alpha1.LoggingStatus{
					Conditions: []metav1.Condition{
						{Type: loggingReady, Status: metav1.ConditionFalse, Reason: loggingResourceInvalid, Message: "flow has errors"},
					},
				},
			},
			enabled:        map[string]bool{rmanagers.SyslogNGFlow: true},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: cappv1alpha1.CappReadyReasonLoggingNotReady,
		},
		{
			name: "ready when logging is NOT enabled even though conditions are false",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativeServiceReady(corev1.ConditionTrue),
				LoggingStatus: cappv1alpha1.LoggingStatus{
					Conditions: []metav1.Condition{
						{Type: loggingReady, Status: metav1.ConditionFalse, Reason: loggingResourceInvalid},
					},
				},
			},
			enabled:        map[string]bool{},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: cappv1alpha1.CappReadyReasonReady,
		},

		// --- DomainMapping ---
		{
			name: "not ready when domain mapping is enabled and not ready",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativeServiceReady(corev1.ConditionTrue),
				RouteStatus: cappv1alpha1.RouteStatus{
					DomainMappingObjectStatus: domainMappingStatus(corev1.ConditionFalse),
				},
			},
			enabled:        map[string]bool{rmanagers.DomainMapping: true},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: cappv1alpha1.CappReadyReasonDomainMappingNotReady,
		},
		{
			name: "ready when domain mapping is NOT enabled even though DM is false",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativeServiceReady(corev1.ConditionTrue),
				RouteStatus: cappv1alpha1.RouteStatus{
					DomainMappingObjectStatus: domainMappingStatus(corev1.ConditionFalse),
				},
			},
			enabled:        map[string]bool{},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: cappv1alpha1.CappReadyReasonReady,
		},

		// --- Certificate ---
		{
			name: "not ready when certificate is enabled and not ready",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativeServiceReady(corev1.ConditionTrue),
				RouteStatus: cappv1alpha1.RouteStatus{
					CertificateObjectStatus: certificateStatus(cmmeta.ConditionFalse),
				},
			},
			enabled:        map[string]bool{rmanagers.Certificate: true},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: cappv1alpha1.CappReadyReasonCertificateNotReady,
		},
		{
			name: "ready when certificate is NOT enabled even though cert is false",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativeServiceReady(corev1.ConditionTrue),
				RouteStatus: cappv1alpha1.RouteStatus{
					CertificateObjectStatus: certificateStatus(cmmeta.ConditionFalse),
				},
			},
			enabled:        map[string]bool{},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: cappv1alpha1.CappReadyReasonReady,
		},

		// --- NFS Volumes ---
		{
			name: "not ready when NFS volumes enabled and not bound",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativeServiceReady(corev1.ConditionTrue),
				VolumesStatus:       nfsVolumesUnbound("shared-data"),
			},
			enabled:        map[string]bool{rmanagers.NfsPvc: true},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: cappv1alpha1.CappReadyReasonVolumesNotReady,
		},
		{
			name: "ready when NFS volumes enabled and bound",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativeServiceReady(corev1.ConditionTrue),
				VolumesStatus:       nfsVolumesBound("shared-data"),
			},
			enabled:        map[string]bool{rmanagers.NfsPvc: true},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: cappv1alpha1.CappReadyReasonReady,
		},

		// --- Eventing ---
		{
			name: "not ready when PingSource enabled and event source not ready",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativeServiceReady(corev1.ConditionTrue),
				EventingStatus: cappv1alpha1.EventingStatus{
					EventSources: []cappv1alpha1.EventSourceStatus{
						{Name: pingEventSource, Condition: kapis.Condition{Status: corev1.ConditionFalse}},
					},
				},
			},
			enabled:        map[string]bool{rmanagers.PingSource: true},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: cappv1alpha1.CappReadyReasonEventingNotReady,
		},
		{
			name: "ready when PingSource enabled and all event sources ready",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativeServiceReady(corev1.ConditionTrue),
				EventingStatus: cappv1alpha1.EventingStatus{
					EventSources: []cappv1alpha1.EventSourceStatus{
						{Name: pingEventSource, Condition: kapis.Condition{Status: corev1.ConditionTrue}},
					},
				},
			},
			enabled:        map[string]bool{rmanagers.PingSource: true},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: cappv1alpha1.CappReadyReasonReady,
		},
		{
			name: "not ready when KafkaSource enabled and event source not ready",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativeServiceReady(corev1.ConditionTrue),
				EventingStatus: cappv1alpha1.EventingStatus{
					EventSources: []cappv1alpha1.EventSourceStatus{
						{Name: kafkaEventSource, Condition: kapis.Condition{Status: corev1.ConditionFalse}},
					},
				},
			},
			enabled:        map[string]bool{rmanagers.KafkaSource: true},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: cappv1alpha1.CappReadyReasonEventingNotReady,
		},
		{
			name: "ready when mixed event sources enabled and all ready",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativeServiceReady(corev1.ConditionTrue),
				EventingStatus: cappv1alpha1.EventingStatus{
					EventSources: []cappv1alpha1.EventSourceStatus{
						{Name: pingEventSource, Condition: kapis.Condition{Status: corev1.ConditionTrue}},
						{Name: kafkaEventSource, Condition: kapis.Condition{Status: corev1.ConditionTrue}},
					},
				},
			},
			enabled: map[string]bool{
				rmanagers.PingSource:  true,
				rmanagers.KafkaSource: true,
			},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: cappv1alpha1.CappReadyReasonReady,
		},

		// --- Cascade order: logging before knative ---
		{
			name: "logging failure reported before knative failure",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativeServiceReady(corev1.ConditionFalse),
				LoggingStatus: cappv1alpha1.LoggingStatus{
					Conditions: []metav1.Condition{
						{Type: loggingReady, Status: metav1.ConditionFalse, Reason: loggingResourceInvalid, Message: "flow err"},
					},
				},
			},
			enabled:        map[string]bool{rmanagers.SyslogNGFlow: true},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: cappv1alpha1.CappReadyReasonLoggingNotReady,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := tt.status
			managers := buildManagers(tt.enabled)
			buildCappConditions(&status, capp, managers)

			cond := readyCondition(&status)
			require.NotNil(t, cond, "Ready condition should be set")
			assert.Equal(t, tt.expectedStatus, cond.Status)
			assert.Equal(t, tt.expectedReason, cond.Reason)
		})
	}
}

func TestBuildCappConditionsPreservesExistingConditions(t *testing.T) {
	status := cappv1alpha1.CappStatus{
		KnativeObjectStatus: knativeServiceReady(corev1.ConditionTrue),
		Conditions: []metav1.Condition{
			{Type: "SomeOtherCondition", Status: metav1.ConditionTrue, Reason: "test"},
		},
	}

	managers := buildManagers(map[string]bool{})
	buildCappConditions(&status, cappv1alpha1.Capp{}, managers)

	assert.Len(t, status.Conditions, 2)
	cond := readyCondition(&status)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)

	var other *metav1.Condition
	for i := range status.Conditions {
		if status.Conditions[i].Type == "SomeOtherCondition" {
			other = &status.Conditions[i]
		}
	}
	require.NotNil(t, other)
	assert.Equal(t, metav1.ConditionTrue, other.Status)
}
