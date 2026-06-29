package resourcemanagers

import (
	"context"
	"fmt"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/events"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	sourceName = "ping"
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

func TestPingSourceManagerCleanUpOrphans(t *testing.T) {
	t.Run("deletes orphaned PingSource not in spec", func(t *testing.T) {
		ctx := context.Background()
		fakeClient := newFakeClient(newPingSourceScheme())
		for _, source := range []string{sourceA, sourceB} {
			require.NoError(t, fakeClient.Create(ctx, newPingSource(source)))
		}

		pingCfg := cappv1alpha1.PingSourceConfiguration{Schedule: schedule}
		capp := newBaseCapp()
		capp.Spec.EventSourcesSpec.Sources = []cappv1alpha1.SourceConfiguration{
			newPingSourceEntry(sourceA, pingCfg),
		}
		require.NoError(t, newPingSourceManager(fakeClient).cleanUpOrphans(ctx, capp))

		got := &sourcesv1.PingSource{}
		require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{
			Name: fmt.Sprintf("%s-%s", cappName, sourceA), Namespace: cappNamespace,
		}, got))

		deleted := &sourcesv1.PingSource{}
		getErr := fakeClient.Get(ctx, types.NamespacedName{
			Name: fmt.Sprintf("%s-%s", cappName, sourceB), Namespace: cappNamespace,
		}, deleted)
		require.True(t, errors.IsNotFound(getErr), "expected orphan to not exist")
	})
}

func TestPingSourceManagerCreateOrUpdate(t *testing.T) {
	tests := []struct {
		name         string
		preCreate    bool
		preData      string
		data         string
		expectedData string
	}{
		{
			name:         "creates when not found",
			data:         "data",
			expectedData: "data",
		},
		{
			name:         "updates when spec differs",
			preCreate:    true,
			preData:      "old-data",
			data:         "new-data",
			expectedData: "new-data",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			fakeClient := newFakeClient(newPingSourceScheme())
			pm := newPingSourceManager(fakeClient)
			capp := newBaseCapp()

			if tt.preCreate {
				cfg := cappv1alpha1.PingSourceConfiguration{Schedule: schedule}
				cfg.Data = tt.preData
				require.NoError(t, pm.createOrUpdate(ctx, capp, newPingSourceEntry(sourceName, cfg)))
			}

			cfg := cappv1alpha1.PingSourceConfiguration{Schedule: schedule}
			cfg.Data = tt.data
			src := newPingSourceEntry(sourceName, cfg)
			require.NoError(t, pm.createOrUpdate(ctx, capp, src))
			got := &sourcesv1.PingSource{}
			require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-%s", cappName, sourceName), Namespace: cappNamespace}, got))
			require.Equal(t, tt.expectedData, got.Spec.Data)
			require.Len(t, got.OwnerReferences, 1)
			require.Equal(t, capp.Name, got.OwnerReferences[0].Name)
		})
	}
}

func TestPingSourceManagerManage(t *testing.T) {
	ctx := context.Background()

	t.Run("reconciles when required", func(t *testing.T) {
		pm := newPingSourceManager(newFakeClient(newPingSourceScheme()))
		capp := newBaseCapp()
		capp.Spec.EventSourcesSpec.Sources = []cappv1alpha1.SourceConfiguration{
			newPingSourceEntry(sourceA, cappv1alpha1.PingSourceConfiguration{Schedule: schedule}),
		}
		require.NoError(t, pm.Manage(ctx, capp))
	})

	t.Run("cleans up when not required", func(t *testing.T) {
		fakeClient := newFakeClient(newPingSourceScheme())
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
		require.True(t, errors.IsNotFound(getErr), "expected %q to not exist", fmt.Sprintf("%s-%s", cappName, ordersA))
	})
}
