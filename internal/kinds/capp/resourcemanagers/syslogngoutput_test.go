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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	elasticHost           = "https://elastic.example:9200/_bulk"
	elasticIndex          = "my-index"
	elasticIndexUpdated   = "my-index-v2"
	elasticUser           = "elastic-user"
	elasticPasswordSecret = "elastic-creds"
	unsupportedLogType    = "splunk"
)

func newSyslogNGOutputScheme() *runtime.Scheme {
	s := newScheme()
	utilruntime.Must(loggingv1beta1.AddToScheme(s))
	return s
}

func newSyslogNGOutputManager(k8sClient client.Client) SyslogNGOutputManager {
	return SyslogNGOutputManager{
		ResourceManagerClient: rclient.ResourceManagerClient{K8sclient: k8sClient, Log: logr.Discard()},
		EventRecorder:         events.NewFakeRecorder(10),
	}
}

func newLogSpec(logType cappv1alpha1.LogType) cappv1alpha1.LogSpec {
	spec := cappv1alpha1.LogSpec{
		Type:           logType,
		Host:           elasticHost,
		User:           elasticUser,
		PasswordSecret: elasticPasswordSecret,
	}
	if logType == cappv1alpha1.LogTypeElastic {
		spec.Index = elasticIndex
	}
	return spec
}

func newSyslogNGOutputCapp(logSpec cappv1alpha1.LogSpec) cappv1alpha1.Capp {
	capp := newBaseCapp()
	capp.Spec.LogSpec = logSpec
	return capp
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

func TestSyslogNGOutputCreateOrUpdate(t *testing.T) {
	ctx := context.Background()
	key := types.NamespacedName{Name: cappName, Namespace: cappNamespace}

	t.Run("creates when not found", func(t *testing.T) {
		om := newSyslogNGOutputManager(fake.NewClientBuilder().WithScheme(newSyslogNGOutputScheme()).Build())
		capp := newSyslogNGOutputCapp(newLogSpec(cappv1alpha1.LogTypeElastic))

		require.NoError(t, om.createOrUpdate(ctx, capp))

		got := &loggingv1beta1.SyslogNGOutput{}
		require.NoError(t, om.K8sclient.Get(ctx, key, got))
		require.Equal(t, elasticIndex, got.Spec.Elasticsearch.Index)
		require.Equal(t, elasticHost, got.Spec.Elasticsearch.URL)
		require.Equal(t, cappName, got.OwnerReferences[0].Name)
	})

	t.Run("updates when spec differs", func(t *testing.T) {
		om := newSyslogNGOutputManager(fake.NewClientBuilder().WithScheme(newSyslogNGOutputScheme()).Build())
		require.NoError(t, om.K8sclient.Create(ctx, newSyslogNGOutput()))

		spec := newLogSpec(cappv1alpha1.LogTypeElastic)
		spec.Index = elasticIndexUpdated
		require.NoError(t, om.createOrUpdate(ctx, newSyslogNGOutputCapp(spec)))

		got := &loggingv1beta1.SyslogNGOutput{}
		require.NoError(t, om.K8sclient.Get(ctx, key, got))
		require.Equal(t, elasticIndexUpdated, got.Spec.Elasticsearch.Index)
	})

	t.Run("creates datastream output when log type is elastic-datastream", func(t *testing.T) {
		om := newSyslogNGOutputManager(fake.NewClientBuilder().WithScheme(newSyslogNGOutputScheme()).Build())
		require.NoError(t, om.createOrUpdate(ctx, newSyslogNGOutputCapp(newLogSpec(cappv1alpha1.LogTypeElasticDataStream))))

		got := &loggingv1beta1.SyslogNGOutput{}
		require.NoError(t, om.K8sclient.Get(ctx, key, got))
		require.Nil(t, got.Spec.Elasticsearch)
		require.NotNil(t, got.Spec.ElasticsearchDatastream)
		require.Equal(t, elasticHost, got.Spec.ElasticsearchDatastream.URL)
		require.Equal(t, cappName, got.OwnerReferences[0].Name)
	})
}

func TestSyslogNGOutputManage(t *testing.T) {
	ctx := context.Background()

	t.Run("reconciles when log is required", func(t *testing.T) {
		om := newSyslogNGOutputManager(fake.NewClientBuilder().WithScheme(newSyslogNGOutputScheme()).Build())
		require.NoError(t, om.Manage(ctx, newSyslogNGOutputCapp(newLogSpec(cappv1alpha1.LogTypeElastic))))
	})

	t.Run("cleans up when log is not required", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newSyslogNGOutputScheme()).Build()
		require.NoError(t, fakeClient.Create(ctx, newSyslogNGOutput()))

		om := newSyslogNGOutputManager(fakeClient)
		require.NoError(t, om.Manage(ctx, newBaseCapp()))

		got := &loggingv1beta1.SyslogNGOutput{}
		getErr := fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got)
		require.True(t, client.IgnoreNotFound(getErr) == nil && getErr != nil,
			"expected %q to not exist", cappName)
	})

	t.Run("cleans up when log type is unsupported", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newSyslogNGOutputScheme()).Build()
		require.NoError(t, fakeClient.Create(ctx, newSyslogNGOutput()))

		om := newSyslogNGOutputManager(fakeClient)
		capp := newSyslogNGOutputCapp(cappv1alpha1.LogSpec{
			Type: cappv1alpha1.LogType(unsupportedLogType),
			Host: elasticHost,
		})
		require.NoError(t, om.Manage(ctx, capp))

		got := &loggingv1beta1.SyslogNGOutput{}
		getErr := fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got)
		require.True(t, client.IgnoreNotFound(getErr) == nil && getErr != nil,
			"expected %q to not exist", cappName)
	})
}

func TestSyslogNGOutputCleanUp(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes SyslogNGOutput by capp name", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newSyslogNGOutputScheme()).Build()
		require.NoError(t, fakeClient.Create(ctx, newSyslogNGOutput()))

		require.NoError(t, newSyslogNGOutputManager(fakeClient).CleanUp(ctx, newBaseCapp()))

		got := &loggingv1beta1.SyslogNGOutput{}
		getErr := fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got)
		require.True(t, client.IgnoreNotFound(getErr) == nil && getErr != nil,
			"expected %q to not exist", cappName)
	})

	t.Run("skips delete when capp is deleting and output has owner reference", func(t *testing.T) {
		capp := newBaseCapp()
		now := metav1.Now()
		capp.DeletionTimestamp = &now

		syslogOutput := newSyslogNGOutput()
		require.NoError(t, controllerutil.SetOwnerReference(&capp, syslogOutput, newSyslogNGOutputScheme()))

		om := newSyslogNGOutputManager(fake.NewClientBuilder().WithScheme(newSyslogNGOutputScheme()).WithObjects(syslogOutput).Build())
		require.NoError(t, om.CleanUp(ctx, capp))

		got := &loggingv1beta1.SyslogNGOutput{}
		require.NoError(t, om.K8sclient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got))
	})
}
