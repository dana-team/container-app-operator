package resourcemanagers

import (
	"context"
	"fmt"
	"maps"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ClusterDomainClaim                        = "ClusterDomainClaim"
	eventCappClusterDomainClaimCreationFailed = "ClusterDomainClaimCreationFailed"
	eventCappClusterDomainClaimCreated        = "ClusterDomainClaimCreated"
)

// ClusterDomainClaimManager claims a Capp's hostname for the Capp's namespace when hostnames are routed through
// Gateway API objects. A DomainMapping gets this claim from Knative; without a DomainMapping the operator creates
// it, so a hostname stays unique within the cluster, also against DomainMappings created outside the operator.
//
// The controller watches claims with Capp labels, so a claim that is deleted, for example by the finalizer of the
// DomainMapping being replaced, is created again on the next reconcile.
type ClusterDomainClaimManager struct {
	rclient.ResourceManagerClient
	EventRecorder events.EventRecorder
	CappConfig    *cappv1alpha1.CappConfig
	// APIReader reads claims and Capps from the API server. When nil, K8sClient is used.
	APIReader client.Reader
}

// IsRequired reports whether the Capp's hostname must be claimed by the operator.
func (c ClusterDomainClaimManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return IsGatewayAPIRouting(c.CappConfig) && capp.Spec.RouteSpec.Hostname != ""
}

// Manage creates the claim, or adopts an existing claim of the Capp's namespace that no other Capp holds.
// When Gateway API routing is off, a claim of a Capp that still has a hostname is left in place: the Capp's
// DomainMapping uses the same claim, and deleting it would let another namespace take the hostname in between.
func (c ClusterDomainClaimManager) Manage(ctx context.Context, capp cappv1alpha1.Capp) error {
	if capp.Spec.RouteSpec.Hostname == "" {
		return c.CleanUp(ctx, capp)
	}
	if !c.IsRequired(capp) {
		return nil
	}

	hostname := cappHostname(c.CappConfig, capp)
	if err := c.createOrAdopt(ctx, capp, hostname); err != nil {
		return err
	}
	// Claims for a former hostname of this Capp, for example after the DNS zone changed.
	return c.deleteClaims(ctx, capp, hostname)
}

func (c ClusterDomainClaimManager) reader() client.Reader {
	return readerOrClient(c.APIReader, c.K8sClient)
}

func (c ClusterDomainClaimManager) createOrAdopt(ctx context.Context, capp cappv1alpha1.Capp, hostname string) error {
	wantLabels := gatewayAPIManagedLabels(capp)

	claim := netv1alpha1.ClusterDomainClaim{}
	if err := c.reader().Get(ctx, types.NamespacedName{Name: hostname}, &claim); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get ClusterDomainClaim %q: %w", hostname, err)
		}
		return c.create(ctx, capp, hostname, wantLabels)
	}

	namespace, owner := claimOwner(claim)
	if namespace != capp.Namespace {
		return HostnameClaimedError{Hostname: hostname, Namespace: namespace}
	}
	if owner == capp.Name {
		return nil
	}
	if owner != "" {
		// Another Capp of this namespace labelled the claim. It keeps it while it exists, so two Capps with one
		// hostname do not take the claim from each other, and deleting one cannot release the other's hostname.
		held, err := c.cappHoldsClaim(ctx, capp.Namespace, owner)
		if err != nil {
			return err
		}
		if held {
			return HostnameClaimedError{Hostname: hostname, Namespace: namespace, Capp: owner}
		}
	}

	// A claim Knative created for the Capp's former DomainMapping, or one left by a deleted Capp.
	if claim.Labels == nil {
		claim.Labels = map[string]string{}
	}
	maps.Copy(claim.Labels, wantLabels)
	return c.UpdateResource(ctx, &claim)
}

func (c ClusterDomainClaimManager) create(ctx context.Context, capp cappv1alpha1.Capp, hostname string, labels map[string]string) error {
	claim := netv1alpha1.ClusterDomainClaim{
		ObjectMeta: metav1.ObjectMeta{Name: hostname, Labels: labels},
		Spec:       netv1alpha1.ClusterDomainClaimSpec{Namespace: capp.Namespace},
	}
	if err := c.CreateResource(ctx, &claim); err != nil {
		if errors.IsAlreadyExists(err) {
			return fmt.Errorf("ClusterDomainClaim %q was created concurrently: %w", hostname, err)
		}
		c.EventRecorder.Eventf(&capp, nil, corev1.EventTypeWarning, eventCappClusterDomainClaimCreationFailed, eventCappClusterDomainClaimCreationFailed,
			fmt.Sprintf("Failed to create %s %s", ClusterDomainClaim, hostname))
		return err
	}
	c.EventRecorder.Eventf(&capp, nil, corev1.EventTypeNormal, eventCappClusterDomainClaimCreated, eventCappClusterDomainClaimCreated,
		fmt.Sprintf("Created %s %s", ClusterDomainClaim, hostname))
	return nil
}

// cappHoldsClaim reports whether the Capp named in a claim's labels still exists and is not being deleted.
func (c ClusterDomainClaimManager) cappHoldsClaim(ctx context.Context, namespace, name string) (bool, error) {
	owner := cappv1alpha1.Capp{}
	if err := c.reader().Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &owner); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get Capp %s/%s holding a ClusterDomainClaim: %w", namespace, name, err)
	}
	return owner.DeletionTimestamp == nil, nil
}

// CleanUp deletes the claims this Capp holds. Claims are cluster scoped, so they carry no owner reference
// and are found by the Capp name and namespace labels.
func (c ClusterDomainClaimManager) CleanUp(ctx context.Context, capp cappv1alpha1.Capp) error {
	return c.deleteClaims(ctx, capp, "")
}

// deleteClaims deletes the claims labelled for this Capp, except the one named keep (empty deletes all).
func (c ClusterDomainClaimManager) deleteClaims(ctx context.Context, capp cappv1alpha1.Capp, keep string) error {
	claims := netv1alpha1.ClusterDomainClaimList{}
	if err := c.K8sClient.List(ctx, &claims, client.MatchingLabels(gatewayAPIManagedLabels(capp))); err != nil {
		return fmt.Errorf("unable to list %ss of Capp %q: %w", ClusterDomainClaim, capp.Name, err)
	}
	for i := range claims.Items {
		claim := &claims.Items[i]
		if claim.Name == keep || claim.Spec.Namespace != capp.Namespace {
			continue
		}
		if err := client.IgnoreNotFound(c.K8sClient.Delete(ctx, claim)); err != nil {
			return fmt.Errorf("failed to delete %s %q: %w", ClusterDomainClaim, claim.Name, err)
		}
	}
	return nil
}

// ClaimCappRequest returns the Capp a ClusterDomainClaim's labels name, for mapping watch events.
func ClaimCappRequest(claim metav1.Object) (types.NamespacedName, bool) {
	name := claim.GetLabels()[cappmeta.CappResourceKey]
	namespace := claim.GetLabels()[cappmeta.CappNamespaceKey]
	if name == "" || namespace == "" {
		return types.NamespacedName{}, false
	}
	return types.NamespacedName{Namespace: namespace, Name: name}, true
}
