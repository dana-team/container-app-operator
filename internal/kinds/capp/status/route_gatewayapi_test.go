package status

import (
	"context"
	"testing"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/utils/ptr"
	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	gatewayName      = "eg-external"
	gatewayNamespace = "eg-external"
	gatewayObject    = "capp-" + resourceName
)

func newGatewayRouteScheme() *runtime.Scheme {
	s := newRouteScheme()
	utilruntime.Must(gatewayv1.Install(s))
	utilruntime.Must(netv1alpha1.AddToScheme(s))
	return s
}

func newGatewayCappConfig(mode cappv1alpha1.GatewayTLSMode) *cappv1alpha1.CappConfig {
	cfg := newCappConfig()
	cfg.Spec.GatewayAPI = &cappv1alpha1.GatewayAPIConfig{
		Gateway:             cappv1alpha1.NamespacedName{Name: gatewayName, Namespace: gatewayNamespace},
		LocalGatewayService: cappv1alpha1.ServiceReference{Name: "knative-internal", Namespace: "envoy-gateway-system"},
		TLSMode:             mode,
	}
	return cfg
}

func newClaim(namespace string) *netv1alpha1.ClusterDomainClaim {
	return &netv1alpha1.ClusterDomainClaim{
		ObjectMeta: metav1.ObjectMeta{Name: resourceName},
		Spec:       netv1alpha1.ClusterDomainClaimSpec{Namespace: namespace},
	}
}

func newListenerSet(conditions ...metav1.Condition) *gatewayv1.ListenerSet {
	ls := &gatewayv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Name: gatewayObject, Namespace: cappNamespace}}
	ls.Status.Conditions = conditions
	return ls
}

func routeParent(name, namespace, kind string, conditions ...metav1.Condition) gatewayv1.RouteParentStatus {
	return gatewayv1.RouteParentStatus{
		ParentRef: gatewayv1.ParentReference{
			Kind:      ptr.To(gatewayv1.Kind(kind)),
			Name:      gatewayv1.ObjectName(name),
			Namespace: ptr.To(gatewayv1.Namespace(namespace)),
		},
		Conditions: conditions,
	}
}

func newHTTPRoute(withListenerSet bool, parents ...gatewayv1.RouteParentStatus) *gatewayv1.HTTPRoute {
	route := &gatewayv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: gatewayObject, Namespace: cappNamespace}}
	route.Spec.ParentRefs = []gatewayv1.ParentReference{{
		Kind:      ptr.To(gatewayv1.Kind("Gateway")),
		Name:      gatewayName,
		Namespace: ptr.To(gatewayv1.Namespace(gatewayNamespace)),
	}}
	if withListenerSet {
		route.Spec.ParentRefs = append(route.Spec.ParentRefs, gatewayv1.ParentReference{
			Kind: ptr.To(gatewayv1.Kind("ListenerSet")),
			Name: gatewayObject,
		})
	}
	route.Status.Parents = parents
	return route
}

func TestCheckRouteGatewayAPI(t *testing.T) {
	readyCNAME := newCNAMERecord(
		xpv2.Condition{Type: xpv2.TypeSynced, Status: corev1.ConditionTrue},
		xpv2.Condition{Type: xpv2.TypeReady, Status: corev1.ConditionTrue},
	)
	readyCert := newCertificate(cmapi.CertificateCondition{Type: cmapi.CertificateConditionReady, Status: cmmeta.ConditionTrue})
	accepted := metav1.Condition{Type: string(gatewayv1.RouteConditionAccepted), Status: metav1.ConditionTrue}
	resolved := metav1.Condition{Type: string(gatewayv1.RouteConditionResolvedRefs), Status: metav1.ConditionTrue}
	gatewayParent := routeParent(gatewayName, gatewayNamespace, "Gateway", accepted, resolved)
	listenerSetParent := routeParent(gatewayObject, cappNamespace, "ListenerSet", accepted, resolved)
	readyListenerSet := newListenerSet(
		metav1.Condition{Type: string(gatewayv1.ListenerSetConditionAccepted), Status: metav1.ConditionTrue},
		metav1.Condition{Type: string(gatewayv1.ListenerSetConditionProgrammed), Status: metav1.ConditionTrue},
	)

	pending := func(kind, name string) *metav1.Condition {
		return &metav1.Condition{
			Status:  metav1.ConditionUnknown,
			Reason:  cappv1alpha1.CapabilityReasonPending,
			Message: kind + " " + name + " not observed yet",
		}
	}

	tests := []struct {
		name    string
		mode    cappv1alpha1.GatewayTLSMode
		tls     bool
		objects []client.Object
		want    *metav1.Condition
	}{
		{
			name:    "pending until the hostname is claimed",
			objects: []client.Object{readyCNAME},
			want:    pending("ClusterDomainClaim", resourceName),
		},
		{
			name:    "reports a hostname claimed by another namespace",
			objects: []client.Object{readyCNAME, newClaim("other-ns")},
			want: &metav1.Condition{
				Status:  metav1.ConditionFalse,
				Reason:  cappv1alpha1.CapabilityReasonNotReady,
				Message: `ClusterDomainClaim ` + resourceName + `: hostname is claimed by namespace "other-ns"`,
			},
		},
		{
			name: "reports a hostname another Capp of the namespace holds",
			objects: []client.Object{readyCNAME, func() client.Object {
				c := newClaim(cappNamespace)
				c.Labels = map[string]string{cappmeta.CappResourceKey: "holder"}
				return c
			}()},
			want: &metav1.Condition{
				Status:  metav1.ConditionFalse,
				Reason:  cappv1alpha1.CapabilityReasonNotReady,
				Message: `ClusterDomainClaim ` + resourceName + `: hostname is claimed by Capp "holder"`,
			},
		},
		{
			name:    "pending while the Gateway has not reported on the route",
			objects: []client.Object{readyCNAME, newClaim(cappNamespace), newHTTPRoute(false)},
			want:    pending("HTTPRoute", gatewayObject),
		},
		{
			name: "reports a backend the Gateway cannot resolve",
			objects: []client.Object{readyCNAME, newClaim(cappNamespace), newHTTPRoute(false, routeParent(gatewayName, gatewayNamespace, "Gateway", accepted,
				metav1.Condition{Type: string(gatewayv1.RouteConditionResolvedRefs), Status: metav1.ConditionFalse, Reason: "RefNotPermitted", Message: "missing ReferenceGrant"}))},
			want: &metav1.Condition{
				Status:  metav1.ConditionFalse,
				Reason:  "RefNotPermitted",
				Message: "HTTPRoute " + gatewayObject + ": missing ReferenceGrant",
			},
		},
		{
			name:    "nil when the route is accepted and resolved without TLS",
			objects: []client.Object{readyCNAME, newClaim(cappNamespace), newHTTPRoute(false, gatewayParent)},
		},
		{
			name:    "checks the certificate and the ListenerSet in ListenerSet mode",
			tls:     true,
			objects: []client.Object{readyCNAME, newClaim(cappNamespace), readyCert, newHTTPRoute(false, gatewayParent)},
			want:    pending("ListenerSet", gatewayObject),
		},
		{
			name: "reports a ListenerSet that is not programmed",
			tls:  true,
			objects: []client.Object{readyCNAME, newClaim(cappNamespace), readyCert, newListenerSet(
				metav1.Condition{Type: string(gatewayv1.ListenerSetConditionAccepted), Status: metav1.ConditionTrue},
				metav1.Condition{Type: string(gatewayv1.ListenerSetConditionProgrammed), Status: metav1.ConditionFalse, Reason: "Invalid", Message: "certificate not found"},
			)},
			want: &metav1.Condition{
				Status:  metav1.ConditionFalse,
				Reason:  "Invalid",
				Message: "ListenerSet " + gatewayObject + ": certificate not found",
			},
		},
		{
			name:    "pending until the route is attached to the ListenerSet",
			tls:     true,
			objects: []client.Object{readyCNAME, newClaim(cappNamespace), readyCert, readyListenerSet, newHTTPRoute(false, gatewayParent)},
			want:    pending("HTTPRoute", gatewayObject),
		},
		{
			name:    "nil when the route is accepted by the Gateway and the ListenerSet",
			tls:     true,
			objects: []client.Object{readyCNAME, newClaim(cappNamespace), readyCert, readyListenerSet, newHTTPRoute(true, gatewayParent, listenerSetParent)},
		},
		{
			name:    "skips the certificate and the ListenerSet in Gateway mode",
			mode:    cappv1alpha1.GatewayTLSModeGateway,
			tls:     true,
			objects: []client.Object{readyCNAME, newClaim(cappNamespace), newHTTPRoute(false, gatewayParent)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(newGatewayRouteScheme()).WithObjects(tt.objects...).Build()

			capp := routeCapp()
			capp.Spec.RouteSpec.TlsEnabled = tt.tls

			got, err := checkRoute(context.Background(), fakeClient, capp, newGatewayCappConfig(tt.mode))
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
