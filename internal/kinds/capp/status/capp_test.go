package status

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	nfspvcv1alpha1 "github.com/dana-team/nfspvc-operator/api/v1alpha1"
	dnsrecordv1alpha1 "github.com/dana-team/provider-dns-v2/apis/namespaced/record/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kapis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
)

const (
	readyRevision   = "rev-1"
	pendingRevision = "rev-2"
	pingEventSource = "ping-src"
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
				Conditions: []metav1.Condition{
					{Type: nfspvcv1alpha1.ConditionReady, Status: metav1.ConditionTrue},
				},
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
					Conditions: []metav1.Condition{
						{Type: nfspvcv1alpha1.ConditionReady, Status: metav1.ConditionFalse},
					},
				},
			},
		},
	}
}

func TestComputeReadyCondition(t *testing.T) {
	capp := cappv1alpha1.Capp{}

	tests := []struct {
		name           string
		status         cappv1alpha1.CappStatus
		enabled        map[string]bool
		syncErrors     []error
		expectedStatus metav1.ConditionStatus
		expectedReason string
		expectedMsg    string
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
			name: "not ready when knative is not ready",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativeServiceReady(corev1.ConditionFalse),
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

		// --- Resource sync errors ---
		{
			name: "not ready when a single resource manager fails, even though knative is ready",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativeServiceReady(corev1.ConditionTrue),
			},
			enabled:        map[string]bool{},
			syncErrors:     []error{errors.New("KnativeService: failed to create resource Service capp-name: admission webhook denied the request")},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: cappv1alpha1.CappReadyReasonResourceSyncFailed,
			expectedMsg:    "KnativeService: failed to create resource Service capp-name: admission webhook denied the request",
		},
		{
			name: "sync errors take priority over an otherwise-ready cascade",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativeServiceReady(corev1.ConditionTrue),
				VolumesStatus:       nfsVolumesBound("shared-data"),
			},
			enabled:        map[string]bool{rmanagers.NfsPvc: true},
			syncErrors:     []error{errors.New("NfsPvc: conflict")},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: cappv1alpha1.CappReadyReasonResourceSyncFailed,
			expectedMsg:    "NfsPvc: conflict",
		},
		{
			name: "only the first of multiple sync errors is surfaced in the condition message",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativeServiceReady(corev1.ConditionTrue),
			},
			enabled: map[string]bool{},
			syncErrors: []error{
				errors.New("Certificate: cert failed"),
				errors.New("KnativeService: ksvc failed"),
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: cappv1alpha1.CappReadyReasonResourceSyncFailed,
			expectedMsg:    "Certificate: cert failed",
		},
		{
			name: "empty (non-nil) sync errors slice falls through to the existing cascade unchanged",
			status: cappv1alpha1.CappStatus{
				KnativeObjectStatus: knativeServiceReady(corev1.ConditionTrue),
			},
			enabled:        map[string]bool{},
			syncErrors:     []error{},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: cappv1alpha1.CappReadyReasonReady,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := tt.status
			managers := buildManagers(tt.enabled)
			condition := computeReadyCondition(&status, capp, managers, tt.syncErrors)
			meta.SetStatusCondition(&status.Conditions, condition)

			cond := readyCondition(&status)
			require.NotNil(t, cond, "Ready condition should be set")
			assert.Equal(t, tt.expectedStatus, cond.Status)
			assert.Equal(t, tt.expectedReason, cond.Reason)
			if tt.expectedMsg != "" {
				assert.Equal(t, tt.expectedMsg, cond.Message)
			}
		})
	}
}

func TestFormatSyncErrorTruncatesLongMessages(t *testing.T) {
	longErr := errors.New(strings.Repeat("x", maxSyncErrorMessageLen+1000))

	got := formatSyncError(longErr)

	assert.LessOrEqual(t, len(got), maxSyncErrorMessageLen+len("...(truncated)"))
	assert.True(t, strings.HasSuffix(got, "...(truncated)"))
}

func TestBuildCappConditionsPreservesExistingConditions(t *testing.T) {
	status := cappv1alpha1.CappStatus{
		KnativeObjectStatus: knativeServiceReady(corev1.ConditionTrue),
		Conditions: []metav1.Condition{
			{Type: "SomeOtherCondition", Status: metav1.ConditionTrue, Reason: "test"},
		},
	}

	managers := buildManagers(map[string]bool{})
	condition := computeReadyCondition(&status, cappv1alpha1.Capp{}, managers, nil)
	meta.SetStatusCondition(&status.Conditions, condition)

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
					ResourceStatus: xpv1.ResourceStatus{
						ConditionedStatus: xpv1.ConditionedStatus{
							Conditions: []xpv1.Condition{
								{Type: xpv1.TypeReady, Status: corev1.ConditionTrue, LastTransitionTime: ts},
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
