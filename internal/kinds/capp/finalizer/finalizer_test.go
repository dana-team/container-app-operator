package finalizer

import (
	"context"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/scale/scheme"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = cappv1alpha1.AddToScheme(s)
	_ = knativev1beta1.AddToScheme(s)
	_ = knativev1.AddToScheme(s)
	_ = routev1.Install(s)
	_ = scheme.AddToScheme(s)
	return s
}

func newFakeClient() client.Client {
	scheme := newScheme()
	return fake.NewClientBuilder().WithScheme(scheme).Build()
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
			Name:      "test-capp",
			Namespace: "test-ns",
		},
	}
	fakeClient := newFakeClient()
	assert.NoError(t, fakeClient.Create(ctx, capp), "Expected no error when creating capp")
	assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: "test-capp", Namespace: "test-ns"}, capp))
	assert.NoError(t, EnsureFinalizer(ctx, *capp, fakeClient))
	assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: "test-capp", Namespace: "test-ns"}, capp))
	assert.Contains(t, capp.Finalizers, FinalizerCleanupCapp)

	// Check if there is no error after the finalizer exists.
	assert.NoError(t, EnsureFinalizer(ctx, *capp, fakeClient))
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
			Name:      "test-capp",
			Namespace: "test-ns",
			Finalizers: []string{
				FinalizerCleanupCapp,
			},
		},
	}
	fakeClient := newFakeClient()
	assert.NoError(t, fakeClient.Create(ctx, capp))
	assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: "test-capp", Namespace: "test-ns"}, capp))
	assert.NoError(t, RemoveFinalizer(ctx, *capp, fakeClient), "Expected no error when removing finalizer")
	assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: "test-capp", Namespace: "test-ns"}, capp))
	assert.NotContains(t, capp.Finalizers, FinalizerCleanupCapp)

	// Check if there is no error after the finalizer removed.
	assert.NoError(t, RemoveFinalizer(ctx, *capp, fakeClient))
}
