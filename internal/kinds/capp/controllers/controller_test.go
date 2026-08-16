package controllers

import (
	"context"
	"errors"
	"testing"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	dnsrecordv1alpha1 "github.com/dana-team/provider-dns-v2/apis/namespaced/record/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	knativeapis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	conditionTypeReady = "Ready"
	cappNameA          = "app-a"
	cappNameB          = "app-b"
	cappNameC          = "app-c"
	nsName1            = "ns-1"
	nsName2            = "ns-2"
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
	makeDM := func(condStatus corev1.ConditionStatus) *knativev1beta1.DomainMapping {
		dm := &knativev1beta1.DomainMapping{}
		if condStatus != "" {
			dm.Status.Conditions = duckv1.Conditions{
				{Type: knativev1beta1.DomainMappingConditionReady, Status: condStatus},
			}
		}
		return dm
	}

	tests := []struct {
		name     string
		oldObj   *knativev1beta1.DomainMapping
		newObj   *knativev1beta1.DomainMapping
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
			oldObj: func() *knativev1beta1.DomainMapping {
				dm := makeDM(corev1.ConditionTrue)
				dm.Status.Conditions = append(dm.Status.Conditions, knativeapis.Condition{
					Type: knativev1beta1.DomainMappingConditionIngressReady, Status: corev1.ConditionFalse,
				})
				return dm
			}(),
			newObj: func() *knativev1beta1.DomainMapping {
				dm := makeDM(corev1.ConditionTrue)
				dm.Status.Conditions = append(dm.Status.Conditions, knativeapis.Condition{
					Type: knativev1beta1.DomainMappingConditionIngressReady, Status: corev1.ConditionTrue,
				})
				return dm
			}(),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := conditionStatusChanged(
				knativeConditions(tt.oldObj.Status.Conditions),
				knativeConditions(tt.newObj.Status.Conditions),
				string(knativev1beta1.DomainMappingConditionReady),
			)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCertificateWatchPredicate(t *testing.T) {
	makeCert := func(condStatus cmmeta.ConditionStatus) *cmapi.Certificate {
		cert := &cmapi.Certificate{}
		if condStatus != "" {
			cert.Status.Conditions = []cmapi.CertificateCondition{
				{Type: cmapi.CertificateConditionReady, Status: condStatus},
			}
		}
		return cert
	}

	tests := []struct {
		name     string
		oldObj   *cmapi.Certificate
		newObj   *cmapi.Certificate
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
			oldObj: func() *cmapi.Certificate {
				cert := makeCert(cmmeta.ConditionTrue)
				cert.Status.Conditions = append(cert.Status.Conditions, cmapi.CertificateCondition{
					Type: cmapi.CertificateConditionIssuing, Status: cmmeta.ConditionTrue,
				})
				return cert
			}(),
			newObj: func() *cmapi.Certificate {
				cert := makeCert(cmmeta.ConditionTrue)
				cert.Status.Conditions = append(cert.Status.Conditions, cmapi.CertificateCondition{
					Type: cmapi.CertificateConditionIssuing, Status: cmmeta.ConditionFalse,
				})
				return cert
			}(),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := conditionStatusChanged(
				certificateConditions(tt.oldObj.Status.Conditions),
				certificateConditions(tt.newObj.Status.Conditions),
				string(cmapi.CertificateConditionReady),
			)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCnameRecordConditionChanged(t *testing.T) {
	makeCNAME := func(conds ...xpv1.Condition) *dnsrecordv1alpha1.CNAMERecord {
		rec := &dnsrecordv1alpha1.CNAMERecord{}
		rec.Status.SetConditions(conds...)
		return rec
	}

	tests := []struct {
		name          string
		oldObj        *dnsrecordv1alpha1.CNAMERecord
		newObj        *dnsrecordv1alpha1.CNAMERecord
		conditionType xpv1.ConditionType
		expected      bool
	}{
		{
			name:          "stable when both Ready=True",
			oldObj:        makeCNAME(xpv1.Condition{Type: xpv1.TypeReady, Status: corev1.ConditionTrue}),
			newObj:        makeCNAME(xpv1.Condition{Type: xpv1.TypeReady, Status: corev1.ConditionTrue}),
			conditionType: xpv1.TypeReady,
			expected:      false,
		},
		{
			name:          "detects Ready transition False to True",
			oldObj:        makeCNAME(xpv1.Condition{Type: xpv1.TypeReady, Status: corev1.ConditionFalse}),
			newObj:        makeCNAME(xpv1.Condition{Type: xpv1.TypeReady, Status: corev1.ConditionTrue}),
			conditionType: xpv1.TypeReady,
			expected:      true,
		},
		{
			name:          "no change when neither has the condition",
			oldObj:        makeCNAME(),
			newObj:        makeCNAME(),
			conditionType: xpv1.TypeReady,
			expected:      false,
		},
		{
			name:          "changed when condition appears",
			oldObj:        makeCNAME(),
			newObj:        makeCNAME(xpv1.Condition{Type: xpv1.TypeReady, Status: corev1.ConditionTrue}),
			conditionType: xpv1.TypeReady,
			expected:      true,
		},
		{
			name:          "ignores other condition types",
			oldObj:        makeCNAME(xpv1.Condition{Type: xpv1.TypeReady, Status: corev1.ConditionTrue}, xpv1.Condition{Type: xpv1.TypeSynced, Status: corev1.ConditionFalse}),
			newObj:        makeCNAME(xpv1.Condition{Type: xpv1.TypeReady, Status: corev1.ConditionTrue}, xpv1.Condition{Type: xpv1.TypeSynced, Status: corev1.ConditionTrue}),
			conditionType: xpv1.TypeReady,
			expected:      false,
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

	makeCNAME := func(conds ...xpv1.Condition) *dnsrecordv1alpha1.CNAMERecord {
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
			oldObj:   makeCNAME(xpv1.Condition{Type: xpv1.TypeReady, Status: corev1.ConditionTrue}, xpv1.Condition{Type: xpv1.TypeSynced, Status: corev1.ConditionTrue}),
			newObj:   makeCNAME(xpv1.Condition{Type: xpv1.TypeReady, Status: corev1.ConditionTrue}, xpv1.Condition{Type: xpv1.TypeSynced, Status: corev1.ConditionTrue}),
			expected: false,
		},
		{
			name:     "triggers when Ready changes",
			oldObj:   makeCNAME(xpv1.Condition{Type: xpv1.TypeReady, Status: corev1.ConditionFalse}, xpv1.Condition{Type: xpv1.TypeSynced, Status: corev1.ConditionTrue}),
			newObj:   makeCNAME(xpv1.Condition{Type: xpv1.TypeReady, Status: corev1.ConditionTrue}, xpv1.Condition{Type: xpv1.TypeSynced, Status: corev1.ConditionTrue}),
			expected: true,
		},
		{
			name:     "triggers when Synced changes",
			oldObj:   makeCNAME(xpv1.Condition{Type: xpv1.TypeReady, Status: corev1.ConditionTrue}, xpv1.Condition{Type: xpv1.TypeSynced, Status: corev1.ConditionFalse}),
			newObj:   makeCNAME(xpv1.Condition{Type: xpv1.TypeReady, Status: corev1.ConditionTrue}, xpv1.Condition{Type: xpv1.TypeSynced, Status: corev1.ConditionTrue}),
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := event.UpdateEvent{ObjectOld: tt.oldObj, ObjectNew: tt.newObj}
			assert.Equal(t, tt.expected, pred.Update(e))
		})
	}
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
