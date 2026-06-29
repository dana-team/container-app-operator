package resourcemanagers

import (
	"context"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"github.com/go-logr/logr"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/syslogng/output"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func newSyslogNGOutputManager(k8sClient client.Client) SyslogNGOutputManager {
	return SyslogNGOutputManager{
		ResourceManagerClient: rclient.ResourceManagerClient{K8sclient: k8sClient, Log: logr.Discard()},
		EventRecorder:         events.NewFakeRecorder(10),
	}
}

func newSyslogNGOutput() *loggingv1beta1.SyslogNGOutput {
	return &loggingv1beta1.SyslogNGOutput{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cappName,
			Namespace: cappNamespace,
			Labels:    utils.ManagedResourceLabels(cappName),
		},
		Spec: loggingv1beta1.SyslogNGOutputSpec{
			Elasticsearch: &output.ElasticsearchOutput{
				Index: elasticIndex,
				HTTPOutput: output.HTTPOutput{
					URL: elasticHost,
				},
			},
		},
	}
}

func TestSyslogNGOutputManagerCreateOrUpdate(t *testing.T) {
	ctx := context.Background()
	key := types.NamespacedName{Name: cappName, Namespace: cappNamespace}

	t.Run("creates when not found", func(t *testing.T) {
		om := newSyslogNGOutputManager(newFakeClient(newSyslogNGScheme()))
		capp := newBaseCapp()
		capp.Spec.LogSpec = newLogSpec(cappv1alpha1.LogTypeElastic)

		require.NoError(t, om.createOrUpdate(ctx, capp))

		got := &loggingv1beta1.SyslogNGOutput{}
		require.NoError(t, om.K8sclient.Get(ctx, key, got))
		require.Equal(t, elasticIndex, got.Spec.Elasticsearch.Index)
		require.Equal(t, elasticHost, got.Spec.Elasticsearch.URL)
		require.Equal(t, cappName, got.OwnerReferences[0].Name)
	})

	t.Run("updates when spec differs", func(t *testing.T) {
		const updatedIndex = "my-index-v2"

		om := newSyslogNGOutputManager(newFakeClient(newSyslogNGScheme()))
		require.NoError(t, om.K8sclient.Create(ctx, newSyslogNGOutput()))

		spec := newLogSpec(cappv1alpha1.LogTypeElastic)
		spec.Index = updatedIndex
		capp := newBaseCapp()
		capp.Spec.LogSpec = spec
		require.NoError(t, om.createOrUpdate(ctx, capp))

		got := &loggingv1beta1.SyslogNGOutput{}
		require.NoError(t, om.K8sclient.Get(ctx, key, got))
		require.Equal(t, updatedIndex, got.Spec.Elasticsearch.Index)
	})

	t.Run("creates datastream output when log type is elastic-datastream", func(t *testing.T) {
		om := newSyslogNGOutputManager(newFakeClient(newSyslogNGScheme()))
		capp := newBaseCapp()
		capp.Spec.LogSpec = newLogSpec(cappv1alpha1.LogTypeElasticDataStream)
		require.NoError(t, om.createOrUpdate(ctx, capp))

		got := &loggingv1beta1.SyslogNGOutput{}
		require.NoError(t, om.K8sclient.Get(ctx, key, got))
		require.Nil(t, got.Spec.Elasticsearch)
		require.NotNil(t, got.Spec.ElasticsearchDatastream)
		require.Equal(t, elasticHost, got.Spec.ElasticsearchDatastream.URL)
		require.Equal(t, cappName, got.OwnerReferences[0].Name)
	})
}

func TestSyslogNGOutputManagerManage(t *testing.T) {
	ctx := context.Background()

	t.Run("reconciles when required", func(t *testing.T) {
		om := newSyslogNGOutputManager(newFakeClient(newSyslogNGScheme()))
		capp := newBaseCapp()
		capp.Spec.LogSpec = newLogSpec(cappv1alpha1.LogTypeElastic)
		require.NoError(t, om.Manage(ctx, capp))
	})

	t.Run("cleans up when not required", func(t *testing.T) {
		fakeClient := newFakeClient(newSyslogNGScheme())
		require.NoError(t, fakeClient.Create(ctx, newSyslogNGOutput()))

		om := newSyslogNGOutputManager(fakeClient)
		require.NoError(t, om.Manage(ctx, newBaseCapp()))

		got := &loggingv1beta1.SyslogNGOutput{}
		getErr := fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got)
		require.Error(t, getErr)
		require.True(t, errors.IsNotFound(getErr))
	})

	t.Run("cleans up when log type is unsupported", func(t *testing.T) {
		fakeClient := newFakeClient(newSyslogNGScheme())
		require.NoError(t, fakeClient.Create(ctx, newSyslogNGOutput()))

		om := newSyslogNGOutputManager(fakeClient)
		capp := newBaseCapp()
		capp.Spec.LogSpec = cappv1alpha1.LogSpec{
			Type: cappv1alpha1.LogType(unsupportedLogType),
			Host: elasticHost,
		}
		require.NoError(t, om.Manage(ctx, capp))

		got := &loggingv1beta1.SyslogNGOutput{}
		getErr := fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got)
		require.Error(t, getErr)
		require.True(t, errors.IsNotFound(getErr))
	})
}

func TestSyslogNGOutputManagerCleanUp(t *testing.T) {
	ctx := context.Background()

	t.Run("succeeds when none exist", func(t *testing.T) {
		om := newSyslogNGOutputManager(newFakeClient(newSyslogNGScheme()))
		require.NoError(t, om.CleanUp(ctx, newBaseCapp()))
	})

	t.Run("skips delete when deleting and has owner reference", func(t *testing.T) {
		capp := cappWithDeletionTimestamp(newBaseCapp())

		syslogOutput := newSyslogNGOutput()
		require.NoError(t, controllerutil.SetOwnerReference(&capp, syslogOutput, newSyslogNGScheme()))

		om := newSyslogNGOutputManager(newFakeClient(newSyslogNGScheme(), syslogOutput))
		require.NoError(t, om.CleanUp(ctx, capp))

		got := &loggingv1beta1.SyslogNGOutput{}
		require.NoError(t, om.K8sclient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got))
	})

	t.Run("deletes when deleting and lacks owner reference", func(t *testing.T) {
		capp := cappWithDeletionTimestamp(newBaseCapp())

		syslogOutput := newSyslogNGOutput()
		om := newSyslogNGOutputManager(newFakeClient(newSyslogNGScheme(), syslogOutput))
		require.NoError(t, om.CleanUp(ctx, capp))

		got := &loggingv1beta1.SyslogNGOutput{}
		getErr := om.K8sclient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got)
		require.True(t, errors.IsNotFound(getErr))
	})
}
