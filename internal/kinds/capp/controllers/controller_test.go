package controllers

import (
	"context"
	"errors"
	"testing"
	"time"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	nfspvcv1alpha1 "github.com/dana-team/nfspvc-operator/api/v1alpha1"
	dnsrecordv1alpha1 "github.com/dana-team/provider-dns-v2/apis/namespaced/record/v1alpha1"
	"github.com/go-logr/logr"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	kafkasourcev1 "knative.dev/eventing-kafka-broker/control-plane/pkg/apis/sources/v1"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	knativeapis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/serving/pkg/apis/serving"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	conditionTypeReady = "Ready"
	cappNameA          = "app-a"
	cappNameB          = "app-b"
	cappNameC          = "app-c"
	nsName1            = "ns-1"
	nsName2            = "ns-2"
	appContainerName   = "app"
)

func TestHasConflictError(t *testing.T) {
	conflictErr := apierrors.NewConflict(schema.GroupResource{Resource: "capps"}, "capp-a", errors.New("conflict"))
	plainErr := errors.New("boom")

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "nil error", err: nil, expected: false},
		{name: "direct conflict error", err: conflictErr, expected: true},
		{name: "direct non-conflict error", err: plainErr, expected: false},
		{name: "aggregate with no conflict inside", err: utilerrors.NewAggregate([]error{plainErr, errors.New("other")}), expected: false},
		{name: "aggregate with a conflict among other errors", err: utilerrors.NewAggregate([]error{plainErr, conflictErr}), expected: false},
		{name: "aggregate with only conflicts", err: utilerrors.NewAggregate([]error{conflictErr, apierrors.NewConflict(schema.GroupResource{Resource: "capps"}, "capp-b", errors.New("conflict"))}), expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, hasConflictError(tt.err))
		})
	}
}

func TestConditionStatusChanged(t *testing.T) {
	tests := []struct {
		name     string
		oldConds []conditionPair
		newConds []conditionPair
		condType string
		expected bool
	}{
		{
			name:     "no change when both have same status",
			oldConds: []conditionPair{{condType: conditionTypeReady, status: string(metav1.ConditionTrue)}},
			newConds: []conditionPair{{condType: conditionTypeReady, status: string(metav1.ConditionTrue)}},
			condType: conditionTypeReady,
			expected: false,
		},
		{
			name:     "no change when neither has the condition",
			oldConds: []conditionPair{},
			newConds: []conditionPair{},
			condType: conditionTypeReady,
			expected: false,
		},
		{
			name:     "changed when status transitions",
			oldConds: []conditionPair{{condType: conditionTypeReady, status: string(metav1.ConditionFalse)}},
			newConds: []conditionPair{{condType: conditionTypeReady, status: string(metav1.ConditionTrue)}},
			condType: conditionTypeReady,
			expected: true,
		},
		{
			name:     "changed when condition appears",
			oldConds: []conditionPair{},
			newConds: []conditionPair{{condType: conditionTypeReady, status: string(metav1.ConditionTrue)}},
			condType: conditionTypeReady,
			expected: true,
		},
		{
			name:     "changed when condition disappears",
			oldConds: []conditionPair{{condType: conditionTypeReady, status: string(metav1.ConditionTrue)}},
			newConds: []conditionPair{},
			condType: conditionTypeReady,
			expected: true,
		},
		{
			name:     "ignores other condition types",
			oldConds: []conditionPair{{condType: conditionTypeReady, status: string(metav1.ConditionTrue)}, {condType: "Synced", status: string(metav1.ConditionFalse)}},
			newConds: []conditionPair{{condType: conditionTypeReady, status: string(metav1.ConditionTrue)}, {condType: "Synced", status: string(metav1.ConditionTrue)}},
			condType: conditionTypeReady,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, conditionStatusChanged(tt.oldConds, tt.newConds, tt.condType))
		})
	}
}

func TestKnativeConditions(t *testing.T) {
	conds := duckv1.Conditions{
		{Type: knativev1beta1.DomainMappingConditionReady, Status: corev1.ConditionTrue},
		{Type: knativev1beta1.DomainMappingConditionIngressReady, Status: corev1.ConditionFalse},
	}
	pairs := knativeConditions(conds)
	assert.Equal(t, []conditionPair{
		{condType: string(knativev1beta1.DomainMappingConditionReady), status: string(metav1.ConditionTrue)},
		{condType: string(knativev1beta1.DomainMappingConditionIngressReady), status: string(metav1.ConditionFalse)},
	}, pairs)
}

func TestCertificateConditions(t *testing.T) {
	conds := []cmapi.CertificateCondition{
		{Type: cmapi.CertificateConditionReady, Status: cmmeta.ConditionTrue},
		{Type: cmapi.CertificateConditionIssuing, Status: cmmeta.ConditionFalse},
	}
	pairs := certificateConditions(conds)
	assert.Equal(t, []conditionPair{
		{condType: string(cmapi.CertificateConditionReady), status: string(metav1.ConditionTrue)},
		{condType: string(cmapi.CertificateConditionIssuing), status: string(metav1.ConditionFalse)},
	}, pairs)
}

func TestDomainMappingWatchPredicate(t *testing.T) {
	pred := domainMappingWatchPredicate()

	makeDM := func(condStatus corev1.ConditionStatus) *knativev1beta1.DomainMapping {
		dm := &knativev1beta1.DomainMapping{}
		dm.Generation = 1
		if condStatus != "" {
			dm.Status.Conditions = duckv1.Conditions{
				{Type: knativev1beta1.DomainMappingConditionReady, Status: condStatus},
			}
		}
		return dm
	}

	tests := []struct {
		name     string
		oldObj   client.Object
		newObj   client.Object
		expected bool
	}{
		{
			name:     "no change when both Ready=True",
			oldObj:   makeDM(corev1.ConditionTrue),
			newObj:   makeDM(corev1.ConditionTrue),
			expected: false,
		},
		{
			name:     "changed when Ready transitions False to True",
			oldObj:   makeDM(corev1.ConditionFalse),
			newObj:   makeDM(corev1.ConditionTrue),
			expected: true,
		},
		{
			name:     "changed when Ready condition appears",
			oldObj:   makeDM(""),
			newObj:   makeDM(corev1.ConditionTrue),
			expected: true,
		},
		{
			name: "ignores non-Ready condition changes",
			oldObj: func() client.Object {
				dm := makeDM(corev1.ConditionTrue)
				dm.Status.Conditions = append(dm.Status.Conditions, knativeapis.Condition{
					Type: knativev1beta1.DomainMappingConditionIngressReady, Status: corev1.ConditionFalse,
				})
				return dm
			}(),
			newObj: func() client.Object {
				dm := makeDM(corev1.ConditionTrue)
				dm.Status.Conditions = append(dm.Status.Conditions, knativeapis.Condition{
					Type: knativev1beta1.DomainMappingConditionIngressReady, Status: corev1.ConditionTrue,
				})
				return dm
			}(),
			expected: false,
		},
		{
			name:   "changed when the DomainMapping observed generation changes",
			oldObj: makeDM(corev1.ConditionTrue),
			newObj: func() client.Object {
				dm := makeDM(corev1.ConditionTrue)
				dm.Status.ObservedGeneration = 1
				return dm
			}(),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := event.UpdateEvent{ObjectOld: tt.oldObj, ObjectNew: tt.newObj}
			assert.Equal(t, tt.expected, pred.Update(e))
		})
	}
}

func TestCertificateWatchPredicate(t *testing.T) {
	pred := certificateWatchPredicate()

	makeCert := func(condStatus cmmeta.ConditionStatus) *cmapi.Certificate {
		cert := &cmapi.Certificate{}
		cert.Generation = 1
		if condStatus != "" {
			cert.Status.Conditions = []cmapi.CertificateCondition{
				{Type: cmapi.CertificateConditionReady, Status: condStatus},
			}
		}
		return cert
	}

	tests := []struct {
		name     string
		oldObj   client.Object
		newObj   client.Object
		expected bool
	}{
		{
			name:     "no change when both Ready=True",
			oldObj:   makeCert(cmmeta.ConditionTrue),
			newObj:   makeCert(cmmeta.ConditionTrue),
			expected: false,
		},
		{
			name:     "changed when Ready transitions False to True",
			oldObj:   makeCert(cmmeta.ConditionFalse),
			newObj:   makeCert(cmmeta.ConditionTrue),
			expected: true,
		},
		{
			name:     "changed when Ready condition appears",
			oldObj:   makeCert(""),
			newObj:   makeCert(cmmeta.ConditionTrue),
			expected: true,
		},
		{
			name: "ignores non-Ready condition changes",
			oldObj: func() client.Object {
				cert := makeCert(cmmeta.ConditionTrue)
				cert.Status.Conditions = append(cert.Status.Conditions, cmapi.CertificateCondition{
					Type: cmapi.CertificateConditionIssuing, Status: cmmeta.ConditionTrue,
				})
				return cert
			}(),
			newObj: func() client.Object {
				cert := makeCert(cmmeta.ConditionTrue)
				cert.Status.Conditions = append(cert.Status.Conditions, cmapi.CertificateCondition{
					Type: cmapi.CertificateConditionIssuing, Status: cmmeta.ConditionFalse,
				})
				return cert
			}(),
			expected: false,
		},
		{
			name:   "changed when the Ready observed generation changes",
			oldObj: makeCert(cmmeta.ConditionTrue),
			newObj: func() client.Object {
				cert := makeCert(cmmeta.ConditionTrue)
				cert.Status.Conditions[0].ObservedGeneration = 1
				return cert
			}(),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := event.UpdateEvent{ObjectOld: tt.oldObj, ObjectNew: tt.newObj}
			assert.Equal(t, tt.expected, pred.Update(e))
		})
	}
}

func TestCnameRecordConditionChanged(t *testing.T) {
	makeCNAME := func(conds ...xpv2.Condition) *dnsrecordv1alpha1.CNAMERecord {
		rec := &dnsrecordv1alpha1.CNAMERecord{}
		rec.Status.SetConditions(conds...)
		return rec
	}

	tests := []struct {
		name          string
		oldObj        *dnsrecordv1alpha1.CNAMERecord
		newObj        *dnsrecordv1alpha1.CNAMERecord
		conditionType xpv2.ConditionType
		expected      bool
	}{
		{
			name:          "stable when both Ready=True",
			oldObj:        makeCNAME(xpv2.Condition{Type: xpv2.TypeReady, Status: corev1.ConditionTrue}),
			newObj:        makeCNAME(xpv2.Condition{Type: xpv2.TypeReady, Status: corev1.ConditionTrue}),
			conditionType: xpv2.TypeReady,
			expected:      false,
		},
		{
			name:          "detects Ready transition False to True",
			oldObj:        makeCNAME(xpv2.Condition{Type: xpv2.TypeReady, Status: corev1.ConditionFalse}),
			newObj:        makeCNAME(xpv2.Condition{Type: xpv2.TypeReady, Status: corev1.ConditionTrue}),
			conditionType: xpv2.TypeReady,
			expected:      true,
		},
		{
			name:          "no change when neither has the condition",
			oldObj:        makeCNAME(),
			newObj:        makeCNAME(),
			conditionType: xpv2.TypeReady,
			expected:      false,
		},
		{
			name:          "changed when condition appears",
			oldObj:        makeCNAME(),
			newObj:        makeCNAME(xpv2.Condition{Type: xpv2.TypeReady, Status: corev1.ConditionTrue}),
			conditionType: xpv2.TypeReady,
			expected:      true,
		},
		{
			name:          "ignores other condition types",
			oldObj:        makeCNAME(xpv2.Condition{Type: xpv2.TypeReady, Status: corev1.ConditionTrue}, xpv2.Condition{Type: xpv2.TypeSynced, Status: corev1.ConditionFalse}),
			newObj:        makeCNAME(xpv2.Condition{Type: xpv2.TypeReady, Status: corev1.ConditionTrue}, xpv2.Condition{Type: xpv2.TypeSynced, Status: corev1.ConditionTrue}),
			conditionType: xpv2.TypeReady,
			expected:      false,
		},
		{
			name:          "changed when the CNAMERecord observed generation changes",
			oldObj:        makeCNAME(xpv2.Condition{Type: xpv2.TypeReady, Status: corev1.ConditionTrue}),
			newObj:        makeCNAME(xpv2.Condition{Type: xpv2.TypeReady, Status: corev1.ConditionTrue, ObservedGeneration: 1}),
			conditionType: xpv2.TypeReady,
			expected:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, cnameRecordConditionChanged(tt.oldObj, tt.newObj, tt.conditionType))
		})
	}
}

func TestCnameRecordWatchPredicate(t *testing.T) {
	pred := cnameRecordWatchPredicate()

	makeCNAME := func(conds ...xpv2.Condition) *dnsrecordv1alpha1.CNAMERecord {
		rec := &dnsrecordv1alpha1.CNAMERecord{}
		rec.Status.SetConditions(conds...)
		return rec
	}

	t.Run("delete always triggers", func(t *testing.T) {
		e := event.DeleteEvent{Object: makeCNAME()}
		assert.True(t, pred.Delete(e))
	})

	updateTests := []struct {
		name     string
		oldObj   client.Object
		newObj   client.Object
		expected bool
	}{
		{
			name:     "stable when Ready and Synced unchanged",
			oldObj:   makeCNAME(xpv2.Condition{Type: xpv2.TypeReady, Status: corev1.ConditionTrue}, xpv2.Condition{Type: xpv2.TypeSynced, Status: corev1.ConditionTrue}),
			newObj:   makeCNAME(xpv2.Condition{Type: xpv2.TypeReady, Status: corev1.ConditionTrue}, xpv2.Condition{Type: xpv2.TypeSynced, Status: corev1.ConditionTrue}),
			expected: false,
		},
		{
			name:     "triggers when Ready changes",
			oldObj:   makeCNAME(xpv2.Condition{Type: xpv2.TypeReady, Status: corev1.ConditionFalse}, xpv2.Condition{Type: xpv2.TypeSynced, Status: corev1.ConditionTrue}),
			newObj:   makeCNAME(xpv2.Condition{Type: xpv2.TypeReady, Status: corev1.ConditionTrue}, xpv2.Condition{Type: xpv2.TypeSynced, Status: corev1.ConditionTrue}),
			expected: true,
		},
		{
			name:     "triggers when Synced changes",
			oldObj:   makeCNAME(xpv2.Condition{Type: xpv2.TypeReady, Status: corev1.ConditionTrue}, xpv2.Condition{Type: xpv2.TypeSynced, Status: corev1.ConditionFalse}),
			newObj:   makeCNAME(xpv2.Condition{Type: xpv2.TypeReady, Status: corev1.ConditionTrue}, xpv2.Condition{Type: xpv2.TypeSynced, Status: corev1.ConditionTrue}),
			expected: true,
		},
	}

	for _, tt := range updateTests {
		t.Run(tt.name, func(t *testing.T) {
			e := event.UpdateEvent{ObjectOld: tt.oldObj, ObjectNew: tt.newObj}
			assert.Equal(t, tt.expected, pred.Update(e))
		})
	}
}

func TestKnativeServiceWatchPredicate(t *testing.T) {
	pred := knativeServiceWatchPredicate()

	makeSvc := func(latestReady, latestCreated string) *knativev1.Service {
		svc := &knativev1.Service{}
		svc.Generation = 1
		svc.Status.LatestReadyRevisionName = latestReady
		svc.Status.LatestCreatedRevisionName = latestCreated
		return svc
	}

	// trafficNotMigrated mirrors the Service conditions Knative sets while the traffic is
	// still migrating to the latest revision.
	trafficNotMigrated := func() duckv1.Conditions {
		return duckv1.Conditions{
			{Type: knativev1.ServiceConditionConfigurationsReady, Status: corev1.ConditionTrue},
			{
				Type:    knativev1.ServiceConditionRoutesReady,
				Status:  corev1.ConditionUnknown,
				Reason:  "TrafficNotMigrated",
				Message: "Traffic is not yet migrated to the latest revision.",
			},
			{
				Type:    knativeapis.ConditionReady,
				Status:  corev1.ConditionUnknown,
				Reason:  "TrafficNotMigrated",
				Message: "Traffic is not yet migrated to the latest revision.",
			},
		}
	}

	// trafficReady mirrors the Service conditions once the traffic has finished migrating.
	trafficReady := func() duckv1.Conditions {
		return duckv1.Conditions{
			{Type: knativev1.ServiceConditionConfigurationsReady, Status: corev1.ConditionTrue},
			{Type: knativev1.ServiceConditionRoutesReady, Status: corev1.ConditionTrue},
			{Type: knativeapis.ConditionReady, Status: corev1.ConditionTrue},
		}
	}

	restamp := func(conds duckv1.Conditions, at time.Time) duckv1.Conditions {
		out := make(duckv1.Conditions, len(conds))
		copy(out, conds)
		for i := range out {
			out[i].LastTransitionTime = knativeapis.VolatileTime{Inner: metav1.NewTime(at)}
		}
		return out
	}

	setConditions := func(svc *knativev1.Service, conds ...knativeapis.Condition) *knativev1.Service {
		svc.Status.Conditions = conds
		return svc
	}

	tests := []struct {
		name     string
		oldObj   client.Object
		newObj   client.Object
		expected bool
	}{
		{
			name:     "no change when revision names identical",
			oldObj:   makeSvc("rev-1", "rev-2"),
			newObj:   makeSvc("rev-1", "rev-2"),
			expected: false,
		},
		{
			name:     "triggers when LatestReadyRevisionName changes",
			oldObj:   makeSvc("rev-1", "rev-2"),
			newObj:   makeSvc("rev-3", "rev-2"),
			expected: true,
		},
		{
			name:     "triggers when LatestCreatedRevisionName changes",
			oldObj:   makeSvc("rev-1", "rev-2"),
			newObj:   makeSvc("rev-1", "rev-4"),
			expected: true,
		},
		{
			name:     "triggers when traffic finishes migrating without a revision name change",
			oldObj:   setConditions(makeSvc("rev-1", "rev-1"), trafficNotMigrated()...),
			newObj:   setConditions(makeSvc("rev-1", "rev-1"), trafficReady()...),
			expected: true,
		},
		{
			name:     "no change when only condition transition timestamps are restamped",
			oldObj:   setConditions(makeSvc("rev-1", "rev-1"), restamp(trafficReady(), time.Unix(0, 0))...),
			newObj:   setConditions(makeSvc("rev-1", "rev-1"), restamp(trafficReady(), time.Unix(3600, 0))...),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := event.UpdateEvent{ObjectOld: tt.oldObj, ObjectNew: tt.newObj}
			assert.Equal(t, tt.expected, pred.Update(e))
		})
	}
}

func TestNfsPvcWatchPredicate(t *testing.T) {
	pred := nfsPvcWatchPredicate()

	makeNfsPvc := func(readyStatus metav1.ConditionStatus, pvcPhase string) *nfspvcv1alpha1.NfsPvc {
		nfspvc := &nfspvcv1alpha1.NfsPvc{}
		nfspvc.Generation = 1
		nfspvc.Status.PvcPhase = pvcPhase
		nfspvc.Status.Conditions = []metav1.Condition{
			{Type: nfspvcv1alpha1.ConditionReady, Status: readyStatus, Reason: "Bound"},
		}
		return nfspvc
	}

	restamp := func(nfspvc *nfspvcv1alpha1.NfsPvc, at time.Time) *nfspvcv1alpha1.NfsPvc {
		for i := range nfspvc.Status.Conditions {
			nfspvc.Status.Conditions[i].LastTransitionTime = metav1.NewTime(at)
		}
		return nfspvc
	}

	tests := []struct {
		name     string
		oldObj   client.Object
		newObj   client.Object
		expected bool
	}{
		{
			name:     "triggers when the volume becomes ready",
			oldObj:   makeNfsPvc(metav1.ConditionFalse, "Pending"),
			newObj:   makeNfsPvc(metav1.ConditionTrue, "Bound"),
			expected: true,
		},
		{
			name:     "no change when status is identical",
			oldObj:   makeNfsPvc(metav1.ConditionTrue, "Bound"),
			newObj:   makeNfsPvc(metav1.ConditionTrue, "Bound"),
			expected: false,
		},
		{
			name:     "no change when only condition transition timestamps are restamped",
			oldObj:   restamp(makeNfsPvc(metav1.ConditionTrue, "Bound"), time.Unix(0, 0)),
			newObj:   restamp(makeNfsPvc(metav1.ConditionTrue, "Bound"), time.Unix(3600, 0)),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := event.UpdateEvent{ObjectOld: tt.oldObj, ObjectNew: tt.newObj}
			assert.Equal(t, tt.expected, pred.Update(e))
		})
	}
}

func TestSyslogNGWatchPredicates(t *testing.T) {
	makeFlow := func(problemsCount int) *loggingv1beta1.SyslogNGFlow {
		flow := &loggingv1beta1.SyslogNGFlow{}
		flow.Generation = 1
		flow.Status.ProblemsCount = problemsCount
		if problemsCount > 0 {
			flow.Status.Problems = []string{"output not found"}
		}
		return flow
	}

	makeOutput := func(problemsCount int) *loggingv1beta1.SyslogNGOutput {
		output := &loggingv1beta1.SyslogNGOutput{}
		output.Generation = 1
		output.Status.ProblemsCount = problemsCount
		if problemsCount > 0 {
			output.Status.Problems = []string{"secret not found"}
		}
		return output
	}

	tests := []struct {
		name     string
		pred     predicate.Predicate
		oldObj   client.Object
		newObj   client.Object
		expected bool
	}{
		{
			name:     "flow triggers when problems are cleared",
			pred:     syslogNGFlowWatchPredicate(),
			oldObj:   makeFlow(1),
			newObj:   makeFlow(0),
			expected: true,
		},
		{
			name:     "flow ignores an unchanged status",
			pred:     syslogNGFlowWatchPredicate(),
			oldObj:   makeFlow(0),
			newObj:   makeFlow(0),
			expected: false,
		},
		{
			name:     "output triggers when problems are cleared",
			pred:     syslogNGOutputWatchPredicate(),
			oldObj:   makeOutput(1),
			newObj:   makeOutput(0),
			expected: true,
		},
		{
			name:     "output ignores an unchanged status",
			pred:     syslogNGOutputWatchPredicate(),
			oldObj:   makeOutput(0),
			newObj:   makeOutput(0),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := event.UpdateEvent{ObjectOld: tt.oldObj, ObjectNew: tt.newObj}
			assert.Equal(t, tt.expected, tt.pred.Update(e))
		})
	}
}

func TestPingSourceWatchPredicate(t *testing.T) {
	pred := pingSourceWatchPredicate()

	makePingSource := func(readyStatus corev1.ConditionStatus) *sourcesv1.PingSource {
		ps := &sourcesv1.PingSource{}
		ps.Generation = 1
		ps.Status.Conditions = duckv1.Conditions{
			{Type: sourcesv1.PingSourceConditionReady, Status: readyStatus},
		}
		return ps
	}

	tests := []struct {
		name     string
		oldObj   client.Object
		newObj   client.Object
		expected bool
	}{
		{
			name:     "triggers when Ready status changes",
			oldObj:   makePingSource(corev1.ConditionFalse),
			newObj:   makePingSource(corev1.ConditionTrue),
			expected: true,
		},
		{
			name:     "no change when Ready status is identical",
			oldObj:   makePingSource(corev1.ConditionTrue),
			newObj:   makePingSource(corev1.ConditionTrue),
			expected: false,
		},
		{
			name:   "changed when the PingSource observed generation changes",
			oldObj: makePingSource(corev1.ConditionTrue),
			newObj: func() client.Object {
				ps := makePingSource(corev1.ConditionTrue)
				ps.Status.ObservedGeneration = 1
				return ps
			}(),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := event.UpdateEvent{ObjectOld: tt.oldObj, ObjectNew: tt.newObj}
			assert.Equal(t, tt.expected, pred.Update(e))
		})
	}
}

func TestKafkaSourceWatchPredicate(t *testing.T) {
	pred := kafkaSourceWatchPredicate()

	makeKafkaSource := func(readyStatus corev1.ConditionStatus) *kafkasourcev1.KafkaSource {
		ks := &kafkasourcev1.KafkaSource{}
		ks.Generation = 1
		ks.Status.Conditions = duckv1.Conditions{
			{Type: kafkasourcev1.KafkaConditionReady, Status: readyStatus},
		}
		return ks
	}

	tests := []struct {
		name     string
		oldObj   client.Object
		newObj   client.Object
		expected bool
	}{
		{
			name:     "triggers when Ready status changes",
			oldObj:   makeKafkaSource(corev1.ConditionFalse),
			newObj:   makeKafkaSource(corev1.ConditionTrue),
			expected: true,
		},
		{
			name:     "no change when Ready status is identical",
			oldObj:   makeKafkaSource(corev1.ConditionTrue),
			newObj:   makeKafkaSource(corev1.ConditionTrue),
			expected: false,
		},
		{
			name:   "changed when the KafkaSource observed generation changes",
			oldObj: makeKafkaSource(corev1.ConditionTrue),
			newObj: func() client.Object {
				ks := makeKafkaSource(corev1.ConditionTrue)
				ks.Status.ObservedGeneration = 1
				return ks
			}(),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := event.UpdateEvent{ObjectOld: tt.oldObj, ObjectNew: tt.newObj}
			assert.Equal(t, tt.expected, pred.Update(e))
		})
	}
}

func TestStatusChangedPredicateIgnoresMismatchedTypes(t *testing.T) {
	pred := knativeServiceWatchPredicate()

	e := event.UpdateEvent{
		ObjectOld: &knativev1.Service{},
		ObjectNew: &nfspvcv1alpha1.NfsPvc{},
	}
	assert.False(t, pred.Update(e))
}

func TestFindCappsForCappConfig(t *testing.T) {
	ctx := context.Background()
	cappConfig := &cappv1alpha1.CappConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "capp-config",
			Namespace: "container-app-operator-system",
		},
	}

	tests := []struct {
		name     string
		capps    []client.Object
		expected []reconcile.Request
	}{
		{
			name:     "returns empty when no Capps exist",
			capps:    nil,
			expected: nil,
		},
		{
			name: "returns request for single Capp",
			capps: []client.Object{
				&cappv1alpha1.Capp{
					ObjectMeta: metav1.ObjectMeta{Name: cappNameA, Namespace: nsName1},
				},
			},
			expected: []reconcile.Request{
				{NamespacedName: types.NamespacedName{Name: cappNameA, Namespace: nsName1}},
			},
		},
		{
			name: "returns requests for Capps across namespaces",
			capps: []client.Object{
				&cappv1alpha1.Capp{
					ObjectMeta: metav1.ObjectMeta{Name: cappNameA, Namespace: nsName1},
				},
				&cappv1alpha1.Capp{
					ObjectMeta: metav1.ObjectMeta{Name: cappNameB, Namespace: nsName2},
				},
				&cappv1alpha1.Capp{
					ObjectMeta: metav1.ObjectMeta{Name: cappNameC, Namespace: nsName1},
				},
			},
			expected: []reconcile.Request{
				{NamespacedName: types.NamespacedName{Name: cappNameA, Namespace: nsName1}},
				{NamespacedName: types.NamespacedName{Name: cappNameC, Namespace: nsName1}},
				{NamespacedName: types.NamespacedName{Name: cappNameB, Namespace: nsName2}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(newScheme())
			if len(tt.capps) > 0 {
				builder = builder.WithObjects(tt.capps...)
			}
			r := &CappReconciler{Client: builder.Build()}

			result := r.findCappsForCappConfig(ctx, cappConfig)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestFindCappFromEvent(t *testing.T) {
	r := &CappReconciler{}
	ctx := context.Background()

	object := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cappName,
			Namespace: nsName,
		},
	}

	expected := []reconcile.Request{
		{NamespacedName: types.NamespacedName{Namespace: nsName, Name: cappName}},
	}
	assert.Equal(t, expected, r.findCappFromEvent(ctx, object))
}

func TestFindCappFromLabels(t *testing.T) {
	r := &CappReconciler{}
	ctx := context.Background()
	resourceName := "owned-resource"

	tests := []struct {
		name     string
		object   client.Object
		expected []reconcile.Request
	}{
		{
			name: "returns request when label present",
			object: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: nsName,
					Labels:    map[string]string{cappmeta.CappResourceKey: cappName},
				},
			},
			expected: []reconcile.Request{
				{NamespacedName: types.NamespacedName{Namespace: nsName, Name: cappName}},
			},
		},
		{
			name: "returns nil when label missing",
			object: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: nsName,
					Labels:    map[string]string{},
				},
			},
			expected: nil,
		},
		{
			name: "returns nil when labels are nil",
			object: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: nsName,
				},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, r.findCappFromLabels(ctx, tt.object))
		})
	}
}

func TestFindCappFromRevision(t *testing.T) {
	r := &CappReconciler{}
	ctx := context.Background()

	newRevision := func(namespace string, labels map[string]string) *knativev1.Revision {
		return &knativev1.Revision{ObjectMeta: metav1.ObjectMeta{Name: cappName + "-00001", Namespace: namespace, Labels: labels}}
	}

	tests := []struct {
		name     string
		object   client.Object
		expected []reconcile.Request
	}{
		{
			name:     "maps the revision to the Capp named by the service label in the same namespace",
			object:   newRevision(nsName1, map[string]string{serving.ServiceLabelKey: cappNameA}),
			expected: []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: nsName1, Name: cappNameA}}},
		},
		{
			name:     "returns nil when the service label is missing",
			object:   newRevision(nsName1, map[string]string{}),
			expected: nil,
		},
		{
			name:     "returns nil when the labels are nil",
			object:   newRevision(nsName1, nil),
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, r.findCappFromRevision(ctx, tt.object))
		})
	}
}

func TestRevisionWatchPredicate(t *testing.T) {
	pred := revisionWatchPredicate()

	newRevision := func(ready corev1.ConditionStatus) *knativev1.Revision {
		rev := &knativev1.Revision{}
		rev.Status.Conditions = duckv1.Conditions{{Type: knativeapis.ConditionReady, Status: ready}}
		return rev
	}

	t.Run("triggers on create and delete", func(t *testing.T) {
		rev := newRevision(corev1.ConditionTrue)
		assert.True(t, pred.Create(event.TypedCreateEvent[client.Object]{Object: rev}))
		assert.True(t, pred.Delete(event.TypedDeleteEvent[client.Object]{Object: rev}))
	})

	t.Run("triggers when the Ready status changes", func(t *testing.T) {
		assert.True(t, pred.Update(event.TypedUpdateEvent[client.Object]{
			ObjectOld: newRevision(corev1.ConditionUnknown),
			ObjectNew: newRevision(corev1.ConditionFalse),
		}))
	})

	t.Run("ignores updates that keep the Ready status", func(t *testing.T) {
		assert.False(t, pred.Update(event.TypedUpdateEvent[client.Object]{
			ObjectOld: newRevision(corev1.ConditionTrue),
			ObjectNew: newRevision(corev1.ConditionTrue),
		}))
	})

	t.Run("ignores objects of another type", func(t *testing.T) {
		assert.False(t, pred.Update(event.TypedUpdateEvent[client.Object]{
			ObjectOld: &corev1.ConfigMap{},
			ObjectNew: &corev1.ConfigMap{},
		}))
	})
}

type syncStubManager struct {
	manageErr error
	required  bool
}

func (s syncStubManager) Manage(_ context.Context, _ cappv1alpha1.Capp) error  { return s.manageErr }
func (s syncStubManager) CleanUp(_ context.Context, _ cappv1alpha1.Capp) error { return nil }
func (s syncStubManager) IsRequired(_ cappv1alpha1.Capp) bool                  { return s.required }

func TestSyncApplicationReportsSyncErrors(t *testing.T) {
	conflictErr := apierrors.NewConflict(schema.GroupResource{Resource: "services"}, cappName, errors.New("object was modified"))
	webhookErr := errors.New("admission webhook denied the request")

	tests := []struct {
		name         string
		manageErr    error
		wantConflict bool
		wantReason   string
	}{
		{
			name:         "does not report a conflict in the status",
			manageErr:    conflictErr,
			wantConflict: true,
			wantReason:   cappv1alpha1.CapabilityReasonPending,
		},
		{
			name:       "reports other errors on their capability",
			manageErr:  webhookErr,
			wantReason: cappv1alpha1.CapabilityReasonResourceSyncFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := newScheme()
			utilruntime.Must(sourcesv1.AddToScheme(s))
			utilruntime.Must(kafkasourcev1.AddToScheme(s))
			k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(newCapp()).WithStatusSubresource(&cappv1alpha1.Capp{}).Build()

			names := []string{
				rmanagers.KnativeService, rmanagers.NfsPvc, rmanagers.SyslogNGOutput, rmanagers.SyslogNGFlow, rmanagers.Certificate,
				rmanagers.DomainMapping, rmanagers.DNSRecord, rmanagers.PingSource, rmanagers.KafkaSource,
			}
			entries := make([]rmanagers.ResourceManagerEntry, 0, len(names))
			for _, name := range names {
				manager := syncStubManager{}
				if name == rmanagers.KnativeService {
					manager = syncStubManager{manageErr: tt.manageErr, required: true}
				}
				entries = append(entries, rmanagers.ResourceManagerEntry{Name: name, Manager: manager})
			}
			r := &CappReconciler{Client: k8sClient}

			err := r.SyncApplication(ctx, *newCapp(), entries, &cappv1alpha1.CappConfig{}, logr.Discard())
			assert.Error(t, err)
			assert.Equal(t, tt.wantConflict, hasConflictError(err))

			capp := cappv1alpha1.Capp{}
			assert.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Namespace: nsName, Name: cappName}, &capp))
			serving := meta.FindStatusCondition(capp.Status.Conditions, cappv1alpha1.CappConditionServingReady)
			if assert.NotNil(t, serving) {
				assert.Equal(t, tt.wantReason, serving.Reason)
			}
		})
	}
}

// conflictingKnativeManager fails to update the Knative Service with a conflict and checks its template with the real manager.
type conflictingKnativeManager struct {
	syncStubManager
	templates rmanagers.KnativeServiceManager
}

func (c conflictingKnativeManager) TemplateApplied(ctx context.Context, capp cappv1alpha1.Capp, ksvc *knativev1.Service) bool {
	return c.templates.TemplateApplied(ctx, capp, ksvc)
}

// TestSyncApplicationConflictedKnativeServiceIsNotRolledOut covers a Capp update whose Knative Service update conflicted.
// The conflict is not reported as a sync error, so the Knative Service still holds the old template at the old generation
// and RolloutSucceeded must not be True for the new generation.
func TestSyncApplicationConflictedKnativeServiceIsNotRolledOut(t *testing.T) {
	ctx := context.Background()
	conflictErr := apierrors.NewConflict(schema.GroupResource{Resource: "services"}, cappName, errors.New("object was modified"))

	capp := newCapp()
	capp.Generation = 2
	capp.Spec.ConfigurationSpec.Template.Spec.Containers = []corev1.Container{{Name: appContainerName, Image: "example.com/app:v2"}}
	capp.Status.Rollout = &cappv1alpha1.RolloutStatus{StableRevision: cappName + "-00001", Phase: cappv1alpha1.RolloutPhaseStable}

	ksvc := &knativev1.Service{ObjectMeta: metav1.ObjectMeta{Name: cappName, Namespace: nsName}}
	ksvc.Generation = 1
	ksvc.Spec.Template.Spec.Containers = []corev1.Container{{Name: appContainerName, Image: "example.com/app:v1"}}
	ksvc.Status.ObservedGeneration = 1
	ksvc.Status.LatestCreatedRevisionName = cappName + "-00001"
	ksvc.Status.LatestReadyRevisionName = cappName + "-00001"
	ksvc.Status.Traffic = []knativev1.TrafficTarget{{RevisionName: cappName + "-00001", Percent: ptr.To(int64(100))}}
	ksvc.Status.SetConditions(knativeapis.Conditions{
		{Type: knativeapis.ConditionReady, Status: corev1.ConditionTrue},
		{Type: knativev1.ServiceConditionConfigurationsReady, Status: corev1.ConditionTrue},
		{Type: knativev1.ServiceConditionRoutesReady, Status: corev1.ConditionTrue},
	})

	s := newScheme()
	utilruntime.Must(sourcesv1.AddToScheme(s))
	utilruntime.Must(kafkasourcev1.AddToScheme(s))
	k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(capp, ksvc).WithStatusSubresource(&cappv1alpha1.Capp{}).Build()

	cappConfig := &cappv1alpha1.CappConfig{}
	names := []string{
		rmanagers.KnativeService, rmanagers.NfsPvc, rmanagers.SyslogNGOutput, rmanagers.SyslogNGFlow, rmanagers.Certificate,
		rmanagers.DomainMapping, rmanagers.DNSRecord, rmanagers.PingSource, rmanagers.KafkaSource,
	}
	entries := make([]rmanagers.ResourceManagerEntry, 0, len(names))
	for _, name := range names {
		var manager rmanagers.ResourceManager = syncStubManager{}
		if name == rmanagers.KnativeService {
			manager = conflictingKnativeManager{
				syncStubManager: syncStubManager{manageErr: conflictErr, required: true},
				templates:       rmanagers.KnativeServiceManager{CappConfig: cappConfig},
			}
		}
		entries = append(entries, rmanagers.ResourceManagerEntry{Name: name, Manager: manager})
	}
	r := &CappReconciler{Client: k8sClient}

	err := r.SyncApplication(ctx, *capp, entries, cappConfig, logr.Discard())
	require.Error(t, err)
	require.True(t, hasConflictError(err))

	updated := cappv1alpha1.Capp{}
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Namespace: nsName, Name: cappName}, &updated))
	succeeded := meta.FindStatusCondition(updated.Status.Conditions, cappv1alpha1.CappConditionRolloutSucceeded)
	require.NotNil(t, succeeded)
	assert.Equal(t, metav1.ConditionUnknown, succeeded.Status)
	assert.Equal(t, int64(2), succeeded.ObservedGeneration)
}

func TestSyncApplicationRemovesRolloutAnnotation(t *testing.T) {
	webhookErr := errors.New("admission webhook denied the request")
	statusErr := errors.New("status write failed")

	tests := []struct {
		name           string
		statusWriteErr error
		wantErr        error
		wantAnnotation bool
	}{
		{
			name:           "removes the annotation once the status is persisted despite a manager error",
			wantErr:        webhookErr,
			wantAnnotation: false,
		},
		{
			name:           "keeps the annotation when the status write fails",
			statusWriteErr: statusErr,
			wantErr:        statusErr,
			wantAnnotation: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := newScheme()
			utilruntime.Must(sourcesv1.AddToScheme(s))
			utilruntime.Must(kafkasourcev1.AddToScheme(s))

			capp := newCapp()
			capp.Annotations = map[string]string{cappmeta.RolloutAnnotationKey: cappmeta.RolloutActionPromote}
			interceptorFuncs := interceptor.Funcs{}
			if tt.statusWriteErr != nil {
				interceptorFuncs.SubResourceUpdate = func(_ context.Context, _ client.Client, _ string, _ client.Object, _ ...client.SubResourceUpdateOption) error {
					return tt.statusWriteErr
				}
			}
			k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(capp.DeepCopy()).WithStatusSubresource(&cappv1alpha1.Capp{}).WithInterceptorFuncs(interceptorFuncs).Build()

			names := []string{
				rmanagers.KnativeService, rmanagers.NfsPvc, rmanagers.SyslogNGOutput, rmanagers.SyslogNGFlow, rmanagers.Certificate,
				rmanagers.DomainMapping, rmanagers.DNSRecord, rmanagers.PingSource, rmanagers.KafkaSource,
			}
			entries := make([]rmanagers.ResourceManagerEntry, 0, len(names))
			for _, name := range names {
				manager := syncStubManager{}
				if name == rmanagers.KnativeService {
					manager = syncStubManager{manageErr: webhookErr, required: true}
				}
				entries = append(entries, rmanagers.ResourceManagerEntry{Name: name, Manager: manager})
			}
			r := &CappReconciler{Client: k8sClient}

			err := r.SyncApplication(ctx, *capp, entries, &cappv1alpha1.CappConfig{}, logr.Discard())
			assert.ErrorIs(t, err, tt.wantErr)

			stored := cappv1alpha1.Capp{}
			assert.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Namespace: nsName, Name: cappName}, &stored))
			_, found := stored.Annotations[cappmeta.RolloutAnnotationKey]
			assert.Equal(t, tt.wantAnnotation, found)
		})
	}
}

func TestReconcileRequeuesWhenRolloutAnnotationChanged(t *testing.T) {
	ctx := context.Background()
	s := newScheme()
	utilruntime.Must(sourcesv1.AddToScheme(s))
	utilruntime.Must(kafkasourcev1.AddToScheme(s))

	capp := newCapp()
	capp.Annotations = map[string]string{cappmeta.RolloutAnnotationKey: cappmeta.RolloutActionPromote + ":r2"}
	cappConfig := &cappv1alpha1.CappConfig{ObjectMeta: metav1.ObjectMeta{Name: cappmeta.CappConfigName, Namespace: cappmeta.CappNS}}
	k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(capp, cappConfig).WithStatusSubresource(&cappv1alpha1.Capp{}).WithInterceptorFuncs(interceptor.Funcs{
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			if patch.Type() == types.JSONPatchType {
				// The user changed the annotation after this reconcile read it, so the test op fails with 422 Invalid.
				return apierrors.NewInvalid(schema.GroupKind{Group: cappv1alpha1.GroupVersion.Group, Kind: "Capp"}, obj.GetName(), nil)
			}
			return c.Patch(ctx, obj, patch, opts...)
		},
	}).Build()
	r := &CappReconciler{Client: k8sClient, Scheme: s, EventRecorder: events.NewFakeRecorder(100)}

	res, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: nsName, Name: cappName}})
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, res.RequeueAfter)
}
