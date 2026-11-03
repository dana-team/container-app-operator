package controllers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/rollout"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	knativeapis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const (
	rolloutNS      = "rollout-ns"
	rolloutAppName = "app"
)

func rolloutTestCapp(annotation string) *cappv1alpha1.Capp {
	capp := &cappv1alpha1.Capp{
		ObjectMeta: metav1.ObjectMeta{Name: rolloutAppName, Namespace: rolloutNS},
		Spec: cappv1alpha1.CappSpec{RouteSpec: cappv1alpha1.RouteSpec{
			RolloutStrategy: &cappv1alpha1.RolloutStrategy{Stages: []cappv1alpha1.RolloutStage{
				{Percent: 10, Duration: &metav1.Duration{Duration: 5 * time.Minute}},
				{Percent: 100},
			}},
		}},
		Status: cappv1alpha1.CappStatus{Rollout: &cappv1alpha1.RolloutStatus{
			StableRevision: "r1",
			Phase:          cappv1alpha1.RolloutPhaseStable,
		}},
	}
	if annotation != "" {
		capp.Annotations = map[string]string{cappmeta.RolloutAnnotationKey: annotation}
	}
	return capp
}

func rolloutTestKsvc() *knativev1.Service {
	return &knativev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: rolloutAppName, Namespace: rolloutNS},
		Status: knativev1.ServiceStatus{ConfigurationStatusFields: knativev1.ConfigurationStatusFields{
			LatestCreatedRevisionName: "r2",
			LatestReadyRevisionName:   "r1",
		}},
	}
}

func rolloutTestRevision() *knativev1.Revision {
	return &knativev1.Revision{
		ObjectMeta: metav1.ObjectMeta{Name: "r2", Namespace: rolloutNS},
		Status: knativev1.RevisionStatus{Status: duckv1.Status{Conditions: duckv1.Conditions{
			{Type: knativeapis.ConditionReady, Status: corev1.ConditionTrue},
		}}},
	}
}

func newRolloutReconciler(t *testing.T, objs ...client.Object) (*CappReconciler, *events.FakeRecorder) {
	t.Helper()
	scheme := runtime.NewScheme()
	utilruntime.Must(cappv1alpha1.AddToScheme(scheme))
	utilruntime.Must(knativev1.AddToScheme(scheme))
	recorder := events.NewFakeRecorder(10)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return &CappReconciler{Client: c, Scheme: scheme, EventRecorder: recorder}, recorder
}

func TestRunRollout_SetsStatusAndRequeue(t *testing.T) {
	capp := rolloutTestCapp("")
	r, _ := newRolloutReconciler(t, capp.DeepCopy(), rolloutTestKsvc(), rolloutTestRevision())

	d, err := r.runRollout(context.Background(), capp)
	require.NoError(t, err)
	require.NotNil(t, capp.Status.Rollout)
	assert.Equal(t, "r2", capp.Status.Rollout.CandidateRevision)
	// Knative has not confirmed the split, so the stage timer waits and the operator checks again soon.
	assert.Nil(t, capp.Status.Rollout.StageStartTime)
	assert.Equal(t, 30*time.Second, d)
}

func TestRunRollout_KeepsAnnotation(t *testing.T) {
	annotation := cappmeta.RolloutActionPromote + ":r2"
	capp := rolloutTestCapp(annotation)
	r, _ := newRolloutReconciler(t, capp.DeepCopy(), rolloutTestKsvc(), rolloutTestRevision())

	_, err := r.runRollout(context.Background(), capp)
	require.NoError(t, err)

	// A failed status write after runRollout must leave the action for the next reconcile.
	stored := &cappv1alpha1.Capp{}
	require.NoError(t, r.Get(context.Background(), types.NamespacedName{Namespace: rolloutNS, Name: rolloutAppName}, stored))
	assert.Equal(t, annotation, stored.Annotations[cappmeta.RolloutAnnotationKey])
	assert.Equal(t, annotation, capp.Annotations[cappmeta.RolloutAnnotationKey])
}

func TestRemoveRolloutAnnotation(t *testing.T) {
	capp := rolloutTestCapp(cappmeta.RolloutActionPromote)
	r, _ := newRolloutReconciler(t, capp.DeepCopy())

	require.NoError(t, r.removeRolloutAnnotation(context.Background(), capp))

	stored := &cappv1alpha1.Capp{}
	require.NoError(t, r.Get(context.Background(), types.NamespacedName{Namespace: rolloutNS, Name: rolloutAppName}, stored))
	assert.NotContains(t, stored.Annotations, cappmeta.RolloutAnnotationKey)
	assert.NotContains(t, capp.Annotations, cappmeta.RolloutAnnotationKey)

	// Without the annotation it is a no-op.
	require.NoError(t, r.removeRolloutAnnotation(context.Background(), capp))
}

func TestRunRollout_InvalidAnnotation(t *testing.T) {
	capp := rolloutTestCapp("promot")
	r, recorder := newRolloutReconciler(t, capp.DeepCopy(), rolloutTestKsvc(), rolloutTestRevision())

	_, err := r.runRollout(context.Background(), capp)
	require.NoError(t, err)

	select {
	case e := <-recorder.Events:
		assert.True(t, strings.Contains(e, corev1.EventTypeWarning) && strings.Contains(e, "RolloutInvalidAction"), e)
	default:
		t.Fatal("expected a RolloutInvalidAction event")
	}
	assert.Equal(t, "r1", capp.Status.Rollout.StableRevision)
	assert.NotEqual(t, cappv1alpha1.RolloutPhaseStable, capp.Status.Rollout.Phase)
}

func TestRunRollout_NoKsvc(t *testing.T) {
	capp := rolloutTestCapp("")
	r, _ := newRolloutReconciler(t, capp.DeepCopy())

	d, err := r.runRollout(context.Background(), capp)
	require.NoError(t, err)
	assert.Nil(t, capp.Status.Rollout)
	assert.Zero(t, d)
}

func TestRunRollout_BareActionIsInvalid(t *testing.T) {
	capp := rolloutTestCapp(cappmeta.RolloutActionPromote)
	r, recorder := newRolloutReconciler(t, capp.DeepCopy(), rolloutTestKsvc(), rolloutTestRevision())

	_, err := r.runRollout(context.Background(), capp)
	require.NoError(t, err)

	select {
	case e := <-recorder.Events:
		assert.True(t, strings.Contains(e, corev1.EventTypeWarning) && strings.Contains(e, "RolloutInvalidAction"), e)
	default:
		t.Fatal("expected a RolloutInvalidAction event")
	}
	assert.Equal(t, "r2", capp.Status.Rollout.CandidateRevision)
	assert.Equal(t, cappv1alpha1.RolloutPhaseProgressing, capp.Status.Rollout.Phase)
}

func TestRunRollout_AppliedTrafficStartsStageTimer(t *testing.T) {
	capp := rolloutTestCapp("")
	ksvc := rolloutTestKsvc()
	ksvc.Generation = 2
	ksvc.Status.ObservedGeneration = 2
	ksvc.Status.Conditions = duckv1.Conditions{{Type: knativev1.ServiceConditionRoutesReady, Status: corev1.ConditionTrue}}
	ksvc.Status.Traffic = []knativev1.TrafficTarget{
		{RevisionName: "r1", Percent: ptr.To(int64(90)), Tag: rollout.TagStable},
		{RevisionName: "r2", Percent: ptr.To(int64(10)), Tag: rollout.TagCandidate},
	}
	r, _ := newRolloutReconciler(t, capp.DeepCopy(), ksvc, rolloutTestRevision())

	d, err := r.runRollout(context.Background(), capp)
	require.NoError(t, err)
	require.NotNil(t, capp.Status.Rollout.StageStartTime)
	assert.Equal(t, 5*time.Minute, d)
}

func TestRunRollout_ServedTrafficBoundsStrategyEdit(t *testing.T) {
	// The Knative Service status is stale, so the split is not confirmed and AppliedTraffic is nil. The traffic Knative
	// still reports bounds the share a strategy edit keeps: the candidate holds at the 10% served, not the 50% pending.
	capp := rolloutTestCapp("")
	capp.Spec.RouteSpec.RolloutStrategy = &cappv1alpha1.RolloutStrategy{Stages: []cappv1alpha1.RolloutStage{
		{Percent: 40, Duration: &metav1.Duration{Duration: 5 * time.Minute}},
		{Percent: 100},
	}}
	capp.Status.Rollout = &cappv1alpha1.RolloutStatus{
		StableRevision:     "r1",
		CandidateRevision:  "r2",
		Phase:              cappv1alpha1.RolloutPhaseProgressing,
		StageIndex:         1,
		CandidateReadyOnce: true,
		CandidateSince:     &metav1.Time{Time: time.Now().Add(-time.Minute)},
		Traffic: []cappv1alpha1.RevisionTraffic{
			{RevisionName: "r1", Percent: 50, Tag: rollout.TagStable},
			{RevisionName: "r2", Percent: 50, Tag: rollout.TagCandidate},
		},
	}
	ksvc := rolloutTestKsvc()
	ksvc.Generation = 2
	ksvc.Status.ObservedGeneration = 1
	ksvc.Status.Traffic = []knativev1.TrafficTarget{
		{RevisionName: "r1", Percent: ptr.To(int64(90)), Tag: rollout.TagStable},
		{RevisionName: "r2", Percent: ptr.To(int64(10)), Tag: rollout.TagCandidate},
	}
	r, _ := newRolloutReconciler(t, capp.DeepCopy(), ksvc, rolloutTestRevision())

	_, err := r.runRollout(context.Background(), capp)
	require.NoError(t, err)
	assert.Equal(t, cappv1alpha1.RolloutPhaseHolding, capp.Status.Rollout.Phase)
	require.NotNil(t, capp.Status.Rollout.HeldPercent)
	assert.Equal(t, int64(10), *capp.Status.Rollout.HeldPercent)
}

func TestAppliedTraffic(t *testing.T) {
	split := []knativev1.TrafficTarget{
		{RevisionName: "r1", Percent: ptr.To(int64(90)), Tag: rollout.TagStable},
		{RevisionName: "r2", Percent: ptr.To(int64(10)), Tag: rollout.TagCandidate},
	}
	routesReady := duckv1.Conditions{{Type: knativev1.ServiceConditionRoutesReady, Status: corev1.ConditionTrue}}
	routesNotReady := duckv1.Conditions{{Type: knativev1.ServiceConditionRoutesReady, Status: corev1.ConditionFalse}}
	tests := []struct {
		name       string
		generation int64
		observed   int64
		conditions duckv1.Conditions
		traffic    []knativev1.TrafficTarget
		want       []cappv1alpha1.RevisionTraffic
	}{
		{
			name:       "confirmed split",
			generation: 2,
			observed:   2,
			conditions: routesReady,
			traffic:    split,
			want: []cappv1alpha1.RevisionTraffic{
				{RevisionName: "r1", Percent: 90, Tag: rollout.TagStable},
				{RevisionName: "r2", Percent: 10, Tag: rollout.TagCandidate},
			},
		},
		{
			name:       "missing percent counts as zero",
			generation: 2,
			observed:   2,
			conditions: routesReady,
			traffic:    []knativev1.TrafficTarget{{RevisionName: "r1", Percent: ptr.To(int64(100))}, {RevisionName: "r2"}},
			want: []cappv1alpha1.RevisionTraffic{
				{RevisionName: "r1", Percent: 100},
				{RevisionName: "r2", Percent: 0},
			},
		},
		{name: "stale status", generation: 3, observed: 2, conditions: routesReady, traffic: split},
		{name: "routes not ready", generation: 2, observed: 2, conditions: routesNotReady, traffic: split},
		{name: "routes condition missing", generation: 2, observed: 2, traffic: split},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ksvc := &knativev1.Service{ObjectMeta: metav1.ObjectMeta{Generation: tt.generation}}
			ksvc.Status.ObservedGeneration = tt.observed
			ksvc.Status.Conditions = tt.conditions
			ksvc.Status.Traffic = tt.traffic
			assert.Equal(t, tt.want, appliedTraffic(ksvc))
		})
	}
}

func TestRemoveRolloutAnnotation_ChangedValueRequeues(t *testing.T) {
	capp := rolloutTestCapp(cappmeta.RolloutActionPromote + ":r2")
	stored := capp.DeepCopy()
	// The user changes the annotation after the reconcile read it.
	stored.Annotations[cappmeta.RolloutAnnotationKey] = cappmeta.RolloutActionAbort + ":r2"

	scheme := runtime.NewScheme()
	utilruntime.Must(cappv1alpha1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stored).WithInterceptorFuncs(interceptor.Funcs{
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			if err := c.Patch(ctx, obj, patch, opts...); err != nil {
				// The API server reports a failed JSON patch test as 422 Invalid.
				return apierrors.NewInvalid(schema.GroupKind{Group: cappv1alpha1.GroupVersion.Group, Kind: "Capp"}, obj.GetName(), nil)
			}
			return nil
		},
	}).Build()
	r := &CappReconciler{Client: c}

	require.ErrorIs(t, r.removeRolloutAnnotation(context.Background(), capp), errRolloutAnnotationChanged)

	got := &cappv1alpha1.Capp{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: rolloutNS, Name: rolloutAppName}, got))
	assert.Equal(t, cappmeta.RolloutActionAbort+":r2", got.Annotations[cappmeta.RolloutAnnotationKey])
}

func TestRemoveRolloutAnnotation_PatchTestsProcessedValue(t *testing.T) {
	capp := rolloutTestCapp(cappmeta.RolloutActionPromote + ":r2")
	var body []byte

	scheme := runtime.NewScheme()
	utilruntime.Must(cappv1alpha1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(capp.DeepCopy()).WithInterceptorFuncs(interceptor.Funcs{
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			data, err := patch.Data(obj)
			require.NoError(t, err)
			body = data
			return c.Patch(ctx, obj, patch, opts...)
		},
	}).Build()
	r := &CappReconciler{Client: c}

	require.NoError(t, r.removeRolloutAnnotation(context.Background(), capp))

	var ops []map[string]any
	require.NoError(t, json.Unmarshal(body, &ops))
	require.Len(t, ops, 2)
	assert.Equal(t, "test", ops[0]["op"])
	assert.Equal(t, "/metadata/annotations/rcs.dana.io~1rollout", ops[0]["path"])
	assert.Equal(t, cappmeta.RolloutActionPromote+":r2", ops[0]["value"])
	assert.Equal(t, "remove", ops[1]["op"])
	assert.Equal(t, "/metadata/annotations/rcs.dana.io~1rollout", ops[1]["path"])
}

func TestRunRollout_NoStrategyActionWarns(t *testing.T) {
	capp := rolloutTestCapp(cappmeta.RolloutActionPromote + ":r2")
	capp.Spec.RouteSpec.RolloutStrategy = nil
	r, recorder := newRolloutReconciler(t, capp.DeepCopy(), rolloutTestKsvc(), rolloutTestRevision())

	_, err := r.runRollout(context.Background(), capp)
	require.NoError(t, err)

	select {
	case e := <-recorder.Events:
		assert.Contains(t, e, corev1.EventTypeWarning)
		assert.Contains(t, e, "RolloutInvalidAction")
		assert.Contains(t, e, "no rollout strategy")
	default:
		t.Fatal("expected a RolloutInvalidAction event")
	}
}
