package resourceprepares

import (
	"context"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/wrappers"
	"github.com/go-logr/logr"
	networkingv1 "github.com/openshift/api/network/v1"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = cappv1alpha1.AddToScheme(s)
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

func TestSetHttpsKnativeDomainMapping(t *testing.T) {
	ctx := context.Background()
	capp := cappv1alpha1.Capp{
		Spec: cappv1alpha1.CappSpec{
			RouteSpec: cappv1alpha1.RouteSpec{
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
	log := logr.Discard()

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
	domainMappingManger := KnativeDomainMappingManager{Ctx: ctx, Log: log, K8sclient: fakeClient, EventRecorder: &record.FakeRecorder{}}
	// Call the function being tested
	domainMappingManger.setHttpsKnativeDomainMapping(capp, knativeDomainMapping, resourceManager)

	// Check that the tls secret was set correctly
	assert.Equal(t, "secure-knativedm-test-capp", knativeDomainMapping.Spec.TLS.SecretName)
}
