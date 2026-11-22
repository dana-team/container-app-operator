package status

import (
	"context"
	"fmt"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"k8s.io/apimachinery/pkg/api/meta"
	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	dnsrecordv1alpha1 "github.com/dana-team/provider-dns-v2/apis/namespaced/record/v1alpha1"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// buildRouteStatus constructs the Route Status of the Capp object in accordance to the
// status of the corresponding DomainMapping, DNSRecord and Certificate objects if such exist.
func buildRouteStatus(ctx context.Context, kubeClient client.Client, capp cappv1alpha1.Capp, isRequired map[string]bool, cappConfig *cappv1alpha1.CappConfig) (cappv1alpha1.RouteStatus, error) {
	routeStatus := cappv1alpha1.RouteStatus{}
	dnsConfig := cappConfig.Spec.DNSConfig

	domainMappingStatus, err := buildDomainMappingStatus(ctx, kubeClient, capp, isRequired[rmanagers.DomainMapping], dnsConfig.Zone)
	if err != nil {
		return routeStatus, err
	}

	dnsRecordStatus, err := buildDNSRecordStatus(ctx, kubeClient, capp, isRequired[rmanagers.DNSRecord], dnsConfig.Zone)
	if err != nil {
		return routeStatus, err
	}

	certificateStatus, err := buildCertificateStatus(ctx, kubeClient, capp, isRequired[rmanagers.Certificate], dnsConfig.Zone)
	if err != nil {
		return routeStatus, err
	}

	routeStatus.DomainMappingObjectStatus = domainMappingStatus
	routeStatus.DNSRecordObjectStatus = dnsRecordStatus
	routeStatus.CertificateObjectStatus = certificateStatus

	return routeStatus, nil
}

// buildDomainMappingStatus partly constructs the Route Status of the Capp object in accordance to the
// status of the corresponding DomainMapping object.
func buildDomainMappingStatus(ctx context.Context, kubeClient client.Client, capp cappv1alpha1.Capp, isRequired bool, zone string) (knativev1beta1.DomainMappingStatus, error) {
	if !isRequired {
		return knativev1beta1.DomainMappingStatus{}, nil
	}

	domainMapping := &knativev1beta1.DomainMapping{}
	domainMappingName := rmanagers.GenerateResourceName(capp.Spec.RouteSpec.Hostname, zone)
	if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: domainMappingName}, domainMapping); err != nil {
		if apierrors.IsNotFound(err) {
			return knativev1beta1.DomainMappingStatus{}, nil
		}
		return knativev1beta1.DomainMappingStatus{}, err
	}

	return domainMapping.Status, nil
}

// buildCertificateStatus partly constructs the Route Status of the Capp object in accordance to the
// status of the corresponding Certificate object.
func buildCertificateStatus(ctx context.Context, kubeClient client.Client, capp cappv1alpha1.Capp, isRequired bool, zone string) (cmapi.CertificateStatus, error) {
	if !isRequired {
		return cmapi.CertificateStatus{}, nil
	}

	certificate := &cmapi.Certificate{}
	certificateName := rmanagers.GenerateResourceName(capp.Spec.RouteSpec.Hostname, zone)

	if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: certificateName}, certificate); err != nil {
		if apierrors.IsNotFound(err) {
			return cmapi.CertificateStatus{}, nil
		}
		return cmapi.CertificateStatus{}, err
	}

	return certificate.Status, nil
}

// buildDNSRecordStatus partly constructs the Route Status of the Capp object in accordance to the
// status of the corresponding DNSRecord object.
func buildDNSRecordStatus(ctx context.Context, kubeClient client.Client, capp cappv1alpha1.Capp, isRequired bool, zone string) (cappv1alpha1.DNSRecordObjectStatus, error) {
	dnsStatus := cappv1alpha1.DNSRecordObjectStatus{}
	var err error

	if !isRequired {
		return dnsStatus, nil
	}

	dnsStatus.CNAMERecordObjectStatus, err = buildCNAMERecordStatus(ctx, kubeClient, capp, zone)
	if err != nil {
		return dnsStatus, err
	}

	return dnsStatus, nil
}

// buildCNAMERecordStatus partly constructs the Route Status of the Capp object in accordance to the
// status of the corresponding CNAMERecord object.
func buildCNAMERecordStatus(ctx context.Context, kubeClient client.Client, capp cappv1alpha1.Capp, zone string) (dnsrecordv1alpha1.CNAMERecordStatus, error) {
	cnameRecord := &dnsrecordv1alpha1.CNAMERecord{}
	cnameRecordName := rmanagers.GenerateResourceName(capp.Spec.RouteSpec.Hostname, zone)
	if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: cnameRecordName}, cnameRecord); err != nil {
		if apierrors.IsNotFound(err) {
			return dnsrecordv1alpha1.CNAMERecordStatus{}, nil
		}
		return dnsrecordv1alpha1.CNAMERecordStatus{}, err
	}

	return cnameRecord.Status, nil
}

// cnameRecordConditionTypes are checked in order: Synced reports provider errors, Ready reports availability.
var cnameRecordConditionTypes = []xpv2.ConditionType{xpv2.TypeSynced, xpv2.TypeReady}

// checkRoute returns why the custom hostname is not ready, or nil when it is.
// The CNAMERecord, Certificate (when TLS is enabled) and DomainMapping are checked in that order.
func checkRoute(ctx context.Context, k8sClient client.Client, capp cappv1alpha1.Capp, cappConfig *cappv1alpha1.CappConfig) (*metav1.Condition, error) {
	zone := cappConfig.Spec.DNSConfig.Zone
	key := types.NamespacedName{Namespace: capp.Namespace, Name: rmanagers.GenerateResourceName(capp.Spec.RouteSpec.Hostname, zone)}

	if cond, err := checkCNAMERecord(ctx, k8sClient, key); cond != nil || err != nil {
		return cond, err
	}

	if !rmanagers.IsGatewayAPIRouting(cappConfig) {
		if capp.Spec.RouteSpec.TlsEnabled {
			if cond, err := checkCertificate(ctx, k8sClient, key); cond != nil || err != nil {
				return cond, err
			}
		}
		return checkDomainMapping(ctx, k8sClient, key)
	}

	// Gateway API routing: the claim, then the per-Capp certificate and listener when used, then the route.
	if cond, err := checkClusterDomainClaim(ctx, k8sClient, key.Name, capp); cond != nil || err != nil {
		return cond, err
	}
	gatewayKey := types.NamespacedName{Namespace: capp.Namespace, Name: rmanagers.GatewayObjectName(key.Name)}
	listenerSetTLS := rmanagers.UsesListenerSetTLS(cappConfig, capp)
	if listenerSetTLS {
		if cond, err := checkCertificate(ctx, k8sClient, key); cond != nil || err != nil {
			return cond, err
		}
		if cond, err := checkListenerSet(ctx, k8sClient, gatewayKey); cond != nil || err != nil {
			return cond, err
		}
	}
	return checkHTTPRoute(ctx, k8sClient, gatewayKey, cappConfig.Spec.GatewayAPI.Gateway, listenerSetTLS)
}

// checkClusterDomainClaim returns why the hostname is not claimed by the Capp, or nil when it is.
func checkClusterDomainClaim(ctx context.Context, k8sClient client.Client, hostname string, capp cappv1alpha1.Capp) (*metav1.Condition, error) {
	claim := &netv1alpha1.ClusterDomainClaim{}
	if cond, err := getChildResource(ctx, k8sClient, rmanagers.ClusterDomainClaim, types.NamespacedName{Name: hostname}, claim); cond != nil || err != nil {
		return cond, err
	}
	notReady := func(message string) *metav1.Condition {
		return &metav1.Condition{
			Status:  metav1.ConditionFalse,
			Reason:  cappv1alpha1.CapabilityReasonNotReady,
			Message: truncateConditionMessage(fmt.Sprintf("%s %s: %s", rmanagers.ClusterDomainClaim, hostname, message)),
		}
	}
	if claim.Spec.Namespace != capp.Namespace {
		return notReady(fmt.Sprintf("hostname is claimed by namespace %q", claim.Spec.Namespace)), nil
	}
	if owner := claim.Labels[cappmeta.CappResourceKey]; owner != "" && owner != capp.Name {
		return notReady(fmt.Sprintf("hostname is claimed by Capp %q", owner)), nil
	}
	return nil, nil
}

// listenerSetConditionTypes are checked in order: Accepted reports attachment to the Gateway, Programmed reports serving.
var listenerSetConditionTypes = []string{string(gatewayv1.ListenerSetConditionAccepted), string(gatewayv1.ListenerSetConditionProgrammed)}

// checkListenerSet returns why the ListenerSet is not attached and programmed, or nil when it is.
func checkListenerSet(ctx context.Context, k8sClient client.Client, key types.NamespacedName) (*metav1.Condition, error) {
	ls := &gatewayv1.ListenerSet{}
	if cond, err := getChildResource(ctx, k8sClient, rmanagers.ListenerSet, key, ls); cond != nil || err != nil {
		return cond, err
	}
	for _, conditionType := range listenerSetConditionTypes {
		c := meta.FindStatusCondition(ls.Status.Conditions, conditionType)
		if c == nil || isConditionStale(c.ObservedGeneration, ls) {
			return newPendingCondition(rmanagers.ListenerSet, ls.Name), nil
		}
		if cond := formatChildCondition(rmanagers.ListenerSet, ls.Name, c.Status, c.Reason, c.Message); cond != nil {
			return cond, nil
		}
	}
	return nil, nil
}

// routeParentConditionTypes are checked in order for each parent of the HTTPRoute.
var routeParentConditionTypes = []string{string(gatewayv1.RouteConditionAccepted), string(gatewayv1.RouteConditionResolvedRefs)}

// checkHTTPRoute returns why the HTTPRoute is not accepted by the Gateway (and the ListenerSet, when used)
// or cannot resolve its backend, or nil when it serves.
func checkHTTPRoute(ctx context.Context, k8sClient client.Client, key types.NamespacedName, gateway cappv1alpha1.NamespacedName, viaListenerSet bool) (*metav1.Condition, error) {
	route := &gatewayv1.HTTPRoute{}
	if cond, err := getChildResource(ctx, k8sClient, rmanagers.HTTPRoute, key, route); cond != nil || err != nil {
		return cond, err
	}

	parents := []types.NamespacedName{{Namespace: gateway.Namespace, Name: gateway.Name}}
	if viaListenerSet {
		if !routeHasListenerSetParent(route, key.Name) {
			// The route is attached to the ListenerSet on the next reconcile after the ListenerSet exists.
			return newPendingCondition(rmanagers.HTTPRoute, route.Name), nil
		}
		parents = append(parents, key)
	}

	for _, parent := range parents {
		status := findRouteParentStatus(route, parent)
		if status == nil {
			return newPendingCondition(rmanagers.HTTPRoute, route.Name), nil
		}
		for _, conditionType := range routeParentConditionTypes {
			c := meta.FindStatusCondition(status.Conditions, conditionType)
			if c == nil || isConditionStale(c.ObservedGeneration, route) {
				return newPendingCondition(rmanagers.HTTPRoute, route.Name), nil
			}
			if cond := formatChildCondition(rmanagers.HTTPRoute, route.Name, c.Status, c.Reason, c.Message); cond != nil {
				return cond, nil
			}
		}
	}
	return nil, nil
}

func routeHasListenerSetParent(route *gatewayv1.HTTPRoute, name string) bool {
	for _, ref := range route.Spec.ParentRefs {
		if ref.Kind != nil && string(*ref.Kind) == rmanagers.ListenerSet && string(ref.Name) == name {
			return true
		}
	}
	return false
}

// findRouteParentStatus returns the status the implementation reported for parent, matched by name and namespace.
// A parentRef without a namespace means the route's namespace.
func findRouteParentStatus(route *gatewayv1.HTTPRoute, parent types.NamespacedName) *gatewayv1.RouteParentStatus {
	for i := range route.Status.Parents {
		ref := route.Status.Parents[i].ParentRef
		namespace := route.Namespace
		if ref.Namespace != nil {
			namespace = string(*ref.Namespace)
		}
		if string(ref.Name) == parent.Name && namespace == parent.Namespace {
			return &route.Status.Parents[i]
		}
	}
	return nil
}

// checkCNAMERecord returns why the CNAMERecord is not ready, or nil when it is.
func checkCNAMERecord(ctx context.Context, k8sClient client.Client, key types.NamespacedName) (*metav1.Condition, error) {
	record := &dnsrecordv1alpha1.CNAMERecord{}
	if cond, err := getChildResource(ctx, k8sClient, dnsrecordv1alpha1.CNAMERecord_Kind, key, record); cond != nil || err != nil {
		return cond, err
	}

	for _, conditionType := range cnameRecordConditionTypes {
		rc := record.Status.GetCondition(conditionType)
		if isConditionStale(rc.ObservedGeneration, record) {
			return newPendingCondition(dnsrecordv1alpha1.CNAMERecord_Kind, record.Name), nil
		}
		if cond := formatChildCondition(dnsrecordv1alpha1.CNAMERecord_Kind, record.Name, metav1.ConditionStatus(rc.Status), string(rc.Reason), rc.Message); cond != nil {
			return cond, nil
		}
	}
	return nil, nil
}

// checkCertificate returns why the Certificate is not ready, or nil when it is.
func checkCertificate(ctx context.Context, k8sClient client.Client, key types.NamespacedName) (*metav1.Condition, error) {
	cert := &cmapi.Certificate{}
	if cond, err := getChildResource(ctx, k8sClient, rmanagers.Certificate, key, cert); cond != nil || err != nil {
		return cond, err
	}

	var ready *cmapi.CertificateCondition
	for i := range cert.Status.Conditions {
		if cert.Status.Conditions[i].Type == cmapi.CertificateConditionReady {
			ready = &cert.Status.Conditions[i]
		}
	}
	if ready == nil || isConditionStale(ready.ObservedGeneration, cert) {
		return newPendingCondition(rmanagers.Certificate, cert.Name), nil
	}
	return formatChildCondition(rmanagers.Certificate, cert.Name, metav1.ConditionStatus(ready.Status), ready.Reason, ready.Message), nil
}

// checkDomainMapping returns why the DomainMapping is not ready, or nil when it is.
func checkDomainMapping(ctx context.Context, k8sClient client.Client, key types.NamespacedName) (*metav1.Condition, error) {
	dm := &knativev1beta1.DomainMapping{}
	if cond, err := getChildResource(ctx, k8sClient, rmanagers.DomainMapping, key, dm); cond != nil || err != nil {
		return cond, err
	}

	ready := dm.Status.GetCondition(knativev1beta1.DomainMappingConditionReady)
	if ready == nil || dm.Status.ObservedGeneration < dm.Generation {
		return newPendingCondition(rmanagers.DomainMapping, dm.Name), nil
	}
	return formatChildCondition(rmanagers.DomainMapping, dm.Name, metav1.ConditionStatus(ready.Status), ready.Reason, ready.Message), nil
}
