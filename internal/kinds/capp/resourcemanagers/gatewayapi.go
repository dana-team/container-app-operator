package resourcemanagers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	stderrors "errors"
	"fmt"
	"slices"
	"strings"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/tools/events"
	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultHTTPSPort             int32 = 443
	defaultLocalGatewayPort      int32 = 80
	gatewayAPIGroup                    = "gateway.networking.k8s.io"
	gatewayKind                        = "Gateway"
	listenerSetKind                    = "ListenerSet"
	httpRouteKind                      = "HTTPRoute"
	serviceKind                        = "Service"
	listenerSetHTTPSListenerName       = "https"
	cappKind                           = "Capp"
)

// gatewayAPIConfig returns the Gateway API routing settings, or nil when hostnames are routed through DomainMappings.
func gatewayAPIConfig(cappConfig *cappv1alpha1.CappConfig) *cappv1alpha1.GatewayAPIConfig {
	if cappConfig == nil {
		return nil
	}
	return cappConfig.Spec.GatewayAPI
}

// IsGatewayAPIRouting reports whether Capp hostnames are routed through operator-managed Gateway API objects.
func IsGatewayAPIRouting(cappConfig *cappv1alpha1.CappConfig) bool {
	return gatewayAPIConfig(cappConfig) != nil
}

// gatewayTLSMode returns the configured TLS mode, defaulting to ListenerSet.
func gatewayTLSMode(cfg *cappv1alpha1.GatewayAPIConfig) cappv1alpha1.GatewayTLSMode {
	if cfg == nil || cfg.TLSMode == "" {
		return cappv1alpha1.GatewayTLSModeListenerSet
	}
	return cfg.TLSMode
}

// UsesListenerSetTLS reports whether a Capp's HTTPS is served by a per-Capp Certificate and ListenerSet.
func UsesListenerSetTLS(cappConfig *cappv1alpha1.CappConfig, capp cappv1alpha1.Capp) bool {
	cfg := gatewayAPIConfig(cappConfig)
	return cfg != nil && capp.Spec.RouteSpec.Hostname != "" && capp.Spec.RouteSpec.TlsEnabled &&
		gatewayTLSMode(cfg) == cappv1alpha1.GatewayTLSModeListenerSet
}

func httpsPort(cfg *cappv1alpha1.GatewayAPIConfig) int32 {
	if cfg.HTTPSPort == 0 {
		return defaultHTTPSPort
	}
	return cfg.HTTPSPort
}

func localGatewayPort(cfg *cappv1alpha1.GatewayAPIConfig) int32 {
	if cfg.LocalGatewayService.Port == 0 {
		return defaultLocalGatewayPort
	}
	return cfg.LocalGatewayService.Port
}

// cappHostname returns the Capp's fully qualified hostname, or "" when it has none.
func cappHostname(cappConfig *cappv1alpha1.CappConfig, capp cappv1alpha1.Capp) string {
	if capp.Spec.RouteSpec.Hostname == "" {
		return ""
	}
	return GenerateResourceName(capp.Spec.RouteSpec.Hostname, cappConfig.Spec.DNSConfig.Zone)
}

// gatewayObjectNamePrefix keeps the operator's HTTPRoute and ListenerSet apart from net-gateway-api's objects.
// net-gateway-api names the HTTPRoute of a DomainMapping after the hostname, in the same namespace, so without
// the prefix the two would be one object while a cluster switches from DomainMappings.
const gatewayObjectNamePrefix = "capp-"

// GatewayObjectName returns the name of the HTTPRoute and ListenerSet for a Capp hostname.
func GatewayObjectName(hostname string) string {
	name := gatewayObjectNamePrefix + hostname
	if len(name) <= validation.DNS1123SubdomainMaxLength {
		return name
	}
	// A hostname is at most 253 characters, so only the longest ones need this. Keep the end of the hostname,
	// which is the zone, and add a hash of the whole hostname so the name stays unique.
	sum := sha256.Sum256([]byte(hostname))
	hash := hex.EncodeToString(sum[:])[:16]
	keep := validation.DNS1123SubdomainMaxLength - len(gatewayObjectNamePrefix) - len(hash) - 1
	return gatewayObjectNamePrefix + hash + "." + strings.TrimLeft(hostname[len(hostname)-keep:], ".-")
}

// isOwnedBy reports whether obj has an owner reference to capp.
func isOwnedBy(obj metav1.Object, capp cappv1alpha1.Capp) bool {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.UID == capp.UID {
			return true
		}
	}
	return false
}

// IsOwnedByAnyCapp reports whether obj has an owner reference to a Capp. Knative copies the Capp labels onto the
// HTTPRoutes it creates, so labels alone do not tell the operator's objects apart.
func IsOwnedByAnyCapp(obj metav1.Object) bool {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.Kind == cappKind && strings.HasPrefix(ref.APIVersion, cappv1alpha1.GroupVersion.Group+"/") {
			return true
		}
	}
	return false
}

// errNotManaged is returned when an object with the name the operator needs exists but does not belong to the Capp.
func errNotManaged(kind, namespace, name string) error {
	return fmt.Errorf("%s %s/%s exists and is not owned by the Capp; delete or rename it", kind, namespace, name)
}

// gatewayAPIManagedLabels labels objects that may live outside the Capp namespace (or are cluster scoped),
// so they can be found by Capp name and namespace.
func gatewayAPIManagedLabels(capp cappv1alpha1.Capp) map[string]string {
	labels := cappmeta.ManagedResourceLabels(capp.Name)
	labels[cappmeta.CappNamespaceKey] = capp.Namespace
	return labels
}

// HostnameClaimedError is returned when the ClusterDomainClaim of a hostname belongs to another namespace, or to
// another Capp of the same namespace.
type HostnameClaimedError struct {
	Hostname  string
	Namespace string
	// Capp is set when the claim belongs to another Capp of the same namespace.
	Capp string
}

func (e HostnameClaimedError) Error() string {
	if e.Capp != "" {
		return fmt.Sprintf("hostname %q is claimed by Capp %s/%s", e.Hostname, e.Namespace, e.Capp)
	}
	return fmt.Sprintf("hostname %q is claimed by namespace %q", e.Hostname, e.Namespace)
}

// IsHostnameClaimedError reports whether err says the hostname belongs to someone else.
func IsHostnameClaimedError(err error) bool {
	var claimed HostnameClaimedError
	return stderrors.As(err, &claimed)
}

// claimOwner reports whom a ClusterDomainClaim belongs to: the namespace, and the Capp named in its labels, if any.
// A claim without Capp labels was created by Knative for a DomainMapping.
func claimOwner(claim netv1alpha1.ClusterDomainClaim) (namespace, cappName string) {
	return claim.Spec.Namespace, claim.Labels[cappmeta.CappResourceKey]
}

// requireHostnameClaim returns nil only when the ClusterDomainClaim of hostname exists and belongs to the Capp:
// its namespace, and its labels when another Capp of that namespace has labelled it. Every object that serves or
// secures the hostname is created only after this check, so a Capp cannot take over a hostname someone else holds.
// The claim is read from the API server: the ClusterDomainClaim manager may have created it in the same reconcile,
// before the cache has it.
func requireHostnameClaim(ctx context.Context, reader client.Reader, hostname string, capp cappv1alpha1.Capp) error {
	claim := netv1alpha1.ClusterDomainClaim{}
	if err := reader.Get(ctx, types.NamespacedName{Name: hostname}, &claim); err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("ClusterDomainClaim %q does not exist yet", hostname)
		}
		return fmt.Errorf("failed to get ClusterDomainClaim %q: %w", hostname, err)
	}
	namespace, owner := claimOwner(claim)
	if namespace != capp.Namespace {
		return HostnameClaimedError{Hostname: hostname, Namespace: namespace}
	}
	if owner != "" && owner != capp.Name {
		return HostnameClaimedError{Hostname: hostname, Namespace: namespace, Capp: owner}
	}
	return nil
}

// domainMappingReady reports whether the Capp's DomainMapping exists and is Ready for its current generation.
// When Gateway API routing is turned off, the operator's route keeps serving until then.
func domainMappingReady(ctx context.Context, c client.Client, namespace, hostname string) (bool, error) {
	dm := knativev1beta1.DomainMapping{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: hostname}, &dm); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get DomainMapping %q: %w", hostname, err)
	}
	ready := dm.Status.GetCondition(knativev1beta1.DomainMappingConditionReady)
	return ready != nil && ready.Status == corev1.ConditionTrue && dm.Status.ObservedGeneration >= dm.Generation, nil
}

// replacedByDomainMapping reports whether the Capp's Gateway API objects may be deleted after Gateway API routing
// was turned off: always when the Capp is deleted or has no hostname, otherwise only once the DomainMapping that
// replaces them is Ready, so the hostname is never left without a route.
func replacedByDomainMapping(ctx context.Context, c client.Client, cappConfig *cappv1alpha1.CappConfig, capp cappv1alpha1.Capp) (bool, error) {
	hostname := cappHostname(cappConfig, capp)
	if capp.DeletionTimestamp != nil || hostname == "" || IsGatewayAPIRouting(cappConfig) {
		return true, nil
	}
	return domainMappingReady(ctx, c, capp.Namespace, hostname)
}

// createOrUpdateOwned creates desired, or updates the existing object of that name when the Capp owns it.
// syncSpec copies the desired spec onto the existing object and reports whether it changed.
func createOrUpdateOwned[T client.Object](
	ctx context.Context,
	rm rclient.ResourceManagerClient,
	recorder events.EventRecorder,
	capp cappv1alpha1.Capp,
	desired, existing T,
	kind, eventCreated, eventFailed string,
	syncSpec func(existing, desired T) bool,
) error {
	if err := rm.K8sClient.Get(ctx, client.ObjectKeyFromObject(desired), existing); err != nil {
		if errors.IsNotFound(err) {
			return createManagedResource(ctx, rm.K8sClient, rm.CreateResource, recorder, &capp, desired, kind, eventCreated, eventFailed)
		}
		return fmt.Errorf("failed to get %s %q: %w", kind, desired.GetName(), err)
	}
	if !isOwnedBy(existing, capp) {
		return errNotManaged(kind, existing.GetNamespace(), existing.GetName())
	}

	origOwners := slices.Clone(existing.GetOwnerReferences())
	changed := syncSpec(existing, desired)
	if err := ensureOwnerReference(rm.K8sClient, &capp, existing, kind); err != nil {
		return err
	}
	if !changed && !managedResourceNeedsUpdate(nil, nil, origOwners, existing.GetOwnerReferences()) {
		return nil
	}
	return rm.UpdateResource(ctx, existing)
}

// deleteOwnedExcept deletes the objects the Capp owns, except the one named keep (empty deletes all). Objects of
// other owners, such as the HTTPRoutes net-gateway-api creates with the same labels, are never touched. When the
// Capp is being deleted, garbage collection removes the owned objects after the finalizer, so nothing is sent.
func deleteOwnedExcept[T client.Object](ctx context.Context, c client.Client, capp cappv1alpha1.Capp, objects []T, keep, kind string) error {
	if capp.DeletionTimestamp != nil {
		return nil
	}
	for _, obj := range objects {
		if !isOwnedBy(obj, capp) || (keep != "" && obj.GetName() == keep) {
			continue
		}
		if err := client.IgnoreNotFound(c.Delete(ctx, obj)); err != nil {
			return fmt.Errorf("failed to delete %s %q: %w", kind, obj.GetName(), err)
		}
	}
	return nil
}

// itemPointers returns pointers to the items of a typed list.
func itemPointers[E any, P interface {
	*E
	client.Object
}](items []E) []P {
	out := make([]P, len(items))
	for i := range items {
		out[i] = P(&items[i])
	}
	return out
}

// readerOrClient returns reader, or c when no uncached reader was given (as in tests).
func readerOrClient(reader client.Reader, c client.Client) client.Reader {
	if reader != nil {
		return reader
	}
	return c
}
