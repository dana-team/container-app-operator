package resourcemanagers

import (
	"context"
	"fmt"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	sourceName = "ping"
	schedule   = "* * * * *"
	sourceA    = "ping-a"
	sourceB    = "ping-b"
	sourceC    = "ping-c"
)

func newPingSourceScheme() *runtime.Scheme {
	s := newScheme()
	utilruntime.Must(sourcesv1.AddToScheme(s))
	utilruntime.Must(servingv1.AddToScheme(s))
	return s
}

func newPingSourceManager(k8sClient client.Client) PingSourceManager {
	return PingSourceManager{
		ResourceManagerClient: rclient.ResourceManagerClient{K8sclient: k8sClient, Log: logr.Discard()},
		EventRecorder:         events.NewFakeRecorder(10),
	}
}

func newPingSource(source string) *sourcesv1.PingSource {
	return &sourcesv1.PingSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", cappName, source),
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
			expectKept:    []string{fmt.Sprintf("%s-%s", cappName, sourceA)},
			expectDeleted: []string{fmt.Sprintf("%s-%s", cappName, sourceB)},
		},
		{
			name: "keeps all owned when all are in spec",
			sources: []cappv1alpha1.SourceConfiguration{
				{Name: sourceA, PingSourceConfiguration: pingCfg},
				{Name: sourceB, PingSourceConfiguration: pingCfg},
			},
			preCreate:  []*sourcesv1.PingSource{newPingSource(sourceA), newPingSource(sourceB)},
			expectKept: []string{fmt.Sprintf("%s-%s", cappName, sourceA), fmt.Sprintf("%s-%s", cappName, sourceB)},
		},
		{
			name:          "deletes all owned when none match spec",
			sources:       []cappv1alpha1.SourceConfiguration{{Name: sourceA, PingSourceConfiguration: pingCfg}},
			preCreate:     []*sourcesv1.PingSource{newPingSource(sourceB), newPingSource(sourceC)},
			expectDeleted: []string{fmt.Sprintf("%s-%s", cappName, sourceB), fmt.Sprintf("%s-%s", cappName, sourceC)},
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
			capp := newBaseCapp()
			capp.Spec.EventSourcesSpec.Sources = tt.sources
			assert.NoError(t, pm.cleanUpOrphans(ctx, capp))
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
			capp := newBaseCapp()

			if tt.preCreate {
				src := cappv1alpha1.SourceConfiguration{
					Name: sourceName,
					PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{
						Schedule: schedule,
						Data:     tt.preData,
					},
				}
				assert.NoError(t, pm.createOrUpdate(ctx, capp, src))
			}

			src := cappv1alpha1.SourceConfiguration{
				Name: sourceName,
				PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{
					Schedule: schedule,
					Data:     tt.data,
				},
			}
			assert.NoError(t, pm.createOrUpdate(ctx, capp, src))
			got := &sourcesv1.PingSource{}
			assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-%s", cappName, sourceName), Namespace: cappNamespace}, got))
			assert.Equal(t, tt.expectedData, got.Spec.Data)
			assert.Len(t, got.OwnerReferences, 1)
			assert.Equal(t, capp.Name, got.OwnerReferences[0].Name)
		})
	}
}

func TestPingSourceManage(t *testing.T) {
	ctx := context.Background()
	pingCfg := &cappv1alpha1.PingSourceConfiguration{Schedule: schedule}

	t.Run("reconciles when ping is required", func(t *testing.T) {
		pm := newPingSourceManager(fake.NewClientBuilder().WithScheme(newPingSourceScheme()).Build())
		capp := newBaseCapp()
		capp.Spec.EventSourcesSpec.Sources = []cappv1alpha1.SourceConfiguration{
			{Name: sourceA, PingSourceConfiguration: pingCfg},
		}
		require.NoError(t, pm.Manage(ctx, capp))
	})

	t.Run("cleans up when ping is not required", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newPingSourceScheme()).Build()
		require.NoError(t, fakeClient.Create(ctx, newPingSource(ordersA)))

		pm := newPingSourceManager(fakeClient)
		capp := newBaseCapp()
		capp.Spec.EventSourcesSpec.Sources = []cappv1alpha1.SourceConfiguration{
			newKafkaSourceEntry(ordersA, newKafkaSourceConfiguration()),
		}
		require.NoError(t, pm.Manage(ctx, capp))

		got := &sourcesv1.PingSource{}
		getErr := fakeClient.Get(ctx, types.NamespacedName{
			Name: fmt.Sprintf("%s-%s", cappName, ordersA), Namespace: cappNamespace,
		}, got)
		require.True(t, client.IgnoreNotFound(getErr) == nil && getErr != nil, "expected %q to not exist", fmt.Sprintf("%s-%s", cappName, ordersA))
	})
}

func TestPingSourceCleanUp(t *testing.T) {
	t.Run("deletes all owned PingSources", func(t *testing.T) {
		ctx := context.Background()
		fakeClient := fake.NewClientBuilder().WithScheme(newPingSourceScheme()).Build()
		for _, source := range []string{sourceA, sourceB} {
			require.NoError(t, fakeClient.Create(ctx, newPingSource(source)))
		}

		require.NoError(t, newPingSourceManager(fakeClient).CleanUp(ctx, newBaseCapp()))

		for _, source := range []string{sourceA, sourceB} {
			got := &sourcesv1.PingSource{}
			getErr := fakeClient.Get(ctx, types.NamespacedName{
				Name: fmt.Sprintf("%s-%s", cappName, source), Namespace: cappNamespace,
			}, got)
			require.True(t, client.IgnoreNotFound(getErr) == nil && getErr != nil, "expected %q to not exist", fmt.Sprintf("%s-%s", cappName, source))
		}
	})
}
