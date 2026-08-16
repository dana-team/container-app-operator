package controllers

import (
	"context"
	"fmt"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type stubResourceManager struct {
	cleanUpErr error
}

func (s stubResourceManager) Manage(_ context.Context, _ cappv1alpha1.Capp) error { return nil }
func (s stubResourceManager) CleanUp(_ context.Context, _ cappv1alpha1.Capp) error {
	return s.cleanUpErr
}
func (s stubResourceManager) IsRequired(_ cappv1alpha1.Capp) bool { return true }

const (
	cappName = "test-capp"
	nsName   = "test-ns"
	stubKey  = "stub"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(s))
	utilruntime.Must(cappv1alpha1.AddToScheme(s))
	utilruntime.Must(knativev1beta1.AddToScheme(s))
	utilruntime.Must(knativev1.AddToScheme(s))
	return s
}

func newFakeClient() client.Client {
	return fake.NewClientBuilder().WithScheme(newScheme()).Build()
}

func newCapp(finalizers ...string) *cappv1alpha1.Capp {
	return &cappv1alpha1.Capp{
		ObjectMeta: metav1.ObjectMeta{
			Name:       cappName,
			Namespace:  nsName,
			Finalizers: finalizers,
		},
	}
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
	assert.NoError(t, fakeClient.Create(ctx, capp))
	assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: nsName}, capp))
	rmClient := rclient.ResourceManagerClient{K8sClient: fakeClient, Log: logr.Discard()}

	t.Run("adds finalizer when absent", func(t *testing.T) {
		assert.NoError(t, ensureFinalizer(ctx, *capp, rmClient))
		assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: nsName}, capp))
		assert.Contains(t, capp.Finalizers, cappCleanupFinalizer)
	})

	t.Run("no-op when finalizer already present", func(t *testing.T) {
		assert.NoError(t, ensureFinalizer(ctx, *capp, rmClient))
		assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: nsName}, capp))
		assert.Contains(t, capp.Finalizers, cappCleanupFinalizer)
	})
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
				cappCleanupFinalizer,
			},
		},
	}
	fakeClient := newFakeClient()
	rmClient := rclient.ResourceManagerClient{K8sClient: fakeClient, Log: logr.Discard()}
	assert.NoError(t, fakeClient.Create(ctx, capp))
	assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: nsName}, capp))

	t.Run("removes finalizer when present", func(t *testing.T) {
		assert.NoError(t, removeFinalizer(ctx, *capp, rmClient))
		assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: nsName}, capp))
		assert.NotContains(t, capp.Finalizers, cappCleanupFinalizer)
	})

	t.Run("no-op when finalizer already absent", func(t *testing.T) {
		assert.NoError(t, removeFinalizer(ctx, *capp, rmClient))
		assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: nsName}, capp))
		assert.NotContains(t, capp.Finalizers, cappCleanupFinalizer)
	})
}

func TestHandleResourceDeletion(t *testing.T) {
	ctx := context.Background()

	t.Run("returns false when not deleting", func(t *testing.T) {
		capp := newCapp(cappCleanupFinalizer)
		fakeClient := newFakeClient()
		assert.NoError(t, fakeClient.Create(ctx, capp))

		rmClient := rclient.ResourceManagerClient{K8sClient: fakeClient, Log: logr.Discard()}
		managers := []rmanagers.ResourceManagerEntry{{Name: stubKey, Manager: stubResourceManager{}}}

		deleted, err := handleResourceDeletion(ctx, *capp, rmClient, managers)

		assert.NoError(t, err)
		assert.False(t, deleted)
		assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: nsName}, capp))
		assert.Contains(t, capp.Finalizers, cappCleanupFinalizer)
	})

	t.Run("removes finalizer and returns true on successful cleanup", func(t *testing.T) {
		capp := newCapp(cappCleanupFinalizer)
		fakeClient := newFakeClient()
		assert.NoError(t, fakeClient.Create(ctx, capp))
		assert.NoError(t, fakeClient.Delete(ctx, capp))
		assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: nsName}, capp))

		rmClient := rclient.ResourceManagerClient{K8sClient: fakeClient, Log: logr.Discard()}
		managers := []rmanagers.ResourceManagerEntry{{Name: stubKey, Manager: stubResourceManager{}}}

		deleted, err := handleResourceDeletion(ctx, *capp, rmClient, managers)

		assert.NoError(t, err)
		assert.True(t, deleted)
	})

	t.Run("returns error when cleanup fails", func(t *testing.T) {
		cleanUpErr := fmt.Errorf("cleanup failed")
		capp := newCapp(cappCleanupFinalizer)
		fakeClient := newFakeClient()
		assert.NoError(t, fakeClient.Create(ctx, capp))
		assert.NoError(t, fakeClient.Delete(ctx, capp))
		assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: nsName}, capp))

		rmClient := rclient.ResourceManagerClient{K8sClient: fakeClient, Log: logr.Discard()}
		managers := []rmanagers.ResourceManagerEntry{{Name: stubKey, Manager: stubResourceManager{cleanUpErr: cleanUpErr}}}

		deleted, err := handleResourceDeletion(ctx, *capp, rmClient, managers)

		assert.ErrorIs(t, err, cleanUpErr)
		assert.False(t, deleted)
		assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: nsName}, capp))
		assert.Contains(t, capp.Finalizers, cappCleanupFinalizer)
	})

	t.Run("returns false when finalizer absent", func(t *testing.T) {
		capp := newCapp("other-finalizer")
		fakeClient := newFakeClient()
		assert.NoError(t, fakeClient.Create(ctx, capp))
		assert.NoError(t, fakeClient.Delete(ctx, capp))
		assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: nsName}, capp))

		rmClient := rclient.ResourceManagerClient{K8sClient: fakeClient, Log: logr.Discard()}
		managers := []rmanagers.ResourceManagerEntry{{Name: stubKey, Manager: stubResourceManager{}}}

		deleted, err := handleResourceDeletion(ctx, *capp, rmClient, managers)

		assert.NoError(t, err)
		assert.False(t, deleted)
	})
}
