package resourcemanagers

import (
	"context"
	"fmt"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/events"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	cappName      = "my-capp"
	cappNamespace = "my-ns"
	sourceName    = "ping"
	schedule      = "* * * * *"
	sourceA       = "ping-a"
	sourceB       = "ping-b"
	sourceC       = "ping-c"
)

func pingSourceName(source string) string {
	return fmt.Sprintf("%s-%s", cappName, source)
}

func newPingSourceScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(cappv1alpha1.AddToScheme(s))
	utilruntime.Must(sourcesv1.AddToScheme(s))
	utilruntime.Must(servingv1.AddToScheme(s))
	return s
}

func newPingSourceManager(k8sClient client.Client) PingSourceManager {
	return PingSourceManager{
		Ctx:           context.Background(),
		K8sclient:     k8sClient,
		Log:           logr.Discard(),
		EventRecorder: events.NewFakeRecorder(10),
	}
}

func newCapp(name, namespace string, sources []cappv1alpha1.SourceConfiguration) cappv1alpha1.Capp {
	return cappv1alpha1.Capp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID("test-uid"),
		},
		Spec: cappv1alpha1.CappSpec{
			EventSourcesSpec: cappv1alpha1.EventSourcesSpec{Sources: sources},
		},
	}
}

func newPingSource(source string) *sourcesv1.PingSource {
	return &sourcesv1.PingSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pingSourceName(source),
			Namespace: cappNamespace,
			Labels:    utils.ManagedResourceLabels(cappName),
		},
	}
}

func TestPingSourceCleanUpOrphans(t *testing.T) {
	pingCfg := &cappv1alpha1.PingSourceConfiguration{Schedule: schedule}
	tests := []struct {
		name          string
		sources       []cappv1alpha1.SourceConfiguration
		preCreate     []*sourcesv1.PingSource
		expectKept    []string
		expectDeleted []string
	}{
		{
			name:          "deletes orphaned PingSource not in spec",
			sources:       []cappv1alpha1.SourceConfiguration{{Name: sourceA, PingSourceConfiguration: pingCfg}},
			preCreate:     []*sourcesv1.PingSource{newPingSource(sourceA), newPingSource(sourceB)},
			expectKept:    []string{pingSourceName(sourceA)},
			expectDeleted: []string{pingSourceName(sourceB)},
		},
		{
			name: "keeps all owned when all are in spec",
			sources: []cappv1alpha1.SourceConfiguration{
				{Name: sourceA, PingSourceConfiguration: pingCfg},
				{Name: sourceB, PingSourceConfiguration: pingCfg},
			},
			preCreate:  []*sourcesv1.PingSource{newPingSource(sourceA), newPingSource(sourceB)},
			expectKept: []string{pingSourceName(sourceA), pingSourceName(sourceB)},
		},
		{
			name:          "deletes all owned when none match spec",
			sources:       []cappv1alpha1.SourceConfiguration{{Name: sourceA, PingSourceConfiguration: pingCfg}},
			preCreate:     []*sourcesv1.PingSource{newPingSource(sourceB), newPingSource(sourceC)},
			expectDeleted: []string{pingSourceName(sourceB), pingSourceName(sourceC)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			fakeClient := fake.NewClientBuilder().WithScheme(newPingSourceScheme()).Build()
			for _, ps := range tt.preCreate {
				assert.NoError(t, fakeClient.Create(ctx, ps))
			}
			pm := newPingSourceManager(fakeClient)
			capp := newCapp(cappName, cappNamespace, tt.sources)
			assert.NoError(t, pm.cleanUpOrphans(capp))
			for _, name := range tt.expectKept {
				got := &sourcesv1.PingSource{}
				assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: cappNamespace}, got))
			}
			for _, name := range tt.expectDeleted {
				got := &sourcesv1.PingSource{}
				getErr := fakeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: cappNamespace}, got)
				assert.True(t, client.IgnoreNotFound(getErr) == nil && getErr != nil, "expected %q to be deleted", name)
			}
		})
	}
}

func TestPingSourceCreateOrUpdate(t *testing.T) {
	tests := []struct {
		name         string
		preCreate    bool
		preData      string
		data         string
		expectedData string
	}{
		{
			name:         "creates PingSource when not found",
			data:         "data",
			expectedData: "data",
		},
		{
			name:         "updates PingSource when spec differs",
			preCreate:    true,
			preData:      "old-data",
			data:         "new-data",
			expectedData: "new-data",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			fakeClient := fake.NewClientBuilder().WithScheme(newPingSourceScheme()).Build()
			pm := newPingSourceManager(fakeClient)
			capp := newCapp(cappName, cappNamespace, nil)

			if tt.preCreate {
				src := cappv1alpha1.SourceConfiguration{
					Name: sourceName,
					PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{
						Schedule: schedule,
						Data:     tt.preData,
					},
				}
				assert.NoError(t, pm.createOrUpdate(capp, src))
			}

			src := cappv1alpha1.SourceConfiguration{
				Name: sourceName,
				PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{
					Schedule: schedule,
					Data:     tt.data,
				},
			}
			assert.NoError(t, pm.createOrUpdate(capp, src))
			got := &sourcesv1.PingSource{}
			assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: pingSourceName(sourceName), Namespace: cappNamespace}, got))
			assert.Equal(t, tt.expectedData, got.Spec.Data)
			assert.Len(t, got.OwnerReferences, 1)
			assert.Equal(t, capp.Name, got.OwnerReferences[0].Name)
		})
	}
}

func TestPingSourceGetStatus(t *testing.T) {
	tests := []struct {
		name          string
		preCreate     []*sourcesv1.PingSource
		expectedNames []string
	}{
		{
			name: "returns statuses sorted by name",
			preCreate: []*sourcesv1.PingSource{
				newPingSource(sourceC),
				newPingSource(sourceA),
				newPingSource(sourceB),
			},
			expectedNames: []string{pingSourceName(sourceA), pingSourceName(sourceB), pingSourceName(sourceC)},
		},
		{
			name:          "returns unknown readiness when no condition present",
			preCreate:     []*sourcesv1.PingSource{newPingSource(sourceName)},
			expectedNames: []string{pingSourceName(sourceName)},
		},
		{
			name:          "returns nil EventSources when no PingSources exist",
			preCreate:     []*sourcesv1.PingSource{},
			expectedNames: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			fakeClient := fake.NewClientBuilder().WithScheme(newPingSourceScheme()).Build()
			for _, ps := range tt.preCreate {
				assert.NoError(t, fakeClient.Create(ctx, ps))
			}
			pm := newPingSourceManager(fakeClient)
			capp := newCapp(cappName, cappNamespace, nil)
			result, err := pm.GetStatus(capp)
			assert.NoError(t, err)
			if tt.expectedNames == nil {
				assert.Nil(t, result.EventSources)
			} else {
				assert.Len(t, result.EventSources, len(tt.expectedNames))
				for i, name := range tt.expectedNames {
					assert.Equal(t, name, result.EventSources[i].Name)
				}
			}
			if len(tt.preCreate) == 1 {
				assert.Equal(t, corev1.ConditionUnknown, result.EventSources[0].Condition.Status)
				assert.Equal(t, "Source readiness not known", result.EventSources[0].Condition.Message)
			}
			_ = ctx
		})
	}
}
