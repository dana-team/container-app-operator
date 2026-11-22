package resourcemanagers

import (
	"context"
	"fmt"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	netheader "knative.dev/networking/pkg/http/header"
	"knative.dev/pkg/network"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const (
	HTTPRoute                        = "HTTPRoute"
	ReferenceGrant                   = "ReferenceGrant"
	eventCappHTTPRouteCreationFailed = "HTTPRouteCreationFailed"
	eventCappHTTPRouteCreated        = "HTTPRouteCreated"

	// noRequestTimeout disables the request timeout of the outer hop. The Knative route on the local gateway
	// applies the Capp's own timeout, as net-gateway-api does for the routes it creates.
	noRequestTimeout = "0s"

	// backendWeight is written explicitly because the CRD defaults it to 1: a route built without it would never
	// equal the stored one, and would be updated on every reconcile.
	backendWeight int32 = 1
)

// HTTPRouteManager routes a Capp hostname without a DomainMapping. A DomainMapping's backend is the Knative
// Service's ExternalName Service, which points at Knative's local gateway. Gateway API implementations such as
// Envoy Gateway refuse ExternalName backends, so the HTTPRoute points at the local gateway Service directly and
// rewrites the Host header to the Knative Service's cluster-local name: the same hops a DomainMapping takes.
// Traffic splits, tags, the activator and scale from zero all stay with Knative's own route on the local gateway.
type HTTPRouteManager struct {
	rclient.ResourceManagerClient
	EventRecorder events.EventRecorder
	CappConfig    *cappv1alpha1.CappConfig
	// APIReader reads the hostname claim from the API server. When nil, K8sClient is used.
	APIReader client.Reader
	// Installed reports whether the HTTPRoute and ReferenceGrant CRDs were installed when the operator started.
	Installed bool
	// ListenerSetInstalled reports whether the ListenerSet CRD was installed when the operator started. Without it
	// the route is still created, attached to the Gateway only, so the hostname is served over HTTP.
	ListenerSetInstalled bool
}

// IsRequired reports whether the Capp's hostname is routed by an operator-managed HTTPRoute.
func (h HTTPRouteManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return IsGatewayAPIRouting(h.CappConfig) && capp.Spec.RouteSpec.Hostname != ""
}

// Manage creates or updates the HTTPRoute and the namespace's ReferenceGrant, or cleans them up when not required.
// When Gateway API routing is turned off, the route keeps serving until the DomainMapping that replaces it is Ready.
func (h HTTPRouteManager) Manage(ctx context.Context, capp cappv1alpha1.Capp) error {
	if !h.IsRequired(capp) {
		if !h.Installed {
			return nil
		}
		replaced, err := replacedByDomainMapping(ctx, h.K8sClient, h.CappConfig, capp)
		if err != nil || !replaced {
			return err
		}
		return h.CleanUp(ctx, capp)
	}

	hostname := cappHostname(h.CappConfig, capp)
	if err := requireHostnameClaim(ctx, readerOrClient(h.APIReader, h.K8sClient), hostname, capp); err != nil {
		if IsHostnameClaimedError(err) {
			// Stop serving a hostname this Capp no longer holds, so the route cannot shadow the holder's.
			if cleanupErr := h.deleteRoutes(ctx, capp, ""); cleanupErr != nil {
				return cleanupErr
			}
		}
		return err
	}

	if err := h.ensureReferenceGrant(ctx, capp); err != nil {
		return err
	}

	attachToListenerSet, err := h.listenerSetExists(ctx, capp, hostname)
	if err != nil {
		return err
	}
	desired := h.prepareResource(capp, hostname, attachToListenerSet)
	if err := createOrUpdateOwned(ctx, h.ResourceManagerClient, h.EventRecorder, capp, &desired, &gatewayv1.HTTPRoute{},
		HTTPRoute, eventCappHTTPRouteCreated, eventCappHTTPRouteCreationFailed,
		func(existing, desired *gatewayv1.HTTPRoute) bool {
			if equality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
				return false
			}
			existing.Spec = desired.Spec
			return true
		}); err != nil {
		return err
	}
	// Routes for a former hostname of this Capp, for example after the DNS zone changed.
	return h.deleteRoutes(ctx, capp, desired.Name)
}

// listenerSetExists reports whether the Capp's ListenerSet exists. The route attaches to it only then, so a
// Capp whose certificate is not issued yet is still served over HTTP.
func (h HTTPRouteManager) listenerSetExists(ctx context.Context, capp cappv1alpha1.Capp, hostname string) (bool, error) {
	if !UsesListenerSetTLS(h.CappConfig, capp) || !h.ListenerSetInstalled {
		return false, nil
	}
	ls := gatewayv1.ListenerSet{}
	name := GatewayObjectName(hostname)
	if err := h.K8sClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: name}, &ls); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get ListenerSet %q: %w", name, err)
	}
	return isOwnedBy(&ls, capp), nil
}

func (h HTTPRouteManager) prepareResource(capp cappv1alpha1.Capp, hostname string, attachToListenerSet bool) gatewayv1.HTTPRoute {
	cfg := gatewayAPIConfig(h.CappConfig)

	parentRefs := []gatewayv1.ParentReference{{
		Group:     ptr.To(gatewayv1.Group(gatewayAPIGroup)),
		Kind:      ptr.To(gatewayv1.Kind(gatewayKind)),
		Namespace: ptr.To(gatewayv1.Namespace(cfg.Gateway.Namespace)),
		Name:      gatewayv1.ObjectName(cfg.Gateway.Name),
	}}
	if attachToListenerSet {
		parentRefs = append(parentRefs, gatewayv1.ParentReference{
			Group: ptr.To(gatewayv1.Group(gatewayAPIGroup)),
			Kind:  ptr.To(gatewayv1.Kind(listenerSetKind)),
			Name:  gatewayv1.ObjectName(GatewayObjectName(hostname)),
		})
	}

	return gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GatewayObjectName(hostname),
			Namespace: capp.Namespace,
			Labels:    cappmeta.ManagedResourceLabels(capp.Name),
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{ParentRefs: parentRefs},
			Hostnames:       []gatewayv1.Hostname{gatewayv1.Hostname(hostname)},
			Rules: []gatewayv1.HTTPRouteRule{{
				Matches: []gatewayv1.HTTPRouteMatch{{
					Path: &gatewayv1.HTTPPathMatch{
						Type:  ptr.To(gatewayv1.PathMatchPathPrefix),
						Value: ptr.To("/"),
					},
				}},
				Filters: []gatewayv1.HTTPRouteFilter{
					{
						Type: gatewayv1.HTTPRouteFilterURLRewrite,
						URLRewrite: &gatewayv1.HTTPURLRewriteFilter{
							Hostname: ptr.To(gatewayv1.PreciseHostname(network.GetServiceHostname(capp.Name, capp.Namespace))),
						},
					},
					{
						// A DomainMapping sets the same header, so the application sees the same request.
						Type: gatewayv1.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gatewayv1.HTTPHeaderFilter{
							Set: []gatewayv1.HTTPHeader{{Name: gatewayv1.HTTPHeaderName(netheader.OriginalHostKey), Value: hostname}},
						},
					},
				},
				BackendRefs: []gatewayv1.HTTPBackendRef{{
					BackendRef: gatewayv1.BackendRef{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Group:     ptr.To(gatewayv1.Group("")),
							Kind:      ptr.To(gatewayv1.Kind(serviceKind)),
							Name:      gatewayv1.ObjectName(cfg.LocalGatewayService.Name),
							Namespace: ptr.To(gatewayv1.Namespace(cfg.LocalGatewayService.Namespace)),
							Port:      ptr.To(localGatewayPort(cfg)),
						},
						Weight: ptr.To(backendWeight),
					},
				}},
				Timeouts: &gatewayv1.HTTPRouteTimeouts{Request: ptr.To(gatewayv1.Duration(noRequestTimeout))},
			}},
		},
	}
}

// deleteRoutes deletes the Capp's HTTPRoutes except the one named keep (empty deletes all). Knative copies the
// Knative Service's labels, and so the Capp labels, onto the HTTPRoutes net-gateway-api creates; only routes with an
// owner reference to the Capp are the operator's.
func (h HTTPRouteManager) deleteRoutes(ctx context.Context, capp cappv1alpha1.Capp, keep string) error {
	routes := gatewayv1.HTTPRouteList{}
	if err := listManagedResources(ctx, h.K8sClient, capp, &routes, HTTPRoute, nil); err != nil {
		return err
	}
	return deleteOwnedExcept(ctx, h.K8sClient, capp, itemPointers(routes.Items), keep, HTTPRoute)
}

// referenceGrantName names the ReferenceGrant that lets the HTTPRoutes of one Capp namespace reference the local
// gateway Service. Namespace names cannot contain dots, so the name is unique per namespace.
func referenceGrantName(namespace string) string {
	return "capp-routes." + namespace
}

// referenceGrantLabels select the grant of one Capp namespace, wherever the local gateway Service lives.
func referenceGrantLabels(namespace string) map[string]string {
	return map[string]string{
		cappmeta.ManagedByLabelKey: cappmeta.CappKey,
		cappmeta.CappNamespaceKey:  namespace,
	}
}

func (h HTTPRouteManager) prepareReferenceGrant(namespace string) gatewayv1beta1.ReferenceGrant {
	cfg := gatewayAPIConfig(h.CappConfig)
	return gatewayv1beta1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      referenceGrantName(namespace),
			Namespace: cfg.LocalGatewayService.Namespace,
			Labels:    referenceGrantLabels(namespace),
		},
		Spec: gatewayv1beta1.ReferenceGrantSpec{
			From: []gatewayv1beta1.ReferenceGrantFrom{{
				Group:     gatewayAPIGroup,
				Kind:      httpRouteKind,
				Namespace: gatewayv1.Namespace(namespace),
			}},
			To: []gatewayv1beta1.ReferenceGrantTo{{
				Group: "",
				Kind:  serviceKind,
				Name:  ptr.To(gatewayv1.ObjectName(cfg.LocalGatewayService.Name)),
			}},
		},
	}
}

// ensureReferenceGrant keeps one grant per Capp namespace in the local gateway Service's namespace, shared by all
// Capps of that namespace. The grant lives outside the Capp namespace, so it has no owner reference. A grant of this
// namespace left in another namespace, for example after localGatewayService moved, is deleted here.
func (h HTTPRouteManager) ensureReferenceGrant(ctx context.Context, capp cappv1alpha1.Capp) error {
	desired := h.prepareReferenceGrant(capp.Namespace)

	existing := gatewayv1beta1.ReferenceGrant{}
	err := h.K8sClient.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, &existing)
	switch {
	case errors.IsNotFound(err):
		if err := h.CreateResource(ctx, &desired); err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create ReferenceGrant %s/%s: %w", desired.Namespace, desired.Name, err)
		}
	case err != nil:
		return fmt.Errorf("failed to get ReferenceGrant %s/%s: %w", desired.Namespace, desired.Name, err)
	case !equality.Semantic.DeepEqual(existing.Spec, desired.Spec) || !equality.Semantic.DeepEqual(existing.Labels, desired.Labels):
		existing.Spec = desired.Spec
		existing.Labels = desired.Labels
		if err := h.UpdateResource(ctx, &existing); err != nil {
			return fmt.Errorf("failed to update ReferenceGrant %s/%s: %w", desired.Namespace, desired.Name, err)
		}
	}

	return h.deleteReferenceGrants(ctx, capp.Namespace, func(grant gatewayv1beta1.ReferenceGrant) bool {
		return grant.Namespace != desired.Namespace || grant.Name != desired.Name
	})
}

func (h HTTPRouteManager) deleteReferenceGrants(ctx context.Context, namespace string, shouldDelete func(gatewayv1beta1.ReferenceGrant) bool) error {
	grants := gatewayv1beta1.ReferenceGrantList{}
	if err := h.K8sClient.List(ctx, &grants, client.MatchingLabels(referenceGrantLabels(namespace))); err != nil {
		return fmt.Errorf("unable to list %ss of namespace %q: %w", ReferenceGrant, namespace, err)
	}
	for i := range grants.Items {
		if !shouldDelete(grants.Items[i]) {
			continue
		}
		if err := client.IgnoreNotFound(h.K8sClient.Delete(ctx, &grants.Items[i])); err != nil {
			return fmt.Errorf("failed to delete %s %s/%s: %w", ReferenceGrant, grants.Items[i].Namespace, grants.Items[i].Name, err)
		}
	}
	return nil
}

// otherCappRoutesInNamespace reports whether an operator HTTPRoute of another Capp exists in the namespace, which
// still needs the namespace's grant. A route created concurrently may be missed; its Capp then sees ResolvedRefs
// fail through the HTTPRoute watch and creates the grant again.
func (h HTTPRouteManager) otherCappRoutesInNamespace(ctx context.Context, capp cappv1alpha1.Capp) (bool, error) {
	routes := gatewayv1.HTTPRouteList{}
	if err := h.K8sClient.List(ctx, &routes, client.InNamespace(capp.Namespace),
		client.MatchingLabels{cappmeta.ManagedByLabelKey: cappmeta.CappKey}); err != nil {
		return false, fmt.Errorf("unable to list %ss of namespace %q: %w", HTTPRoute, capp.Namespace, err)
	}
	for i := range routes.Items {
		route := &routes.Items[i]
		if IsOwnedByAnyCapp(route) && !isOwnedBy(route, capp) {
			return true, nil
		}
	}
	return false, nil
}

// CleanUp deletes the Capp's HTTPRoutes, and the namespace's ReferenceGrant when no other Capp's route needs it.
// When the CRDs were not installed at startup there is nothing to delete, and no request is sent for kinds the API
// server lacks.
func (h HTTPRouteManager) CleanUp(ctx context.Context, capp cappv1alpha1.Capp) error {
	if !h.Installed {
		return nil
	}
	if err := h.deleteRoutes(ctx, capp, ""); err != nil {
		return err
	}

	needed, err := h.otherCappRoutesInNamespace(ctx, capp)
	if err != nil || needed {
		return err
	}
	return h.deleteReferenceGrants(ctx, capp.Namespace, func(gatewayv1beta1.ReferenceGrant) bool { return true })
}
