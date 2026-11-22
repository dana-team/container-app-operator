package resourcemanagers

import (
	"context"
	"strings"
	"testing"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/tools/events"
	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	netheader "knative.dev/networking/pkg/http/header"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/network"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const (
	testGatewayName        = "eg-external"
	testGatewayNamespace   = "eg-external"
	testLocalGatewayName   = "knative-internal"
	testLocalGatewayNS     = "envoy-gateway-system"
	testLocalGatewayPort   = int32(8080)
	testOtherCappNamespace = "other-ns"
	testGatewayObject      = "capp-" + hostnameFQDN
)

func newGatewayScheme() *runtime.Scheme {
	s := newScheme()
	utilruntime.Must(gatewayv1.Install(s))
	utilruntime.Must(gatewayv1beta1.Install(s))
	utilruntime.Must(netv1alpha1.AddToScheme(s))
	utilruntime.Must(knativev1beta1.AddToScheme(s))
	utilruntime.Must(cmapi.AddToScheme(s))
	return s
}

// newIssuedSecret returns the TLS Secret as cert-manager writes it for the Certificate named after the hostname.
func newIssuedSecret() *corev1.Secret {
	secret := newSecret(generateTLSSecretName(hostnameFQDN))
	secret.Annotations = map[string]string{cmapi.CertificateNameKey: hostnameFQDN}
	return secret
}

func newReadyDomainMapping() *knativev1beta1.DomainMapping {
	dm := &knativev1beta1.DomainMapping{ObjectMeta: metav1.ObjectMeta{Name: hostnameFQDN, Namespace: cappNamespace}}
	dm.Status.Conditions = duckv1.Conditions{{Type: knativev1beta1.DomainMappingConditionReady, Status: corev1.ConditionTrue}}
	return dm
}

func newOwnedRoute(capp cappv1alpha1.Capp, name string) *gatewayv1.HTTPRoute {
	return &gatewayv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: capp.Namespace, Labels: cappmeta.ManagedResourceLabels(capp.Name), OwnerReferences: cappOwnerRefs(capp),
	}}
}

func newGatewayClient(objects ...client.Object) client.Client {
	return newFakeClient(newGatewayScheme(), objects...)
}

func newCappConfigWithGatewayAPI(mode cappv1alpha1.GatewayTLSMode) *cappv1alpha1.CappConfig {
	cfg := newCappConfigWithDNS()
	cfg.Spec.GatewayAPI = &cappv1alpha1.GatewayAPIConfig{
		Gateway:             cappv1alpha1.NamespacedName{Name: testGatewayName, Namespace: testGatewayNamespace},
		LocalGatewayService: cappv1alpha1.ServiceReference{Name: testLocalGatewayName, Namespace: testLocalGatewayNS, Port: testLocalGatewayPort},
		TLSMode:             mode,
	}
	return cfg
}

func cappOwnerRefs(capp cappv1alpha1.Capp) []metav1.OwnerReference {
	return []metav1.OwnerReference{{APIVersion: cappv1alpha1.GroupVersion.String(), Kind: "Capp", Name: capp.Name, UID: capp.UID}}
}

func newRMClient(k8sClient client.Client) rclient.ResourceManagerClient {
	return rclient.ResourceManagerClient{K8sClient: k8sClient, Log: logr.Discard()}
}

func newClaimManager(k8sClient client.Client, cfg *cappv1alpha1.CappConfig) ClusterDomainClaimManager {
	return ClusterDomainClaimManager{ResourceManagerClient: newRMClient(k8sClient), EventRecorder: events.NewFakeRecorder(10), CappConfig: cfg}
}

func newListenerSetManager(k8sClient client.Client, cfg *cappv1alpha1.CappConfig) ListenerSetManager {
	return ListenerSetManager{ResourceManagerClient: newRMClient(k8sClient), EventRecorder: events.NewFakeRecorder(10), CappConfig: cfg, Installed: true}
}

func newHTTPRouteManager(k8sClient client.Client, cfg *cappv1alpha1.CappConfig) HTTPRouteManager {
	return HTTPRouteManager{ResourceManagerClient: newRMClient(k8sClient), EventRecorder: events.NewFakeRecorder(10), CappConfig: cfg, Installed: true, ListenerSetInstalled: true}
}

func newTestClaim(namespace string, labels map[string]string) *netv1alpha1.ClusterDomainClaim {
	return &netv1alpha1.ClusterDomainClaim{
		ObjectMeta: metav1.ObjectMeta{Name: hostnameFQDN, Labels: labels},
		Spec:       netv1alpha1.ClusterDomainClaimSpec{Namespace: namespace},
	}
}

func getClaim(ctx context.Context, t *testing.T, k8sClient client.Client) netv1alpha1.ClusterDomainClaim {
	t.Helper()
	claim := netv1alpha1.ClusterDomainClaim{}
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: hostnameFQDN}, &claim))
	return claim
}

func TestClusterDomainClaimManager(t *testing.T) {
	ctx := context.Background()
	capp := newCappWithHostname(hostnameBare)

	t.Run("claims the hostname for the Capp namespace", func(t *testing.T) {
		k8sClient := newGatewayClient()
		require.NoError(t, newClaimManager(k8sClient, newCappConfigWithGatewayAPI("")).Manage(ctx, capp))

		claim := getClaim(ctx, t, k8sClient)
		require.Equal(t, cappNamespace, claim.Spec.Namespace)
		require.Equal(t, cappName, claim.Labels[cappmeta.CappResourceKey])
		require.Equal(t, cappNamespace, claim.Labels[cappmeta.CappNamespaceKey])
	})

	t.Run("refuses a hostname claimed by another namespace", func(t *testing.T) {
		k8sClient := newGatewayClient(newTestClaim(testOtherCappNamespace, nil))
		err := newClaimManager(k8sClient, newCappConfigWithGatewayAPI("")).Manage(ctx, capp)
		require.ErrorAs(t, err, &HostnameClaimedError{})
		require.Equal(t, testOtherCappNamespace, getClaim(ctx, t, k8sClient).Spec.Namespace)
	})

	t.Run("adopts a claim of the same namespace, such as one Knative created for a DomainMapping", func(t *testing.T) {
		k8sClient := newGatewayClient(newTestClaim(cappNamespace, map[string]string{"keep": "me"}))
		require.NoError(t, newClaimManager(k8sClient, newCappConfigWithGatewayAPI("")).Manage(ctx, capp))

		claim := getClaim(ctx, t, k8sClient)
		require.Equal(t, "me", claim.Labels["keep"])
		require.Equal(t, cappName, claim.Labels[cappmeta.CappResourceKey])
	})

	t.Run("refuses a claim that another Capp of the namespace holds", func(t *testing.T) {
		holder := newBaseCapp()
		holder.Name = "holder"
		k8sClient := newGatewayClient(&holder, newTestClaim(cappNamespace, map[string]string{cappmeta.CappResourceKey: "holder", cappmeta.CappNamespaceKey: cappNamespace}))
		err := newClaimManager(k8sClient, newCappConfigWithGatewayAPI("")).Manage(ctx, capp)

		claimed := HostnameClaimedError{}
		require.ErrorAs(t, err, &claimed)
		require.Equal(t, "holder", claimed.Capp)
		require.Equal(t, "holder", getClaim(ctx, t, k8sClient).Labels[cappmeta.CappResourceKey])
	})

	t.Run("adopts a claim left by a Capp that no longer exists", func(t *testing.T) {
		k8sClient := newGatewayClient(newTestClaim(cappNamespace, map[string]string{cappmeta.CappResourceKey: "gone", cappmeta.CappNamespaceKey: cappNamespace}))
		require.NoError(t, newClaimManager(k8sClient, newCappConfigWithGatewayAPI("")).Manage(ctx, capp))
		require.Equal(t, cappName, getClaim(ctx, t, k8sClient).Labels[cappmeta.CappResourceKey])
	})

	t.Run("deletes the claim of a former hostname after the zone changed", func(t *testing.T) {
		old := newTestClaim(cappNamespace, gatewayAPIManagedLabels(capp))
		old.Name = "my-app.old-zone.com"
		k8sClient := newGatewayClient(old)
		require.NoError(t, newClaimManager(k8sClient, newCappConfigWithGatewayAPI("")).Manage(ctx, capp))
		getClaim(ctx, t, k8sClient)
		require.True(t, errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: old.Name}, &netv1alpha1.ClusterDomainClaim{})))
	})

	t.Run("maps a labelled claim to its Capp for the watch", func(t *testing.T) {
		key, ok := ClaimCappRequest(newTestClaim(cappNamespace, gatewayAPIManagedLabels(capp)))
		require.True(t, ok)
		require.Equal(t, types.NamespacedName{Namespace: cappNamespace, Name: cappName}, key)
		_, ok = ClaimCappRequest(newTestClaim(cappNamespace, nil))
		require.False(t, ok)
	})

	t.Run("leaves the claim in place when Gateway API routing is off", func(t *testing.T) {
		k8sClient := newGatewayClient(newTestClaim(cappNamespace, gatewayAPIManagedLabels(capp)))
		require.NoError(t, newClaimManager(k8sClient, newCappConfigWithDNS()).Manage(ctx, capp))
		getClaim(ctx, t, k8sClient)
	})

	t.Run("cleanup deletes only this Capp's claims", func(t *testing.T) {
		other := newTestClaim(cappNamespace, map[string]string{cappmeta.CappResourceKey: "another-capp"})
		other.Name = "another.capp-zone.com"
		k8sClient := newGatewayClient(newTestClaim(cappNamespace, gatewayAPIManagedLabels(capp)), other)

		require.NoError(t, newClaimManager(k8sClient, newCappConfigWithGatewayAPI("")).CleanUp(ctx, cappWithDeletionTimestamp(capp)))

		err := k8sClient.Get(ctx, types.NamespacedName{Name: hostnameFQDN}, &netv1alpha1.ClusterDomainClaim{})
		require.True(t, errors.IsNotFound(err))
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: other.Name}, &netv1alpha1.ClusterDomainClaim{}))
	})
}

func TestListenerSetManager(t *testing.T) {
	ctx := context.Background()
	capp := newCappWithTLS(hostnameBare, true)
	secretName := generateTLSSecretName(hostnameFQDN)
	ownClaim := func() client.Object { return newTestClaim(cappNamespace, nil) }

	t.Run("waits for the TLS secret", func(t *testing.T) {
		k8sClient := newGatewayClient(ownClaim())
		require.NoError(t, newListenerSetManager(k8sClient, newCappConfigWithGatewayAPI("")).Manage(ctx, capp))

		err := k8sClient.Get(ctx, types.NamespacedName{Name: testGatewayObject, Namespace: cappNamespace}, &gatewayv1.ListenerSet{})
		require.True(t, errors.IsNotFound(err))
	})

	t.Run("creates an HTTPS listener with the Capp certificate on the configured Gateway", func(t *testing.T) {
		k8sClient := newGatewayClient(ownClaim(), newSecret(secretName))
		cfg := newCappConfigWithGatewayAPI(cappv1alpha1.GatewayTLSModeListenerSet)
		cfg.Spec.GatewayAPI.HTTPSPort = 8443
		require.NoError(t, newListenerSetManager(k8sClient, cfg).Manage(ctx, capp))

		ls := gatewayv1.ListenerSet{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: testGatewayObject, Namespace: cappNamespace}, &ls))
		require.Equal(t, gatewayv1.ObjectName(testGatewayName), ls.Spec.ParentRef.Name)
		require.Equal(t, gatewayv1.Namespace(testGatewayNamespace), *ls.Spec.ParentRef.Namespace)
		require.Len(t, ls.Spec.Listeners, 1)
		listener := ls.Spec.Listeners[0]
		require.Equal(t, gatewayv1.Hostname(hostnameFQDN), *listener.Hostname)
		require.Equal(t, gatewayv1.PortNumber(8443), listener.Port)
		require.Equal(t, gatewayv1.HTTPSProtocolType, listener.Protocol)
		require.Equal(t, gatewayv1.ObjectName(secretName), listener.TLS.CertificateRefs[0].Name)
		require.Nil(t, listener.TLS.CertificateRefs[0].Namespace, "the Secret is in the ListenerSet namespace, so no ReferenceGrant is needed")
		require.Len(t, ls.OwnerReferences, 1)
	})

	t.Run("creates nothing for a hostname another namespace holds", func(t *testing.T) {
		k8sClient := newGatewayClient(newTestClaim(testOtherCappNamespace, nil), newSecret(secretName))
		err := newListenerSetManager(k8sClient, newCappConfigWithGatewayAPI("")).Manage(ctx, capp)
		require.ErrorAs(t, err, &HostnameClaimedError{})
	})

	t.Run("is not required in Gateway TLS mode, and the unused secret is deleted", func(t *testing.T) {
		existing := &gatewayv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Name: testGatewayObject, Namespace: cappNamespace, Labels: cappmeta.ManagedResourceLabels(cappName), OwnerReferences: cappOwnerRefs(capp)}}
		k8sClient := newGatewayClient(ownClaim(), newIssuedSecret(), existing)
		mgr := newListenerSetManager(k8sClient, newCappConfigWithGatewayAPI(cappv1alpha1.GatewayTLSModeGateway))
		require.False(t, mgr.IsRequired(capp))
		require.NoError(t, mgr.Manage(ctx, capp))

		require.True(t, errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: testGatewayObject, Namespace: cappNamespace}, &gatewayv1.ListenerSet{})))
		require.True(t, errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: cappNamespace}, &corev1.Secret{})))
	})

	t.Run("keeps the secret in use by a DomainMapping when Gateway API routing is off", func(t *testing.T) {
		k8sClient := newGatewayClient(newSecret(secretName))
		require.NoError(t, newListenerSetManager(k8sClient, newCappConfigWithDNS()).Manage(ctx, capp))
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: cappNamespace}, &corev1.Secret{}))
	})

	t.Run("stops serving a hostname another namespace now holds", func(t *testing.T) {
		existing := &gatewayv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Name: testGatewayObject, Namespace: cappNamespace, Labels: cappmeta.ManagedResourceLabels(cappName), OwnerReferences: cappOwnerRefs(capp)}}
		k8sClient := newGatewayClient(newTestClaim(testOtherCappNamespace, nil), existing)
		require.ErrorAs(t, newListenerSetManager(k8sClient, newCappConfigWithGatewayAPI("")).Manage(ctx, capp), &HostnameClaimedError{})
		require.True(t, errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: testGatewayObject, Namespace: cappNamespace}, &gatewayv1.ListenerSet{})))
	})

	t.Run("leaves a Secret of that name that cert-manager did not issue for the Capp", func(t *testing.T) {
		k8sClient := newGatewayClient(ownClaim(), newSecret(secretName))
		require.NoError(t, newListenerSetManager(k8sClient, newCappConfigWithGatewayAPI(cappv1alpha1.GatewayTLSModeGateway)).Manage(ctx, capp))
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: cappNamespace}, &corev1.Secret{}))
	})

	t.Run("deletes the Certificate before its Secret when the Capp is deleted, so it is not issued again", func(t *testing.T) {
		cert := &cmapi.Certificate{ObjectMeta: metav1.ObjectMeta{Name: hostnameFQDN, Namespace: cappNamespace, OwnerReferences: cappOwnerRefs(capp)}}
		k8sClient := newGatewayClient(ownClaim(), newIssuedSecret(), cert)
		require.NoError(t, newListenerSetManager(k8sClient, newCappConfigWithGatewayAPI("")).CleanUp(ctx, cappWithDeletionTimestamp(capp)))

		require.True(t, errors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(cert), &cmapi.Certificate{})))
		require.True(t, errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: cappNamespace}, &corev1.Secret{})))
	})

	t.Run("reports a missing ListenerSet kind in ListenerSet mode", func(t *testing.T) {
		mgr := newListenerSetManager(newGatewayClient(ownClaim(), newSecret(secretName)), newCappConfigWithGatewayAPI(""))
		mgr.Installed = false
		require.ErrorContains(t, mgr.Manage(ctx, capp), "tlsMode to Gateway")
	})

	t.Run("sends no request when the CRD is not installed", func(t *testing.T) {
		// The scheme lacks the ListenerSet kind, so any List would fail.
		k8sClient := newFakeClient(newScheme())
		mgr := ListenerSetManager{ResourceManagerClient: newRMClient(k8sClient), EventRecorder: events.NewFakeRecorder(10), CappConfig: newCappConfigWithDNS()}
		require.NoError(t, mgr.CleanUp(ctx, capp))
	})
}

func TestHTTPRouteManager(t *testing.T) {
	ctx := context.Background()
	capp := newCappWithTLS(hostnameBare, true)
	ownClaim := func() client.Object { return newTestClaim(cappNamespace, nil) }

	getRoute := func(t *testing.T, k8sClient client.Client) gatewayv1.HTTPRoute {
		t.Helper()
		route := gatewayv1.HTTPRoute{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: testGatewayObject, Namespace: cappNamespace}, &route))
		return route
	}

	t.Run("routes the hostname to the local gateway with the Knative Service's cluster-local host", func(t *testing.T) {
		k8sClient := newGatewayClient(ownClaim())
		require.NoError(t, newHTTPRouteManager(k8sClient, newCappConfigWithGatewayAPI("")).Manage(ctx, capp))

		route := getRoute(t, k8sClient)
		require.Equal(t, []gatewayv1.Hostname{hostnameFQDN}, route.Spec.Hostnames)
		require.Len(t, route.Spec.ParentRefs, 1, "no ListenerSet exists yet, so the route is served over HTTP only")
		require.Equal(t, gatewayv1.ObjectName(testGatewayName), route.Spec.ParentRefs[0].Name)

		rule := route.Spec.Rules[0]
		require.Equal(t, gatewayv1.PreciseHostname(network.GetServiceHostname(cappName, cappNamespace)), *rule.Filters[0].URLRewrite.Hostname)
		require.Equal(t, gatewayv1.HTTPHeaderName(netheader.OriginalHostKey), rule.Filters[1].RequestHeaderModifier.Set[0].Name)
		require.Equal(t, hostnameFQDN, rule.Filters[1].RequestHeaderModifier.Set[0].Value)

		backend := rule.BackendRefs[0].BackendObjectReference
		require.Equal(t, gatewayv1.ObjectName(testLocalGatewayName), backend.Name)
		require.Equal(t, gatewayv1.Namespace(testLocalGatewayNS), *backend.Namespace)
		require.Equal(t, testLocalGatewayPort, *backend.Port)
		require.Equal(t, gatewayv1.Kind("Service"), *backend.Kind, "never the ExternalName Service a DomainMapping uses")
		require.Equal(t, gatewayv1.Duration("0s"), *rule.Timeouts.Request)
	})

	t.Run("attaches to the Capp's ListenerSet once it exists", func(t *testing.T) {
		ls := &gatewayv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Name: testGatewayObject, Namespace: cappNamespace, OwnerReferences: cappOwnerRefs(capp)}}
		k8sClient := newGatewayClient(ownClaim(), ls)
		require.NoError(t, newHTTPRouteManager(k8sClient, newCappConfigWithGatewayAPI("")).Manage(ctx, capp))

		route := getRoute(t, k8sClient)
		require.Len(t, route.Spec.ParentRefs, 2)
		require.Equal(t, gatewayv1.Kind("ListenerSet"), *route.Spec.ParentRefs[1].Kind)
		require.Equal(t, gatewayv1.ObjectName(testGatewayObject), route.Spec.ParentRefs[1].Name)
	})

	t.Run("does not attach to a ListenerSet in Gateway TLS mode", func(t *testing.T) {
		ls := &gatewayv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Name: testGatewayObject, Namespace: cappNamespace, OwnerReferences: cappOwnerRefs(capp)}}
		k8sClient := newGatewayClient(ownClaim(), ls)
		require.NoError(t, newHTTPRouteManager(k8sClient, newCappConfigWithGatewayAPI(cappv1alpha1.GatewayTLSModeGateway)).Manage(ctx, capp))
		require.Len(t, getRoute(t, k8sClient).Spec.ParentRefs, 1)
	})

	t.Run("grants the namespace's routes access to the local gateway Service and removes grants left in other namespaces", func(t *testing.T) {
		stale := &gatewayv1beta1.ReferenceGrant{ObjectMeta: metav1.ObjectMeta{
			Name: referenceGrantName(cappNamespace), Namespace: "old-gateway-ns", Labels: referenceGrantLabels(cappNamespace),
		}}
		k8sClient := newGatewayClient(ownClaim(), stale)
		require.NoError(t, newHTTPRouteManager(k8sClient, newCappConfigWithGatewayAPI("")).Manage(ctx, capp))

		grant := gatewayv1beta1.ReferenceGrant{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: referenceGrantName(cappNamespace), Namespace: testLocalGatewayNS}, &grant))
		require.Equal(t, gatewayv1.Namespace(cappNamespace), grant.Spec.From[0].Namespace)
		require.Equal(t, gatewayv1.Kind("HTTPRoute"), grant.Spec.From[0].Kind)
		require.Equal(t, gatewayv1.ObjectName(testLocalGatewayName), *grant.Spec.To[0].Name)

		err := k8sClient.Get(ctx, types.NamespacedName{Name: referenceGrantName(cappNamespace), Namespace: "old-gateway-ns"}, &gatewayv1beta1.ReferenceGrant{})
		require.True(t, errors.IsNotFound(err))
	})

	t.Run("creates no route for a hostname another namespace holds", func(t *testing.T) {
		k8sClient := newGatewayClient(newTestClaim(testOtherCappNamespace, nil))
		err := newHTTPRouteManager(k8sClient, newCappConfigWithGatewayAPI("")).Manage(ctx, capp)
		require.ErrorAs(t, err, &HostnameClaimedError{})
		require.True(t, errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: testGatewayObject, Namespace: cappNamespace}, &gatewayv1.HTTPRoute{})))
	})

	t.Run("keeps serving after a switch back to DomainMappings until the DomainMapping is Ready", func(t *testing.T) {
		k8sClient := newGatewayClient(ownClaim())
		require.NoError(t, newHTTPRouteManager(k8sClient, newCappConfigWithGatewayAPI("")).Manage(ctx, capp))
		require.NoError(t, newHTTPRouteManager(k8sClient, newCappConfigWithDNS()).Manage(ctx, capp))
		getRoute(t, k8sClient)

		notReady := newReadyDomainMapping()
		notReady.Status.Conditions[0].Status = corev1.ConditionFalse
		require.NoError(t, k8sClient.Create(ctx, notReady))
		require.NoError(t, newHTTPRouteManager(k8sClient, newCappConfigWithDNS()).Manage(ctx, capp))
		getRoute(t, k8sClient)
	})

	t.Run("cleans up the route and the grant once the DomainMapping that replaces them is Ready", func(t *testing.T) {
		k8sClient := newGatewayClient(ownClaim(), newReadyDomainMapping())
		require.NoError(t, newHTTPRouteManager(k8sClient, newCappConfigWithGatewayAPI("")).Manage(ctx, capp))
		require.NoError(t, newHTTPRouteManager(k8sClient, newCappConfigWithDNS()).Manage(ctx, capp))

		require.True(t, errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: testGatewayObject, Namespace: cappNamespace}, &gatewayv1.HTTPRoute{})))
		grants := gatewayv1beta1.ReferenceGrantList{}
		require.NoError(t, k8sClient.List(ctx, &grants))
		require.Empty(t, grants.Items)
	})

	t.Run("leaves net-gateway-api's HTTPRoutes alone, although they carry the Capp labels", func(t *testing.T) {
		// Knative copies the Knative Service's labels onto its HTTPRoutes, and names a DomainMapping's route after
		// the hostname.
		knativeRoute := &gatewayv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{
			Name: hostnameFQDN, Namespace: cappNamespace, Labels: cappmeta.ManagedResourceLabels(cappName),
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "networking.internal.knative.dev/v1alpha1", Kind: "Ingress", Name: hostnameFQDN, UID: "kingress-uid"}},
		}}
		k8sClient := newGatewayClient(ownClaim(), knativeRoute, newReadyDomainMapping())
		require.NoError(t, newHTTPRouteManager(k8sClient, newCappConfigWithGatewayAPI("")).Manage(ctx, capp))
		require.NoError(t, newHTTPRouteManager(k8sClient, newCappConfigWithDNS()).Manage(ctx, capp))

		route := gatewayv1.HTTPRoute{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, &route))
		require.Empty(t, route.Spec.Rules, "the Knative route was neither changed nor deleted")
	})

	t.Run("refuses to take over an HTTPRoute the Capp does not own", func(t *testing.T) {
		foreign := &gatewayv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: testGatewayObject, Namespace: cappNamespace}}
		k8sClient := newGatewayClient(ownClaim(), foreign)
		err := newHTTPRouteManager(k8sClient, newCappConfigWithGatewayAPI("")).Manage(ctx, capp)
		require.ErrorContains(t, err, "is not owned by the Capp")
	})

	t.Run("still serves HTTP when the cluster has no ListenerSet kind", func(t *testing.T) {
		// The scheme lacks ListenerSet, so a ListenerSet lookup would fail.
		s := newScheme()
		utilruntime.Must(gatewayv1beta1.Install(s))
		utilruntime.Must(netv1alpha1.AddToScheme(s))
		utilruntime.Must(gatewayv1.Install(s))
		k8sClient := newFakeClient(s, ownClaim())
		mgr := newHTTPRouteManager(k8sClient, newCappConfigWithGatewayAPI(""))
		mgr.ListenerSetInstalled = false
		require.NoError(t, mgr.Manage(ctx, capp))
		require.Len(t, getRoute(t, k8sClient).Spec.ParentRefs, 1)
	})

	t.Run("writes the backend weight the CRD defaults, and does not update an unchanged route", func(t *testing.T) {
		k8sClient := newGatewayClient(ownClaim())
		mgr := newHTTPRouteManager(k8sClient, newCappConfigWithGatewayAPI(""))
		require.NoError(t, mgr.Manage(ctx, capp))
		route := getRoute(t, k8sClient)
		require.Equal(t, int32(1), *route.Spec.Rules[0].BackendRefs[0].Weight)

		require.NoError(t, mgr.Manage(ctx, capp))
		require.Equal(t, route.ResourceVersion, getRoute(t, k8sClient).ResourceVersion)
	})

	t.Run("stops serving a hostname another namespace now holds", func(t *testing.T) {
		k8sClient := newGatewayClient(newTestClaim(testOtherCappNamespace, nil), newOwnedRoute(capp, testGatewayObject))
		err := newHTTPRouteManager(k8sClient, newCappConfigWithGatewayAPI("")).Manage(ctx, capp)
		require.ErrorAs(t, err, &HostnameClaimedError{})
		require.True(t, errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: testGatewayObject, Namespace: cappNamespace}, &gatewayv1.HTTPRoute{})))
	})

	t.Run("reads the claim from the API server, not the cache", func(t *testing.T) {
		cached := newGatewayClient()
		mgr := newHTTPRouteManager(cached, newCappConfigWithGatewayAPI(""))
		mgr.APIReader = newGatewayClient(ownClaim())
		require.NoError(t, mgr.Manage(ctx, capp))
		getRoute(t, cached)
	})

	t.Run("deletes the route of a former hostname after the zone changed", func(t *testing.T) {
		old := newOwnedRoute(capp, "capp-my-app.old-zone.com")
		k8sClient := newGatewayClient(ownClaim(), old)
		require.NoError(t, newHTTPRouteManager(k8sClient, newCappConfigWithGatewayAPI("")).Manage(ctx, capp))
		getRoute(t, k8sClient)
		require.True(t, errors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(old), &gatewayv1.HTTPRoute{})))
	})

	t.Run("keeps the namespace's grant while another Capp's route uses it", func(t *testing.T) {
		other := newCappWithTLS("other-app", true)
		other.Name, other.UID = "other-capp", "other-uid"
		k8sClient := newGatewayClient(ownClaim(), newOwnedRoute(other, "capp-other-app.capp-zone.com"), newReadyDomainMapping())
		require.NoError(t, newHTTPRouteManager(k8sClient, newCappConfigWithGatewayAPI("")).Manage(ctx, capp))
		require.NoError(t, newHTTPRouteManager(k8sClient, newCappConfigWithGatewayAPI("")).CleanUp(ctx, capp))

		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: referenceGrantName(cappNamespace), Namespace: testLocalGatewayNS}, &gatewayv1beta1.ReferenceGrant{}))
	})

	t.Run("sends no request when the CRDs are not installed", func(t *testing.T) {
		k8sClient := newFakeClient(newScheme())
		mgr := HTTPRouteManager{ResourceManagerClient: newRMClient(k8sClient), EventRecorder: events.NewFakeRecorder(10), CappConfig: newCappConfigWithDNS()}
		require.NoError(t, mgr.Manage(ctx, capp))
	})
}

func TestRoutingModeSelectsManagers(t *testing.T) {
	tlsCapp := newCappWithTLS(hostnameBare, true)

	tests := []struct {
		name              string
		cfg               *cappv1alpha1.CappConfig
		wantDomainMapping bool
		wantCertificate   bool
		wantListenerSet   bool
		wantHTTPRoute     bool
		wantClaim         bool
	}{
		{name: "DomainMappings", cfg: newCappConfigWithDNS(), wantDomainMapping: true, wantCertificate: true},
		{name: "Gateway API with ListenerSets", cfg: newCappConfigWithGatewayAPI(""), wantCertificate: true, wantListenerSet: true, wantHTTPRoute: true, wantClaim: true},
		{name: "Gateway API with a Gateway listener", cfg: newCappConfigWithGatewayAPI(cappv1alpha1.GatewayTLSModeGateway), wantHTTPRoute: true, wantClaim: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := newGatewayClient()
			require.Equal(t, tt.wantDomainMapping, DomainMappingManager{CappConfig: tt.cfg}.IsRequired(tlsCapp))
			require.Equal(t, tt.wantCertificate, CertificateManager{CappConfig: tt.cfg}.IsRequired(tlsCapp))
			require.Equal(t, tt.wantListenerSet, newListenerSetManager(k8sClient, tt.cfg).IsRequired(tlsCapp))
			require.Equal(t, tt.wantHTTPRoute, newHTTPRouteManager(k8sClient, tt.cfg).IsRequired(tlsCapp))
			require.Equal(t, tt.wantClaim, newClaimManager(k8sClient, tt.cfg).IsRequired(tlsCapp))
		})
	}
}

func TestDomainMappingCleanUpKeepsSecretForListenerSet(t *testing.T) {
	ctx := context.Background()
	capp := newCappWithTLS(hostnameBare, true)
	secretName := generateTLSSecretName(hostnameFQDN)
	dm := &knativev1beta1.DomainMapping{ObjectMeta: metav1.ObjectMeta{Name: hostnameFQDN, Namespace: cappNamespace, Labels: cappmeta.ManagedResourceLabels(cappName)}}
	k8sClient := newGatewayClient(dm, newSecret(secretName))

	mgr := DomainMappingManager{ResourceManagerClient: newRMClient(k8sClient), EventRecorder: events.NewFakeRecorder(10), CappConfig: newCappConfigWithGatewayAPI("")}
	require.NoError(t, mgr.Manage(ctx, capp))

	require.True(t, errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, &knativev1beta1.DomainMapping{})))
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: cappNamespace}, &corev1.Secret{}),
		"the ListenerSet serves the certificate after the switch")
}

func TestGatewayObjectName(t *testing.T) {
	require.Equal(t, "capp-"+hostnameFQDN, GatewayObjectName(hostnameFQDN))

	label := strings.Repeat("a", 61)
	long := strings.Join([]string{label, label, label, label}, ".") + ".capp-zone.com"
	other := "b" + long[1:]
	name := GatewayObjectName(long)
	require.Empty(t, validation.IsDNS1123Subdomain(name))
	require.True(t, strings.HasSuffix(name, ".capp-zone.com"))
	require.NotEqual(t, name, GatewayObjectName(other))
}

func TestCertificateManagerGatewayAPI(t *testing.T) {
	ctx := context.Background()
	capp := newCappWithTLS(hostnameBare, true)
	newMgr := func(k8sClient client.Client) CertificateManager {
		return CertificateManager{ResourceManagerClient: newRMClient(k8sClient), EventRecorder: events.NewFakeRecorder(10), CappConfig: newCappConfigWithGatewayAPI("")}
	}

	t.Run("issues no certificate for a hostname another namespace holds", func(t *testing.T) {
		k8sClient := newGatewayClient(newTestClaim(testOtherCappNamespace, nil))
		require.ErrorAs(t, newMgr(k8sClient).Manage(ctx, capp), &HostnameClaimedError{})
		require.True(t, errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, &cmapi.Certificate{})))
	})

	t.Run("issues the certificate once the hostname is claimed, with the Capp labels on its Secret", func(t *testing.T) {
		k8sClient := newGatewayClient(newTestClaim(cappNamespace, nil))
		require.NoError(t, newMgr(k8sClient).Manage(ctx, capp))

		cert := cmapi.Certificate{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, &cert))
		require.Equal(t, cappName, cert.Spec.SecretTemplate.Labels[cappmeta.CappResourceKey])
		require.Contains(t, cert.Spec.SecretTemplate.Labels, certificateUIDSecretLabelKey)
	})
}
