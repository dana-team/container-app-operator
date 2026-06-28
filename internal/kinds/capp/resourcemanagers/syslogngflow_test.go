package resourcemanagers

import (
	"context"
	"fmt"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"github.com/go-logr/logr"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/syslogng/filter"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func newSyslogNGFlowManager(k8sClient client.Client) SyslogNGFlowManager {
	return SyslogNGFlowManager{
		ResourceManagerClient: rclient.ResourceManagerClient{K8sclient: k8sClient, Log: logr.Discard()},
		EventRecorder:         events.NewFakeRecorder(10),
	}
}

func newSyslogNGFlow(outputRefs ...string) *loggingv1beta1.SyslogNGFlow {
	if len(outputRefs) == 0 {
		outputRefs = []string{cappName}
	}
	return &loggingv1beta1.SyslogNGFlow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cappName,
			Namespace: cappNamespace,
			Labels:    utils.ManagedResourceLabels(cappName),
		},
		Spec: loggingv1beta1.SyslogNGFlowSpec{
			Match: &loggingv1beta1.SyslogNGMatch{
				Regexp: &filter.RegexpMatchExpr{
					Pattern: cappName,
					Type:    "string",
					Value:   fmt.Sprintf("json#kubernetes#labels#%s", knativeConfiguration),
				},
			},
			LocalOutputRefs: outputRefs,
		},
	}
}

func TestSyslogNGFlowManagerCreateOrUpdate(t *testing.T) {
	ctx := context.Background()
	key := types.NamespacedName{Name: cappName, Namespace: cappNamespace}

	t.Run("creates when not found", func(t *testing.T) {
		fm := newSyslogNGFlowManager(newFakeClient(newSyslogNGScheme()))
		capp := newBaseCapp()
		capp.Spec.LogSpec = newLogSpec(cappv1alpha1.LogTypeElastic)

		require.NoError(t, fm.createOrUpdate(ctx, capp))

		got := &loggingv1beta1.SyslogNGFlow{}
		require.NoError(t, fm.K8sclient.Get(ctx, key, got))
		require.Equal(t, cappName, got.Spec.Match.Regexp.Pattern)
		require.Equal(t, []string{cappName}, got.Spec.LocalOutputRefs)
		require.Equal(t, cappName, got.OwnerReferences[0].Name)
	})

	t.Run("updates when spec differs", func(t *testing.T) {
		fm := newSyslogNGFlowManager(newFakeClient(newSyslogNGScheme()))
		require.NoError(t, fm.K8sclient.Create(ctx, newSyslogNGFlow("stale-output")))

		capp := newBaseCapp()
		capp.Spec.LogSpec = newLogSpec(cappv1alpha1.LogTypeElastic)
		require.NoError(t, fm.createOrUpdate(ctx, capp))

		got := &loggingv1beta1.SyslogNGFlow{}
		require.NoError(t, fm.K8sclient.Get(ctx, key, got))
		require.Equal(t, []string{cappName}, got.Spec.LocalOutputRefs)
	})
}

func TestSyslogNGFlowManagerManage(t *testing.T) {
	ctx := context.Background()

	t.Run("reconciles when required", func(t *testing.T) {
		fm := newSyslogNGFlowManager(newFakeClient(newSyslogNGScheme()))
		capp := newBaseCapp()
		capp.Spec.LogSpec = newLogSpec(cappv1alpha1.LogTypeElastic)
		require.NoError(t, fm.Manage(ctx, capp))
	})

	t.Run("cleans up when not required", func(t *testing.T) {
		fakeClient := newFakeClient(newSyslogNGScheme())
		require.NoError(t, fakeClient.Create(ctx, newSyslogNGFlow()))

		fm := newSyslogNGFlowManager(fakeClient)
		require.NoError(t, fm.Manage(ctx, newBaseCapp()))

		got := &loggingv1beta1.SyslogNGFlow{}
		getErr := fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got)
		require.Error(t, getErr)
		require.True(t, errors.IsNotFound(getErr))
	})

	t.Run("cleans up when log type is unsupported", func(t *testing.T) {
		fakeClient := newFakeClient(newSyslogNGScheme())
		require.NoError(t, fakeClient.Create(ctx, newSyslogNGFlow()))

		fm := newSyslogNGFlowManager(fakeClient)
		capp := newBaseCapp()
		capp.Spec.LogSpec = cappv1alpha1.LogSpec{
			Type: cappv1alpha1.LogType(unsupportedLogType),
			Host: elasticHost,
		}
		require.NoError(t, fm.Manage(ctx, capp))

		got := &loggingv1beta1.SyslogNGFlow{}
		getErr := fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got)
		require.Error(t, getErr)
		require.True(t, errors.IsNotFound(getErr))
	})
}

func TestSyslogNGFlowManagerCleanUp(t *testing.T) {
	ctx := context.Background()

	t.Run("succeeds when none exist", func(t *testing.T) {
		fm := newSyslogNGFlowManager(newFakeClient(newSyslogNGScheme()))
		require.NoError(t, fm.CleanUp(ctx, newBaseCapp()))
	})

	t.Run("deletes SyslogNGFlow by capp name", func(t *testing.T) {
		fakeClient := newFakeClient(newSyslogNGScheme())
		require.NoError(t, fakeClient.Create(ctx, newSyslogNGFlow()))

		require.NoError(t, newSyslogNGFlowManager(fakeClient).CleanUp(ctx, newBaseCapp()))

		got := &loggingv1beta1.SyslogNGFlow{}
		getErr := fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got)
		require.Error(t, getErr)
		require.True(t, errors.IsNotFound(getErr))
	})

	t.Run("skips delete when deleting and has owner reference", func(t *testing.T) {
		capp := cappWithDeletionTimestamp(newBaseCapp())

		flow := newSyslogNGFlow()
		require.NoError(t, controllerutil.SetOwnerReference(&capp, flow, newSyslogNGScheme()))

		fm := newSyslogNGFlowManager(newFakeClient(newSyslogNGScheme(), flow))
		require.NoError(t, fm.CleanUp(ctx, capp))

		got := &loggingv1beta1.SyslogNGFlow{}
		require.NoError(t, fm.K8sclient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got))
	})
}
