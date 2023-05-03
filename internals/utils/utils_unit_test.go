package utils_test

import (
	"context"
	"testing"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	autoscale_utils "github.com/dana-team/container-app-operator/internals/utils/autoscale"
	"github.com/dana-team/container-app-operator/internals/utils/finalizer"
	"github.com/dana-team/container-app-operator/internals/utils/secure"
	status_utils "github.com/dana-team/container-app-operator/internals/utils/status"
	rclient "github.com/dana-team/container-app-operator/internals/wrappers"
	networkingv1 "github.com/openshift/api/network/v1"
	v1 "knative.dev/pkg/apis/duck/v1"
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
func TestSyncStatus(t *testing.T) {
	ctx := context.Background()
	cappName := "test-capp"
	namespace := "test-ns"

	// Create a fake client.
	fakeClient := newFakeClient()

	// Create the Capp CRD object.
	capp := &rcsv1alpha1.Capp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cappName,
			Namespace: namespace,
		},
	}
	// Add the Capp object to the fake client.
	assert.NoError(t, fakeClient.Create(ctx, capp))

	// Create the knativev1.Service object.
	kservice := &knativev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cappName,
			Namespace: namespace,
		},
		Spec: knativev1.ServiceSpec{
			ConfigurationSpec: knativev1.ConfigurationSpec{
				Template: knativev1.RevisionTemplateSpec{
					Spec: knativev1.RevisionSpec{
						PodSpec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "code",
									Image: "test",
								},
							},
						},
					},
				},
			},
		},
		Status: knativev1.ServiceStatus{
			ConfigurationStatusFields: knativev1.ConfigurationStatusFields{
				LatestReadyRevisionName:   cappName + "-001",
				LatestCreatedRevisionName: cappName + "-001",
			},
			Status: v1.Status{
				Conditions: v1.Conditions{},
			},
		},
	}

	revision := &knativev1.Revision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cappName + "-001",
			Namespace: namespace,
			Annotations: map[string]string{
				"serving.knative.dev/configuration": cappName,
			},
		},
		Spec: knativev1.RevisionSpec{
			PodSpec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "code",
						Image: "test",
					},
				},
			},
		},
		Status: knativev1.RevisionStatus{
			ContainerStatuses: []knativev1.ContainerStatus{
				{Name: "code"},
			},
		},
	}
	// Add the knativev1.Service object to the fake client.
	assert.NoError(t, fakeClient.Create(ctx, kservice))
	assert.NoError(t, fakeClient.Create(ctx, revision))

	// Create the routev1.Route object for the cluster console.
	clusterConsole := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "console",
			Namespace: "openshift-console",
		},
		Spec: routev1.RouteSpec{
			Host: "console-openshift-console.apps.cluster.example.com",
		},
	}
	// Add the routev1.Route object to the fake client.
	assert.NoError(t, fakeClient.Create(ctx, clusterConsole))

	// Create the networkingv1.HostSubnet object for the cluster segment.
	clusterSubnet := &networkingv1.HostSubnet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
		},
		Subnet: "10.0.0.0/24",
	}
	// Add the networkingv1.HostSubnet object to the fake client.
	assert.NoError(t, fakeClient.Create(ctx, clusterSubnet))

	// Call SyncStatus function.
	assert.NoError(t, status_utils.SyncStatus(context.Background(), *capp, ctrl.Log.WithName("test"), fakeClient))
	assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: namespace}, capp))

	// Check that the RevisionInfo status was updated correctly.
	revisions := capp.Status.RevisionInfo
	assert.Equal(t, 1, len(revisions))
	assert.Equal(t, "test-capp-001", revisions[0].RevisionName)
	assert.Equal(t, corev1.ConditionTrue, revisions[0].RevisionStatus.Conditions[0].Status)

	// Check that the ApplicationLinks status was updated correctly.
	appLinks := capp.Status.ApplicationLinks
	assert.NotNil(t, appLinks)
	assert.Equal(t, "console-openshift-console.apps.cluster.example.com", appLinks.ConsoleLink)
	assert.Equal(t, "cluster", appLinks.Site)
	assert.Equal(t, "10.0.0.0/24", appLinks.ClusterSegment)
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
	finalizer.RemoveFinalizer(ctx, *capp, ctrl.Log, fakeClient)
	assert.NotContains(t, capp.Finalizers, finalizer.FinalizerCleanupCapp)
	// Check if there is no error after the finalizer removed.
	assert.NoError(t, finalizer.RemoveFinalizer(ctx, *capp, ctrl.Log, fakeClient))
}
