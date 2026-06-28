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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/events"
	kafkasourcev1 "knative.dev/eventing-kafka-broker/control-plane/pkg/apis/sources/v1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func newKafkaSourceScheme() *runtime.Scheme {
	s := newScheme()
	utilruntime.Must(kafkasourcev1.AddToScheme(s))
	utilruntime.Must(servingv1.AddToScheme(s))
	return s
}

func newKafkaSourceManager(k8sClient client.Client) KafkaSourceManager {
	return KafkaSourceManager{
		ResourceManagerClient: rclient.ResourceManagerClient{K8sclient: k8sClient, Log: logr.Discard()},
		EventRecorder:         events.NewFakeRecorder(10),
	}
}

func newKafkaSource(source string) *kafkasourcev1.KafkaSource {
	return &kafkasourcev1.KafkaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", cappName, source),
			Namespace: cappNamespace,
			Labels:    utils.ManagedResourceLabels(cappName),
		},
	}
}

func TestKafkaSourceManagerCreateOrUpdate(t *testing.T) {
	ctx := context.Background()
	key := types.NamespacedName{Name: fmt.Sprintf("%s-%s", cappName, ordersSource), Namespace: cappNamespace}

	t.Run("creates when not found", func(t *testing.T) {
		km := newKafkaSourceManager(newFakeClient(newKafkaSourceScheme()))
		capp := newBaseCapp()
		cfg := newKafkaSourceConfiguration()

		require.NoError(t, km.createOrUpdate(ctx, capp, newKafkaSourceEntry(ordersSource, cfg)))

		got := &kafkasourcev1.KafkaSource{}
		require.NoError(t, km.K8sclient.Get(ctx, key, got))
		require.Equal(t, []string{topicOrders, topicPayments}, got.Spec.Topics)
		require.Equal(t, fmt.Sprintf("%s-%s", cappName, ordersSource), got.Spec.ConsumerGroup)
		require.Equal(t, cappName, got.OwnerReferences[0].Name)
	})

	t.Run("updates when spec differs", func(t *testing.T) {
		km := newKafkaSourceManager(newFakeClient(newKafkaSourceScheme()))
		capp := newBaseCapp()
		existing := newKafkaSource(ordersSource)
		existing.Spec.Topics = []string{topicOrders}
		require.NoError(t, km.K8sclient.Create(ctx, existing))

		cfg := newKafkaSourceConfiguration()
		require.NoError(t, km.createOrUpdate(ctx, capp, newKafkaSourceEntry(ordersSource, cfg)))

		got := &kafkasourcev1.KafkaSource{}
		require.NoError(t, km.K8sclient.Get(ctx, key, got))
		require.Equal(t, []string{topicOrders, topicPayments}, got.Spec.Topics)
	})

	t.Run("pauses consumption when Capp is disabled", func(t *testing.T) {
		km := newKafkaSourceManager(newFakeClient(newKafkaSourceScheme()))
		capp := newBaseCapp()
		capp.Spec.State = cappv1alpha1.CappStateDisabled
		cfg := newKafkaSourceConfiguration()
		cfg.Topics = []string{topicOrders}

		require.NoError(t, km.createOrUpdate(ctx, capp, newKafkaSourceEntry(ordersSource, cfg)))

		got := &kafkasourcev1.KafkaSource{}
		require.NoError(t, km.K8sclient.Get(ctx, key, got))
		require.NotNil(t, got.Spec.Consumers)
		require.Equal(t, int32(0), *got.Spec.Consumers)
	})

	t.Run("preserves consumer group on update", func(t *testing.T) {
		km := newKafkaSourceManager(newFakeClient(newKafkaSourceScheme()))
		capp := newBaseCapp()
		existing := newKafkaSource(ordersSource)
		existing.Spec.ConsumerGroup = "immutable-group"
		require.NoError(t, km.K8sclient.Create(ctx, existing))

		cfg := newKafkaSourceConfiguration()
		cfg.Topics = []string{topicOrders}
		cfg.ConsumerGroup = "new-group-from-capp"
		require.NoError(t, km.createOrUpdate(ctx, capp, newKafkaSourceEntry(ordersSource, cfg)))

		got := &kafkasourcev1.KafkaSource{}
		require.NoError(t, km.K8sclient.Get(ctx, key, got))
		require.Equal(t, []string{topicOrders}, got.Spec.Topics)
		require.Equal(t, "immutable-group", got.Spec.ConsumerGroup)
	})
}

func TestKafkaSourceManagerCleanUpOrphans(t *testing.T) {
	t.Run("deletes orphan not in spec", func(t *testing.T) {
		ctx := context.Background()
		fakeClient := newFakeClient(newKafkaSourceScheme())
		for _, source := range []string{ordersA, ordersB} {
			require.NoError(t, fakeClient.Create(ctx, newKafkaSource(source)))
		}

		capp := newBaseCapp()
		capp.Spec.EventSourcesSpec.Sources = []cappv1alpha1.SourceConfiguration{
			newKafkaSourceEntry(ordersA, newKafkaSourceConfiguration()),
		}
		require.NoError(t, newKafkaSourceManager(fakeClient).cleanUpOrphans(ctx, capp))

		got := &kafkasourcev1.KafkaSource{}
		require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{
			Name: fmt.Sprintf("%s-%s", cappName, ordersA), Namespace: cappNamespace,
		}, got))

		deleted := &kafkasourcev1.KafkaSource{}
		getErr := fakeClient.Get(ctx, types.NamespacedName{
			Name: fmt.Sprintf("%s-%s", cappName, ordersB), Namespace: cappNamespace,
		}, deleted)
		require.True(t, client.IgnoreNotFound(getErr) == nil && getErr != nil, "expected orphan to not exist")
	})
}

func TestKafkaSourceManagerManage(t *testing.T) {
	ctx := context.Background()
	kafkaCfg := newKafkaSourceConfiguration()

	t.Run("reconciles when required", func(t *testing.T) {
		km := newKafkaSourceManager(newFakeClient(newKafkaSourceScheme()))
		capp := newBaseCapp()
		capp.Spec.EventSourcesSpec.Sources = []cappv1alpha1.SourceConfiguration{newKafkaSourceEntry(ordersA, kafkaCfg)}
		require.NoError(t, km.Manage(ctx, capp))
	})

	t.Run("cleans up when not required", func(t *testing.T) {
		fakeClient := newFakeClient(newKafkaSourceScheme())
		require.NoError(t, fakeClient.Create(ctx, newKafkaSource(ordersA)))

		km := newKafkaSourceManager(fakeClient)
		capp := newBaseCapp()
		capp.Spec.EventSourcesSpec.Sources = []cappv1alpha1.SourceConfiguration{
			newPingSourceEntry(ordersA, cappv1alpha1.PingSourceConfiguration{Schedule: schedule}),
		}
		require.NoError(t, km.Manage(ctx, capp))

		got := &kafkasourcev1.KafkaSource{}
		getErr := fakeClient.Get(ctx, types.NamespacedName{
			Name: fmt.Sprintf("%s-%s", cappName, ordersA), Namespace: cappNamespace,
		}, got)
		require.True(t, client.IgnoreNotFound(getErr) == nil && getErr != nil, "expected %q to not exist", fmt.Sprintf("%s-%s", cappName, ordersA))
	})
}

func TestKafkaSourceManagerCleanUp(t *testing.T) {
	t.Run("deletes all owned resources", func(t *testing.T) {
		ctx := context.Background()
		fakeClient := newFakeClient(newKafkaSourceScheme())
		for _, source := range []string{ordersA, ordersB} {
			require.NoError(t, fakeClient.Create(ctx, newKafkaSource(source)))
		}

		require.NoError(t, newKafkaSourceManager(fakeClient).CleanUp(ctx, newBaseCapp()))

		for _, source := range []string{ordersA, ordersB} {
			got := &kafkasourcev1.KafkaSource{}
			getErr := fakeClient.Get(ctx, types.NamespacedName{
				Name: fmt.Sprintf("%s-%s", cappName, source), Namespace: cappNamespace,
			}, got)
			require.True(t, client.IgnoreNotFound(getErr) == nil && getErr != nil, "expected %q to not exist", fmt.Sprintf("%s-%s", cappName, source))
		}
	})
}
