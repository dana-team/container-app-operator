package resourceprepares_test

import (
	"context"
	"testing"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	resourceprepares "github.com/dana-team/container-app-operator/internals/resource-managers"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/scale/scheme"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const CappResourceKey = "rcs.dana.io/parent-capp"

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = rcsv1alpha1.AddToScheme(s)
	_ = knativev1beta1.AddToScheme(s)
	_ = knativev1.AddToScheme(s)
	_ = scheme.AddToScheme(s)
	return s
}

func newFakeClient() client.Client {
	scheme := newScheme()
	return fake.NewClientBuilder().WithScheme(scheme).Build()
}

func generateBaseCapp() rcsv1alpha1.Capp {
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

func TestCleanUpKnativeSerivce(t *testing.T) {
	fakeClient := newFakeClient()
	ctx := context.Background()
	capp := generateBaseCapp()
	kService := knativev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-capp",
			Namespace: "test-ns",
		},
		Spec: knativev1.ServiceSpec{
			ConfigurationSpec: capp.Spec.ConfigurationSpec,
		},
	}
	assert.NoError(t, fakeClient.Create(ctx, &capp), "Expected no error when creating capp")
	assert.NoError(t, fakeClient.Create(ctx, &kService), "Expected no error when creating knative service")
	knativeManager := resourceprepares.KnativeServiceManager{Ctx: ctx, Log: logr.Logger{}, K8sclient: fakeClient}
	assert.NoError(t, knativeManager.CleanUp(capp), "Expected no error when calling clean up.")
	err := fakeClient.Get(ctx, types.NamespacedName{Name: "test-capp", Namespace: "test-ns"}, &kService)
	assert.True(t, errors.IsNotFound(err), "Expected the knative service to be deleted.")
}

func TestCleanUpDomainMapping(t *testing.T) {
	fakeClient := newFakeClient()
	ctx := context.Background()
	capp := generateBaseCapp()
	capp.Spec.RouteSpec.Hostname = "test-dm"
	domainMapping := knativev1beta1.DomainMapping{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dm",
			Namespace: "test-ns",
		},
		Spec: knativev1beta1.DomainMappingSpec{},
	}
	assert.NoError(t, fakeClient.Create(ctx, &capp), "Expected no error when creating capp")
	assert.NoError(t, fakeClient.Create(ctx, &domainMapping), "Expected no error when creating knative")
	knativeManager := resourceprepares.KnativeServiceManager{Ctx: ctx, Log: logr.Logger{}, K8sclient: fakeClient}
	assert.NoError(t, knativeManager.CleanUp(capp), "Expected no error when calling clean up.")
	err := fakeClient.Get(ctx, types.NamespacedName{Name: "test-capp", Namespace: "test-ns"}, &domainMapping)
	assert.True(t, errors.IsNotFound(err), "Expected the knative service to be deleted.")
}

func TestDommainMappingHostname(t *testing.T) {
	fakeClient := newFakeClient()
	ctx := context.Background()
	capp := generateBaseCapp()
	capp.Spec.RouteSpec.Hostname = "dma.dev"
	domainMapping := knativev1beta1.DomainMapping{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dma.dev",
			Namespace: "test-ns",
			Labels: map[string]string{
				CappResourceKey: capp.Name,
			},
			Annotations: map[string]string{
				CappResourceKey: capp.Name,
			},
		},
		Spec: knativev1beta1.DomainMappingSpec{},
	}
	assert.NoError(t, fakeClient.Create(ctx, &capp))
	assert.NoError(t, fakeClient.Create(ctx, &domainMapping))
	capp.Spec.RouteSpec.Hostname = "dmc.dev"
	assert.NoError(t, fakeClient.Update(ctx, &capp))
	knativeManager := resourceprepares.KnativeDomainMappingManager{Ctx: ctx, Log: logr.Logger{}, K8sclient: fakeClient}
	assert.NoError(t, knativeManager.HandleIrrelevantDomainMapping(capp), "Expected no error when calling Handling DomainMapping hostname.")
	err := fakeClient.Get(ctx, types.NamespacedName{Name: "dma.dev", Namespace: "test-ns"}, &domainMapping)
	assert.True(t, errors.IsNotFound(err), "Expected the DomainMapping to be deleted.")
}
