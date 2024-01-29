package utils_test

import (
	"context"
	"testing"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internals/utils"
	autoscaleutils "github.com/dana-team/container-app-operator/internals/utils/autoscale"
	"github.com/dana-team/container-app-operator/internals/utils/finalizer"
	networkingv1 "github.com/openshift/api/network/v1"
	knativev1alphav1 "knative.dev/serving/pkg/apis/serving/v1alpha1"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/scale/scheme"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = rcsv1alpha1.AddToScheme(s)
	_ = knativev1alphav1.AddToScheme(s)
	_ = knativev1.AddToScheme(s)
	_ = networkingv1.Install(s)
	_ = routev1.Install(s)
	_ = scheme.AddToScheme(s)
	return s
}

func newFakeClient() client.Client {
	Scheme := newScheme()
	return fake.NewClientBuilder().WithScheme(Scheme).Build()
}

func TestSetAutoScaler(t *testing.T) {
	exampleCapp := rcsv1alpha1.Capp{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: rcsv1alpha1.CappSpec{
			ScaleMetric: "cpu",
		},
	}
	exampleCappCpuExpected := map[string]string{
		"autoscaling.knative.dev/activation-scale": "3",
		"autoscaling.knative.dev/class":            "hpa.autoscaling.knative.dev",
		"autoscaling.knative.dev/metric":           "cpu",
		"autoscaling.knative.dev/target":           "80",
	}
	annotationsCpu := autoscaleutils.SetAutoScaler(exampleCapp)
	assert.Equal(t, exampleCappCpuExpected, annotationsCpu)

	exampleCapp.Spec.ScaleMetric = "rps"
	exampleCappRpsExpected := map[string]string{
		"autoscaling.knative.dev/activation-scale": "3",
		"autoscaling.knative.dev/class":            "kpa.autoscaling.knative.dev",
		"autoscaling.knative.dev/metric":           "rps",
		"autoscaling.knative.dev/target":           "200",
	}
	annotationsRps := autoscaleutils.SetAutoScaler(exampleCapp)
	assert.Equal(t, exampleCappRpsExpected, annotationsRps)

}

func TestEnsureFinalizer(t *testing.T) {
	ctx := context.Background()
	capp := &rcsv1alpha1.Capp{
		Spec: rcsv1alpha1.CappSpec{
			RouteSpec: rcsv1alpha1.RouteSpec{
				TlsEnabled: true,
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-capp",
			Namespace: "test-ns",
		},
	}
	fakeClient := newFakeClient()
	fakeClient.Create(ctx, capp)
	fakeClient.Get(ctx, types.NamespacedName{Name: "test-capp", Namespace: "test-ns"}, capp)
	assert.NoError(t, finalizer.EnsureFinalizer(ctx, *capp, fakeClient))
	fakeClient.Get(ctx, types.NamespacedName{Name: "test-capp", Namespace: "test-ns"}, capp)
	assert.Contains(t, capp.Finalizers, finalizer.FinalizerCleanupCapp)

	// Check if there is no error after the finalizer exists.
	assert.NoError(t, finalizer.EnsureFinalizer(ctx, *capp, fakeClient))
}

func TestRemoveFinalizer(t *testing.T) {
	ctx := context.Background()
	capp := &rcsv1alpha1.Capp{
		Spec: rcsv1alpha1.CappSpec{
			RouteSpec: rcsv1alpha1.RouteSpec{
				TlsEnabled: true,
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-capp",
			Namespace: "test-ns",
			Finalizers: []string{
				finalizer.FinalizerCleanupCapp,
			},
		},
	}
	fakeClient := newFakeClient()
	fakeClient.Create(ctx, capp)
	fakeClient.Get(ctx, types.NamespacedName{Name: "test-capp", Namespace: "test-ns"}, capp)
	finalizer.RemoveFinalizer(ctx, *capp, ctrl.Log, fakeClient)
	fakeClient.Get(ctx, types.NamespacedName{Name: "test-capp", Namespace: "test-ns"}, capp)
	assert.NotContains(t, capp.Finalizers, finalizer.FinalizerCleanupCapp)

	// Check if there is no error after the finalizer removed.
	assert.NoError(t, finalizer.RemoveFinalizer(ctx, *capp, ctrl.Log, fakeClient))
}

func TestFilterKeysWithoutPrefix(t *testing.T) {
	object := map[string]string{
		"prefix_key1": "value1",
		"key2":        "value2",
		"prefix_key3": "value3",
	}
	prefix := "prefix_"
	expected := map[string]string{
		"prefix_key1": "value1",
		"prefix_key3": "value3",
	}

	assert.Equal(t, expected, utils.FilterKeysWithoutPrefix(object, prefix))
}
