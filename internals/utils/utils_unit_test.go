package utils_test

import (
	"context"
	"testing"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	autoscale_utils "github.com/dana-team/container-app-operator/internals/utils/autoscale"
	"github.com/dana-team/container-app-operator/internals/utils/finalizer"
	"github.com/dana-team/container-app-operator/internals/utils/secure"
	rclient "github.com/dana-team/container-app-operator/internals/wrappers"
	networkingv1 "github.com/openshift/api/network/v1"
	knativev1alphav1 "knative.dev/serving/pkg/apis/serving/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/scale/scheme"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	_ = networkingv1.AddToScheme(s)
	_ = routev1.AddToScheme(s)
	_ = scheme.AddToScheme(s)
	return s
}

func newFakeClient() client.Client {
	scheme := newScheme()
	return fake.NewClientBuilder().WithScheme(scheme).Build()
}

func genrateBaseCapp() rcsv1alpha1.Capp {
	capp := rcsv1alpha1.Capp{
		Spec: rcsv1alpha1.CappSpec{
			RouteSpec: rcsv1alpha1.RouteSpec{},
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-capp",
			Namespace: "test-ns",
		},
	}
	return capp
}

func TestSetAutoScaler(t *testing.T) {
	example_capp := rcsv1alpha1.Capp{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: rcsv1alpha1.CappSpec{
			ScaleMetric: "cpu",
		},
	}
	example_capp_cpu_expected := map[string]string{
		"autoscaling.knative.dev/class":     "hpa.autoscaling.knative.dev",
		"autoscaling.knative.dev/metric":    "cpu",
		"autoscaling.knative.dev/max-scale": "10",
		"autoscaling.knative.dev/min-scale": "2",
		"autoscaling.knative.dev/target":    "80",
	}
	annotations_cpu := autoscale_utils.SetAutoScaler(example_capp)
	assert.Equal(t, example_capp_cpu_expected, annotations_cpu)

	example_capp.Spec.ScaleMetric = "rps"
	example_capp_rps_expected := map[string]string{
		"autoscaling.knative.dev/class":     "kpa.autoscaling.knative.dev",
		"autoscaling.knative.dev/max-scale": "10",
		"autoscaling.knative.dev/min-scale": "2",
		"autoscaling.knative.dev/metric":    "rps",
		"autoscaling.knative.dev/target":    "200",
	}
	annotations_rps := autoscale_utils.SetAutoScaler(example_capp)
	assert.Equal(t, example_capp_rps_expected, annotations_rps)

}

func TestSetHttpsKnativeDomainMapping(t *testing.T) {
	ctx := context.Background()
	capp := rcsv1alpha1.Capp{
		Spec: rcsv1alpha1.CappSpec{
			RouteSpec: rcsv1alpha1.RouteSpec{
				Https: true,
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-capp",
			Namespace: "test-ns",
		},
	}

	knativeDomainMapping := &knativev1alphav1.DomainMapping{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dm",
			Namespace: "test-ns",
		},
	}

	// Create a fake client and add the capp and tls secret to it
	fakeClient := newFakeClient()

	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secure-knativedm-test-capp",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"tls.crt": []byte("dummy-cert-data"),
			"tls.key": []byte("dummy-key-data"),
		},
	}
	err := fakeClient.Create(ctx, tlsSecret)
	assert.NoError(t, err)

	// Create a resource manager using the fake client
	resourceManager := rclient.ResourceBaseManager{
		K8sclient: fakeClient,
		Ctx:       ctx,
		Log:       ctrl.Log.WithName("test"),
	}

	// Call the function being tested
	secure.SetHttpsKnativeDomainMapping(capp, knativeDomainMapping, resourceManager)

	// Check that the tls secret was set correctly
	assert.Equal(t, "secure-knativedm-test-capp", knativeDomainMapping.Spec.TLS.SecretName)
}

func TestEnsureFinalizer(t *testing.T) {
	ctx := context.Background()
	capp := &rcsv1alpha1.Capp{
		Spec: rcsv1alpha1.CappSpec{
			RouteSpec: rcsv1alpha1.RouteSpec{
				Https: true,
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
				Https: true,
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
