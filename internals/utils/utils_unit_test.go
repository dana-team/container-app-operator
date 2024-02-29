package utils_test

import (
	"context"
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internals/utils"
	autoscaleutils "github.com/dana-team/container-app-operator/internals/utils/autoscale"
	"github.com/dana-team/container-app-operator/internals/utils/finalizer"
	"github.com/dana-team/container-app-operator/internals/utils/secure"
	rclient "github.com/dana-team/container-app-operator/internals/wrappers"
	networkingv1 "github.com/openshift/api/network/v1"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/scale/scheme"
	"k8s.io/client-go/tools/record"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = rcsv1alpha1.AddToScheme(s)
	_ = knativev1beta1.AddToScheme(s)
	_ = knativev1.AddToScheme(s)
	_ = networkingv1.Install(s)
	_ = routev1.Install(s)
	_ = scheme.AddToScheme(s)
	return s
}

func newFakeClient() client.Client {
	scheme := newScheme()
	return fake.NewClientBuilder().WithScheme(scheme).Build()
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
		"autoscaling.knative.dev/class":  "hpa.autoscaling.knative.dev",
		"autoscaling.knative.dev/metric": "cpu",
		"autoscaling.knative.dev/target": "80",
	}
	annotationsCpu := autoscaleutils.SetAutoScaler(exampleCapp)
	assert.Equal(t, exampleCappCpuExpected, annotationsCpu)

	exampleCapp.Spec.ScaleMetric = "rps"
	exampleCappRpsExpected := map[string]string{
		"autoscaling.knative.dev/class":  "kpa.autoscaling.knative.dev",
		"autoscaling.knative.dev/metric": "rps",
		"autoscaling.knative.dev/target": "200",
	}
	annotationsRps := autoscaleutils.SetAutoScaler(exampleCapp)
	assert.Equal(t, exampleCappRpsExpected, annotationsRps)

}

func TestSetHttpsKnativeDomainMapping(t *testing.T) {
	ctx := context.Background()
	capp := rcsv1alpha1.Capp{
		Spec: rcsv1alpha1.CappSpec{
			RouteSpec: rcsv1alpha1.RouteSpec{
				TlsEnabled: true,
				Hostname:   "test-dm",
				TlsSecret:  "secure-knativedm-test-capp",
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-capp",
			Namespace: "test-ns",
		},
	}

	knativeDomainMapping := &knativev1beta1.DomainMapping{
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
	resourceManager := rclient.ResourceBaseManagerClient{
		K8sclient: fakeClient,
		Ctx:       ctx,
		Log:       ctrl.Log.WithName("test"),
	}

	// Call the function being tested
	secure.SetHttpsKnativeDomainMapping(capp, knativeDomainMapping, resourceManager, &record.FakeRecorder{})

	// Check that the tls secret was set correctly
	assert.Equal(t, "secure-knativedm-test-capp", knativeDomainMapping.Spec.TLS.SecretName)
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
	assert.NoError(t, fakeClient.Create(ctx, capp), "Expected no error when creating capp")
	assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: "test-capp", Namespace: "test-ns"}, capp))
	assert.NoError(t, finalizer.EnsureFinalizer(ctx, *capp, fakeClient))
	assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: "test-capp", Namespace: "test-ns"}, capp))
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
	assert.NoError(t, fakeClient.Create(ctx, capp))
	assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: "test-capp", Namespace: "test-ns"}, capp))
	assert.NoError(t, finalizer.RemoveFinalizer(ctx, *capp, fakeClient), "Expected no error when removing finalizer")
	assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: "test-capp", Namespace: "test-ns"}, capp))
	assert.NotContains(t, capp.Finalizers, finalizer.FinalizerCleanupCapp)

	// Check if there is no error after the finalizer removed.
	assert.NoError(t, finalizer.RemoveFinalizer(ctx, *capp, fakeClient))
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
