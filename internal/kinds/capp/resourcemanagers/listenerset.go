package resourcemanagers

import (
	"context"
	"fmt"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// errListenerSetNotInstalled is reported for a TLS Capp in ListenerSet mode on a cluster whose Gateway API has no
// ListenerSet kind. The Capp is still served over HTTP.
var errListenerSetNotInstalled = errors.NewBadRequest(
	"the ListenerSet kind (gateway.networking.k8s.io/v1) is not served by this cluster: HTTPS is not set up for this hostname; " +
		"set CappConfig spec.gatewayAPI.tlsMode to Gateway, or install a Gateway API version with ListenerSet and restart the operator")

const (
	ListenerSet                        = "ListenerSet"
	eventCappListenerSetCreationFailed = "ListenerSetCreationFailed"
	eventCappListenerSetCreated        = "ListenerSetCreated"
)

// ListenerSetManager serves HTTPS for a Capp hostname with the Capp's own certificate. Each Capp gets its own
// ListenerSet attached to the Gateway, so the number of TLS hostnames is not limited by the 64 listeners a
// single Gateway object may hold, and no shared object is written for each Capp.
type ListenerSetManager struct {
	rclient.ResourceManagerClient
	EventRecorder events.EventRecorder
	CappConfig    *cappv1alpha1.CappConfig
	// APIReader reads the hostname claim from the API server. When nil, K8sClient is used.
	APIReader client.Reader
	// Installed reports whether the ListenerSet CRD was installed when the operator started.
	Installed bool
}

// IsRequired reports whether the Capp's HTTPS is served by its own ListenerSet.
func (l ListenerSetManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return UsesListenerSetTLS(l.CappConfig, capp)
}

// Manage creates or updates the Capp's ListenerSet once its TLS Secret exists, or cleans it up when not required.
func (l ListenerSetManager) Manage(ctx context.Context, capp cappv1alpha1.Capp) error {
	if !l.IsRequired(capp) {
		if !l.Installed && !IsGatewayAPIRouting(l.CappConfig) {
			return nil
		}
		replaced, err := replacedByDomainMapping(ctx, l.K8sClient, l.CappConfig, capp)
		if err != nil || !replaced {
			return err
		}
		return l.CleanUp(ctx, capp)
	}

	if !l.Installed {
		return errListenerSetNotInstalled
	}

	hostname := cappHostname(l.CappConfig, capp)
	if err := requireHostnameClaim(ctx, readerOrClient(l.APIReader, l.K8sClient), hostname, capp); err != nil {
		if IsHostnameClaimedError(err) {
			// Stop serving a hostname this Capp no longer holds.
			if cleanupErr := l.deleteListenerSets(ctx, capp, ""); cleanupErr != nil {
				return cleanupErr
			}
		}
		return err
	}

	secretName := generateTLSSecretName(hostname)
	secret := corev1.Secret{}
	if err := l.K8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: capp.Namespace}, &secret); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get tlsSecret %s for ListenerSet: %w", secretName, err)
		}
		// The listener is added once cert-manager has issued the certificate. The controller watches the
		// Capp-labelled Secret, so its creation triggers the next reconcile.
		l.Log.Info("tlsSecret does not yet exist", "secretName", secretName)
		return nil
	}

	desired := l.prepareResource(capp, hostname, secretName)
	if err := createOrUpdateOwned(ctx, l.ResourceManagerClient, l.EventRecorder, capp, &desired, &gatewayv1.ListenerSet{},
		ListenerSet, eventCappListenerSetCreated, eventCappListenerSetCreationFailed,
		func(existing, desired *gatewayv1.ListenerSet) bool {
			if equality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
				return false
			}
			existing.Spec = desired.Spec
			return true
		}); err != nil {
		return err
	}
	// ListenerSets for a former hostname of this Capp, for example after the DNS zone changed.
	return l.deleteListenerSets(ctx, capp, desired.Name)
}

func (l ListenerSetManager) prepareResource(capp cappv1alpha1.Capp, hostname, secretName string) gatewayv1.ListenerSet {
	cfg := gatewayAPIConfig(l.CappConfig)
	return gatewayv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GatewayObjectName(hostname),
			Namespace: capp.Namespace,
			Labels:    cappmeta.ManagedResourceLabels(capp.Name),
		},
		Spec: gatewayv1.ListenerSetSpec{
			ParentRef: gatewayv1.ParentGatewayReference{
				Group:     ptr.To(gatewayv1.Group(gatewayAPIGroup)),
				Kind:      ptr.To(gatewayv1.Kind(gatewayKind)),
				Name:      gatewayv1.ObjectName(cfg.Gateway.Name),
				Namespace: ptr.To(gatewayv1.Namespace(cfg.Gateway.Namespace)),
			},
			Listeners: []gatewayv1.ListenerEntry{{
				Name:     listenerSetHTTPSListenerName,
				Hostname: ptr.To(gatewayv1.Hostname(hostname)),
				Port:     httpsPort(cfg),
				Protocol: gatewayv1.HTTPSProtocolType,
				TLS: &gatewayv1.ListenerTLSConfig{
					Mode: ptr.To(gatewayv1.TLSModeTerminate),
					CertificateRefs: []gatewayv1.SecretObjectReference{{
						Group: ptr.To(gatewayv1.Group("")),
						Kind:  ptr.To(gatewayv1.Kind("Secret")),
						Name:  gatewayv1.ObjectName(secretName),
					}},
				},
				AllowedRoutes: &gatewayv1.AllowedRoutes{
					Namespaces: &gatewayv1.RouteNamespaces{From: ptr.To(gatewayv1.NamespacesFromSame)},
				},
			}},
		},
	}
}

// deleteListenerSets deletes the Capp's ListenerSets except the one named keep (empty deletes all). Nothing is
// sent when the ListenerSet CRD was not installed at startup; the Gateway TLS mode works without it.
func (l ListenerSetManager) deleteListenerSets(ctx context.Context, capp cappv1alpha1.Capp, keep string) error {
	if !l.Installed {
		return nil
	}
	list := gatewayv1.ListenerSetList{}
	if err := listManagedResources(ctx, l.K8sClient, capp, &list, ListenerSet, nil); err != nil {
		return err
	}
	return deleteOwnedExcept(ctx, l.K8sClient, capp, itemPointers(list.Items), keep, ListenerSet)
}

// CleanUp deletes the Capp's ListenerSets, and with Gateway API routing the TLS Secret once nothing uses it.
func (l ListenerSetManager) CleanUp(ctx context.Context, capp cappv1alpha1.Capp) error {
	if err := l.deleteListenerSets(ctx, capp, ""); err != nil {
		return err
	}
	if !IsGatewayAPIRouting(l.CappConfig) {
		// The DomainMapping manager owns the TLS Secret in this mode.
		return nil
	}
	return l.deleteUnusedTLSSecret(ctx, capp)
}

// deleteUnusedTLSSecret deletes the Capp's TLS Secret once no Certificate serves it. cert-manager leaves the
// Secret behind when its Certificate is deleted; with DomainMappings the DomainMapping manager deletes it, and
// with Gateway API routing this manager does.
func (l ListenerSetManager) deleteUnusedTLSSecret(ctx context.Context, capp cappv1alpha1.Capp) error {
	hostname := cappHostname(l.CappConfig, capp)
	if hostname == "" || (capp.DeletionTimestamp == nil && certificateRequired(l.CappConfig, capp)) {
		return nil
	}
	return deleteIssuedTLSSecret(ctx, l.ResourceManagerClient, capp, hostname)
}
