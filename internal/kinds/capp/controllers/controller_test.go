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
	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	dnsrecordv1alpha1 "github.com/dana-team/provider-dns-v2/apis/namespaced/record/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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

// syncStubManager is a configurable rmanagers.ResourceManager used to exercise
// SyncApplication's error-collection behavior without a real managed resource.
type syncStubManager struct {
	manageErr error
	calls     *int
}

func (s syncStubManager) Manage(_ context.Context, _ cappv1alpha1.Capp) error {
	*s.calls++
	return s.manageErr
}
func (s syncStubManager) CleanUp(_ context.Context, _ cappv1alpha1.Capp) error { return nil }
func (s syncStubManager) IsRequired(_ cappv1alpha1.Capp) bool                  { return false }

// allManagerNames mirrors the manager set registered in Reconcile; SyncStatus
// map-indexes by name so every one of these must be present in the test's managers slice.
var allManagerNames = []string{
	rmanagers.KnativeService,
	rmanagers.NfsPvc,
	rmanagers.SyslogNGOutput,
	rmanagers.SyslogNGFlow,
	rmanagers.Certificate,
	rmanagers.DomainMapping,
	rmanagers.DNSRecord,
	rmanagers.PingSource,
	rmanagers.KafkaSource,
}

// newSyncManagers builds one syncStubManager per registered manager name, failing
// with manageErrs[name] where set, and returns the entries plus a per-name call counter.
func newSyncManagers(manageErrs map[string]error) ([]rmanagers.ResourceManagerEntry, map[string]*int) {
	entries := make([]rmanagers.ResourceManagerEntry, 0, len(allManagerNames))
	calls := make(map[string]*int, len(allManagerNames))
	for _, name := range allManagerNames {
		count := 0
		calls[name] = &count
		entries = append(entries, rmanagers.ResourceManagerEntry{
			Name:    name,
			Manager: syncStubManager{manageErr: manageErrs[name], calls: &count},
		})
	}
	return entries, calls
}

func readyConditionFromCapp(capp *cappv1alpha1.Capp) *metav1.Condition {
	for i := range capp.Status.Conditions {
		if capp.Status.Conditions[i].Type == conditionTypeReady {
			return &capp.Status.Conditions[i]
		}
	}
	return nil
}

func TestSyncApplication(t *testing.T) {
	ctx := context.Background()

	newReconciler := func(capp *cappv1alpha1.Capp) (*CappReconciler, *cappv1alpha1.CappConfig) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(newScheme()).
			WithObjects(capp).
			WithStatusSubresource(&cappv1alpha1.Capp{}).
			Build()
		return &CappReconciler{Client: fakeClient}, &cappv1alpha1.CappConfig{}
	}

	getCapp := func(t *testing.T, r *CappReconciler) *cappv1alpha1.Capp {
		t.Helper()
		got := &cappv1alpha1.Capp{}
		require.NoError(t, r.Get(ctx, types.NamespacedName{Namespace: nsName, Name: cappName}, got))
		return got
	}

	t.Run("all managers succeed", func(t *testing.T) {
		capp := newCapp()
		r, cappConfig := newReconciler(capp)
		entries, calls := newSyncManagers(nil)

		err := r.SyncApplication(ctx, *capp, entries, cappConfig, logr.Discard())
		require.NoError(t, err)

		for _, name := range allManagerNames {
			assert.Equal(t, 1, *calls[name], "manager %s should have been called", name)
		}

		got := getCapp(t, r)
		cond := readyConditionFromCapp(got)
		require.NotNil(t, cond, "Ready condition should be set")
		// KnativeService.IsRequired() is false here, but computeReadyCondition checks
		// knativeNotReady unconditionally, so the cascade lands on KnativeServiceNotReady.
		assert.Equal(t, cappv1alpha1.CappReadyReasonKnativeNotReady, cond.Reason)
	})

	t.Run("one manager fails with a non-conflict error", func(t *testing.T) {
		capp := newCapp()
		r, cappConfig := newReconciler(capp)
		manageErr := errors.New("admission webhook denied the request")
		entries, calls := newSyncManagers(map[string]error{rmanagers.KnativeService: manageErr})

		err := r.SyncApplication(ctx, *capp, entries, cappConfig, logr.Discard())
		require.Error(t, err)
		assert.ErrorIs(t, err, manageErr)

		for _, name := range allManagerNames {
			assert.Equal(t, 1, *calls[name], "manager %s should still have been called", name)
		}

		got := getCapp(t, r)
		cond := readyConditionFromCapp(got)
		require.NotNil(t, cond)
		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, cappv1alpha1.CappReadyReasonManagedResourceError, cond.Reason)
		assert.Contains(t, cond.Message, rmanagers.KnativeService)
		assert.Contains(t, cond.Message, "admission webhook denied the request")
	})

	t.Run("two managers fail", func(t *testing.T) {
		capp := newCapp()
		r, cappConfig := newReconciler(capp)
		ksvcErr := errors.New("ksvc denied")
		dmErr := errors.New("domain mapping denied")
		entries, _ := newSyncManagers(map[string]error{
			rmanagers.KnativeService: ksvcErr,
			rmanagers.DomainMapping:  dmErr,
		})

		err := r.SyncApplication(ctx, *capp, entries, cappConfig, logr.Discard())
		require.Error(t, err)
		assert.ErrorIs(t, err, ksvcErr)
		assert.ErrorIs(t, err, dmErr)

		got := getCapp(t, r)
		cond := readyConditionFromCapp(got)
		require.NotNil(t, cond)
		assert.Equal(t, cappv1alpha1.CappReadyReasonManagedResourceError, cond.Reason)
		assert.Contains(t, cond.Message, rmanagers.KnativeService)
		assert.Contains(t, cond.Message, rmanagers.DomainMapping)
	})

	t.Run("conflict error is quietly propagated without touching status", func(t *testing.T) {
		capp := newCapp()
		r, cappConfig := newReconciler(capp)
		conflictErr := apierrors.NewConflict(schema.GroupResource{Resource: "services"}, cappName, errors.New("resourceVersion mismatch"))
		entries, _ := newSyncManagers(map[string]error{rmanagers.KnativeService: conflictErr})

		err := r.SyncApplication(ctx, *capp, entries, cappConfig, logr.Discard())
		require.Error(t, err)
		assert.True(t, apierrors.IsConflict(err))

		got := getCapp(t, r)
		assert.Nil(t, readyConditionFromCapp(got), "status should not be touched on a conflict-only failure")
	})

	t.Run("conflict plus real failure surfaces only the real failure", func(t *testing.T) {
		capp := newCapp()
		r, cappConfig := newReconciler(capp)
		conflictErr := apierrors.NewConflict(schema.GroupResource{Resource: "services"}, cappName, errors.New("resourceVersion mismatch"))
		realErr := errors.New("admission webhook denied the request")
		entries, _ := newSyncManagers(map[string]error{
			rmanagers.KnativeService: conflictErr,
			rmanagers.DomainMapping:  realErr,
		})

		err := r.SyncApplication(ctx, *capp, entries, cappConfig, logr.Discard())
		require.Error(t, err)
		assert.False(t, apierrors.IsConflict(err))
		assert.ErrorIs(t, err, realErr)

		got := getCapp(t, r)
		cond := readyConditionFromCapp(got)
		require.NotNil(t, cond)
		assert.Equal(t, cappv1alpha1.CappReadyReasonManagedResourceError, cond.Reason)
		assert.Contains(t, cond.Message, rmanagers.DomainMapping)
		assert.NotContains(t, cond.Message, rmanagers.KnativeService+":")
	})
}
