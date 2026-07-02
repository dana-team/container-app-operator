package finalizer

import (
	"context"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/scale/scheme"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	cappName = "test-capp"
	nsName   = "test-ns"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(s))
	utilruntime.Must(cappv1alpha1.AddToScheme(s))
	utilruntime.Must(knativev1beta1.AddToScheme(s))
	utilruntime.Must(knativev1.AddToScheme(s))
	utilruntime.Must(scheme.AddToScheme(s))
	return s
}

func newFakeClient() client.Client {
	runtimeScheme := newScheme()
	return fake.NewClientBuilder().WithScheme(runtimeScheme).Build()
}

func TestEnsureFinalizer(t *testing.T) {
	ctx := context.Background()
	capp := &cappv1alpha1.Capp{
		Spec: cappv1alpha1.CappSpec{
			RouteSpec: cappv1alpha1.RouteSpec{
				TlsEnabled: true,
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cappName,
			Namespace: nsName,
		},
	}
	fakeClient := newFakeClient()
	assert.NoError(t, fakeClient.Create(ctx, capp), "Expected no error when creating capp")
	assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: nsName}, capp))
	rmClient := rclient.ResourceManagerClient{K8sClient: fakeClient, Log: logr.Discard()}
	assert.NoError(t, EnsureFinalizer(ctx, *capp, rmClient))
	assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: nsName}, capp))
	assert.Contains(t, capp.Finalizers, CappCleanupFinalizer)

	// Check if there is no error after the finalizer exists.
	assert.NoError(t, EnsureFinalizer(ctx, *capp, rmClient))
}

func TestRemoveFinalizer(t *testing.T) {
	ctx := context.Background()
	capp := &cappv1alpha1.Capp{
		Spec: cappv1alpha1.CappSpec{
			RouteSpec: cappv1alpha1.RouteSpec{
				TlsEnabled: true,
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cappName,
			Namespace: nsName,
			Finalizers: []string{
				CappCleanupFinalizer,
			},
		},
	}
	fakeClient := newFakeClient()
	rmClient := rclient.ResourceManagerClient{K8sClient: fakeClient, Log: logr.Discard()}
	assert.NoError(t, fakeClient.Create(ctx, capp))
	assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: nsName}, capp))
	assert.NoError(t, RemoveFinalizer(ctx, *capp, rmClient), "Expected no error when removing finalizer")
	assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: nsName}, capp))
	assert.NotContains(t, capp.Finalizers, CappCleanupFinalizer)

	// Check if there is no error after the finalizer removed.
	assert.NoError(t, RemoveFinalizer(ctx, *capp, rmClient))
}
