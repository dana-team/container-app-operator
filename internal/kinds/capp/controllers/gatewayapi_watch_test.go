package controllers

import (
	"reflect"
	"testing"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestManagedResourcePredicateSkipsKnativeRoutes(t *testing.T) {
	labels := cappmeta.ManagedResourceLabels("my-capp")
	knativeRoute := &gatewayv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Labels: labels,
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "networking.internal.knative.dev/v1alpha1", Kind: "Ingress", Name: "x"}}}}
	operatorRoute := &gatewayv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Labels: labels,
		OwnerReferences: []metav1.OwnerReference{{APIVersion: cappv1alpha1.GroupVersion.String(), Kind: reflect.TypeFor[cappv1alpha1.Capp]().Name(), Name: "my-capp"}}}}

	p := managedResourcePredicate()
	require.False(t, p.Create(event.CreateEvent{Object: knativeRoute}), "Knative copies the Capp labels onto its routes")
	require.True(t, p.Create(event.CreateEvent{Object: operatorRoute}))
}

func TestIssuedTLSSecretPredicate(t *testing.T) {
	issued := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Labels:      cappmeta.ManagedResourceLabels("my-capp"),
		Annotations: map[string]string{cmapi.CertificateNameKey: "app.example.com"},
	}}
	unlabelled := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Annotations: issued.Annotations}}

	p := issuedTLSSecretPredicate()
	require.True(t, p.Create(event.CreateEvent{Object: issued}))
	require.False(t, p.Create(event.CreateEvent{Object: unlabelled}))
	require.False(t, p.Update(event.UpdateEvent{ObjectOld: issued, ObjectNew: issued}))
}
