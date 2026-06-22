package controllers

import (
	"context"
	"testing"
	"time"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	cappName      = "my-capp"
	cappNamespace = "my-ns"
	testCappUID   = types.UID("test-uid")

	stateDisabled       = "disabled"
	defaultHistoryLimit = 10
	defaultRevisionName = "rev-00001"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(s))
	utilruntime.Must(cappv1alpha1.AddToScheme(s))
	return s
}

func newFakeClient(objects ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(newScheme()).
		WithObjects(objects...).
		Build()
}

func newBaseCapp() *cappv1alpha1.Capp {
	return &cappv1alpha1.Capp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cappName,
			Namespace: cappNamespace,
			UID:       testCappUID,
		},
		Spec: cappv1alpha1.CappSpec{
			ConfigurationSpec: knativev1.ConfigurationSpec{
				Template: knativev1.RevisionTemplateSpec{
					Spec: knativev1.RevisionSpec{
						PodSpec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "app",
								Image: "example.com/app:v1",
							}},
						},
					},
				},
			},
		},
	}
}

func newDefaultCappConfig() *cappv1alpha1.CappConfig {
	return &cappv1alpha1.CappConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.CappConfigName,
			Namespace: utils.CappNS,
		},
		Spec: cappv1alpha1.CappConfigSpec{
			RevisionHistoryLimit: defaultHistoryLimit,
		},
	}
}

func newCappRevision(capp *cappv1alpha1.Capp, creationTime time.Time) *cappv1alpha1.CappRevision {
	return &cappv1alpha1.CappRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:              defaultRevisionName,
			Namespace:         capp.Namespace,
			CreationTimestamp: metav1.NewTime(creationTime),
			Labels: map[string]string{
				cappv1alpha1.GroupVersion.Group + "/cappName": capp.Name,
			},
		},
		Spec: cappv1alpha1.CappRevisionSpec{
			RevisionNumber: 1,
			CappTemplate: cappv1alpha1.CappTemplate{
				Spec:        *capp.Spec.DeepCopy(),
				Labels:      capp.Labels,
				Annotations: capp.Annotations,
			},
		},
	}
}

func newReconciler(k8sClient client.Client) *CappRevisionReconciler {
	return &CappRevisionReconciler{
		Log:    logr.Discard(),
		Client: k8sClient,
		Scheme: newScheme(),
	}
}

func newRequest() reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      cappName,
			Namespace: cappNamespace,
		},
	}
}

func TestReconcile(t *testing.T) {
	ctx := context.Background()

	t.Run("returns empty result when capp not found", func(t *testing.T) {
		r := newReconciler(newFakeClient())

		result, err := r.Reconcile(ctx, newRequest())

		require.NoError(t, err)
		require.Equal(t, ctrl.Result{}, result)
	})

	t.Run("returns empty result when capp is deleting", func(t *testing.T) {
		capp := newBaseCapp()
		now := metav1.Now()
		capp.DeletionTimestamp = &now
		capp.Finalizers = []string{"test-finalizer"}

		r := newReconciler(newFakeClient(capp))

		result, err := r.Reconcile(ctx, newRequest())

		require.NoError(t, err)
		require.Equal(t, ctrl.Result{}, result)
	})

	t.Run("creates first revision for new capp", func(t *testing.T) {
		capp := newBaseCapp()
		r := newReconciler(newFakeClient(capp, newDefaultCappConfig()))

		result, err := r.Reconcile(ctx, newRequest())

		require.NoError(t, err)
		require.Equal(t, ctrl.Result{}, result)

		revList := &cappv1alpha1.CappRevisionList{}
		require.NoError(t, r.List(ctx, revList))
		require.Len(t, revList.Items, 1)
		require.Equal(t, 1, revList.Items[0].Spec.RevisionNumber)
	})

	t.Run("creates new revision when capp spec changes", func(t *testing.T) {
		capp := newBaseCapp()
		rev := newCappRevision(capp, time.Now())
		r := newReconciler(newFakeClient(capp, rev, newDefaultCappConfig()))

		capp.Spec.State = stateDisabled
		require.NoError(t, r.Update(ctx, capp))

		result, err := r.Reconcile(ctx, newRequest())

		require.NoError(t, err)
		require.Equal(t, ctrl.Result{}, result)

		revList := &cappv1alpha1.CappRevisionList{}
		require.NoError(t, r.List(ctx, revList))
		require.Len(t, revList.Items, 2)
	})

	t.Run("no-op when capp matches latest revision", func(t *testing.T) {
		capp := newBaseCapp()
		rev := newCappRevision(capp, time.Now())
		r := newReconciler(newFakeClient(capp, rev, newDefaultCappConfig()))

		result, err := r.Reconcile(ctx, newRequest())

		require.NoError(t, err)
		require.Equal(t, ctrl.Result{}, result)

		revList := &cappv1alpha1.CappRevisionList{}
		require.NoError(t, r.List(ctx, revList))
		require.Len(t, revList.Items, 1)
	})

	t.Run("returns error when capp config is missing with existing revisions", func(t *testing.T) {
		capp := newBaseCapp()
		rev := newCappRevision(capp, time.Now())
		r := newReconciler(newFakeClient(capp, rev))

		capp.Spec.State = stateDisabled
		require.NoError(t, r.Update(ctx, capp))

		_, err := r.Reconcile(ctx, newRequest())

		require.Error(t, err)
	})
}

func TestSyncCappRevision(t *testing.T) {
	ctx := context.Background()
	logger := logr.Discard()

	t.Run("creates first revision when no revisions exist", func(t *testing.T) {
		capp := newBaseCapp()
		k8sClient := newFakeClient(capp)

		require.NoError(t, syncCappRevision(ctx, k8sClient, *capp, logger))

		revList := &cappv1alpha1.CappRevisionList{}
		require.NoError(t, k8sClient.List(ctx, revList))
		require.Len(t, revList.Items, 1)
	})

	t.Run("calls update flow when revisions exist", func(t *testing.T) {
		capp := newBaseCapp()
		rev := newCappRevision(capp, time.Now())
		k8sClient := newFakeClient(capp, rev, newDefaultCappConfig())

		capp.Spec.State = stateDisabled
		require.NoError(t, k8sClient.Update(ctx, capp))

		require.NoError(t, syncCappRevision(ctx, k8sClient, *capp, logger))

		revList := &cappv1alpha1.CappRevisionList{}
		require.NoError(t, k8sClient.List(ctx, revList))
		require.Len(t, revList.Items, 2)
	})
}
