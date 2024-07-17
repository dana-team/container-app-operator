package resourcemanagers

import (
	"context"
	"fmt"
	"net"
	"reflect"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	xpcommonv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	dnsv1alpha1 "github.com/dana-team/provider-dns/apis/recordset/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ARecordSet                        = "ARecordSet"
	eventCappARecordSetCreationFailed = "ARecordSetCreationFailed"
	eventCappARecordSetCreated        = "ARecordSetCreated"
	cappNamespaceKey                  = "rcs.dana.io/parent-capp-ns"
	xpProviderDNSConfigRefName        = "dns-default"
	localhostAddress                  = "127.0.0.1"
)

type ARecordSetManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder record.EventRecorder
}

// getAddressesForRecord performs a DNS lookup for the given URL and returns a slice of IP address strings.
func getAddressesForRecord(url string) ([]*string, error) {
	ips, err := net.LookupIP(url)
	if err != nil {
		return []*string{}, err
	}

	addresses := make([]*string, len(ips))
	for i, ip := range ips {
		target := ip.String()
		addresses[i] = &(target)
	}

	return addresses, err
}

// prepareResource prepares a ARecordSet resource based on the provided Capp.
func (r ARecordSetManager) prepareResource(capp cappv1alpha1.Capp) (dnsv1alpha1.ARecordSet, error) {
	placeholderAddress := localhostAddress
	addresses := []*string{&placeholderAddress}

	zone, err := utils.GetZoneFromConfig(r.Ctx, r.K8sclient)
	if err != nil {
		return dnsv1alpha1.ARecordSet{}, err
	}

	resourceName := utils.GenerateResourceName(capp.Spec.RouteSpec.Hostname, zone)
	recordName := utils.GenerateRecordName(capp.Spec.RouteSpec.Hostname, zone)

	// in a normal behavior, it can be assumed that this condition will eventually be satisfied
	// since there will be another reconciliation loop after the Capp status is updated
	if capp.Status.KnativeObjectStatus.URL != nil {
		addresses, err = getAddressesForRecord(capp.Status.KnativeObjectStatus.URL.Host)
		if err != nil {
			return dnsv1alpha1.ARecordSet{}, err
		}
	}

	recordset := dnsv1alpha1.ARecordSet{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: resourceName,
			Labels: map[string]string{
				CappResourceKey:  capp.Name,
				cappNamespaceKey: capp.Namespace,
			},
		},
		Spec: dnsv1alpha1.ARecordSetSpec{
			ForProvider: dnsv1alpha1.ARecordSetParameters{
				Name:      &recordName,
				Zone:      &zone,
				Addresses: addresses,
			},
			ResourceSpec: xpcommonv1.ResourceSpec{
				ProviderConfigReference: &xpcommonv1.Reference{
					Name: xpProviderDNSConfigRefName,
				},
			},
		},
	}

	return recordset, nil
}

// CleanUp attempts to delete the associated ARecordSet for a given Capp resource.
func (r ARecordSetManager) CleanUp(capp cappv1alpha1.Capp) error {
	resourceManager := rclient.ResourceManagerClient{Ctx: r.Ctx, K8sclient: r.K8sclient, Log: r.Log}

	if capp.Status.RouteStatus.DomainMappingObjectStatus.URL != nil {
		aRecordSet := rclient.GetBareARecordSet(capp.Status.RouteStatus.DomainMappingObjectStatus.URL.Host)
		if err := resourceManager.DeleteResource(&aRecordSet); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}
	}
	return nil
}

// IsRequired is responsible to determine if resource ARecordSet is required.
func (r ARecordSetManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return capp.Spec.RouteSpec.Hostname != ""
}

// Manage creates or updates a ARecordSet resource based on the provided Capp if it's required.
// If it's not, then it cleans up the resource if it exists.
func (r ARecordSetManager) Manage(capp cappv1alpha1.Capp) error {
	if r.IsRequired(capp) {
		return r.createOrUpdate(capp)
	}

	return r.CleanUp(capp)
}

// createOrUpdate creates or updates a DomainMapping resource.
func (r ARecordSetManager) createOrUpdate(capp cappv1alpha1.Capp) error {
	aRecordSetFromCapp, err := r.prepareResource(capp)
	if err != nil {
		return fmt.Errorf("failed to prepare ARecordSet: %w", err)
	}

	aRecordSet := dnsv1alpha1.ARecordSet{}
	resourceManager := rclient.ResourceManagerClient{Ctx: r.Ctx, K8sclient: r.K8sclient, Log: r.Log}

	if err := r.K8sclient.Get(r.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: aRecordSetFromCapp.Name}, &aRecordSet); err != nil {
		if errors.IsNotFound(err) {
			if err := r.createARecordSet(capp, aRecordSetFromCapp, resourceManager); err != nil {
				return err
			}
			if capp.Status.RouteStatus.DomainMappingObjectStatus.URL != nil {
				if err := r.handlePreviousARecordSets(capp, resourceManager, aRecordSetFromCapp.Name); err != nil {
					return fmt.Errorf("failed to delete previous Certificates: %w", err)
				}
			}
		} else {
			return fmt.Errorf("failed to get ARecordSet %q: %w", aRecordSetFromCapp.Name, err)
		}
	}

	return r.updateARecordSet(aRecordSet, aRecordSetFromCapp, resourceManager)
}

// createKSVC creates a new ARecordSet and emits an event.
func (r ARecordSetManager) createARecordSet(capp cappv1alpha1.Capp, aRecordSetFromCapp dnsv1alpha1.ARecordSet, resourceManager rclient.ResourceManagerClient) error {
	if err := resourceManager.CreateResource(&aRecordSetFromCapp); err != nil {
		r.EventRecorder.Event(&capp, corev1.EventTypeWarning, eventCappARecordSetCreationFailed,
			fmt.Sprintf("Failed to create ARecordSet %s", aRecordSetFromCapp.Name))

		return err
	}

	r.EventRecorder.Event(&capp, corev1.EventTypeNormal, eventCappARecordSetCreated,
		fmt.Sprintf("Created ARecordSet %s", aRecordSetFromCapp.Name))

	return nil
}

// updateARecordSet checks if an update to the ARecordSet is necessary and performs the update to match desired state.
func (r ARecordSetManager) updateARecordSet(aRecordSet, aRecordSetFromCapp dnsv1alpha1.ARecordSet, resourceManager rclient.ResourceManagerClient) error {
	if !reflect.DeepEqual(aRecordSet.Spec, aRecordSetFromCapp.Spec) {
		aRecordSet.Spec = aRecordSetFromCapp.Spec
		return resourceManager.UpdateResource(&aRecordSet)
	}

	return nil
}

// handlePreviousARecordSets takes care of removing unneeded ARecordSet objects. If the new ARecordSet
// is not yet available then return early and do not delete the previous Records.
func (r ARecordSetManager) handlePreviousARecordSets(capp cappv1alpha1.Capp, resourceManager rclient.ResourceManagerClient, name string) error {
	available, err := utils.IsARecordSetAvailable(r.Ctx, r.K8sclient, name, capp.Namespace)
	if err != nil {
		return err
	}

	if !available {
		return nil
	}

	arecordsets, err := r.getPreviousARecordSets(capp)
	if err != nil {
		return err
	}

	return r.deletePreviousARecordSets(arecordsets, resourceManager, capp.Spec.RouteSpec.Hostname)
}

// getPreviousARecordSets returns a list of all ARecordSet objects that are related to the given Capp.
func (r ARecordSetManager) getPreviousARecordSets(capp cappv1alpha1.Capp) (dnsv1alpha1.ARecordSetList, error) {
	arecordsets := dnsv1alpha1.ARecordSetList{}

	nameRequirement, err := labels.NewRequirement(CappResourceKey, selection.Equals, []string{capp.Name})
	if err != nil {
		return arecordsets, fmt.Errorf("unable to create label requirement for Capp: %w", err)
	}

	nsRequirement, err := labels.NewRequirement(cappNamespaceKey, selection.Equals, []string{capp.Namespace})
	if err != nil {
		return arecordsets, fmt.Errorf("unable to create label requirement for Capp: %w", err)
	}

	labelSelector := labels.NewSelector().Add(*nameRequirement).Add(*nsRequirement)
	listOptions := client.ListOptions{
		LabelSelector: labelSelector,
	}

	if err = r.K8sclient.List(r.Ctx, &arecordsets, &listOptions); err != nil {
		return arecordsets, fmt.Errorf("unable to list ARecordSets of Capp %q: %w", capp.Name, err)
	}

	return arecordsets, nil
}

// deletePreviousARecordSets deletes all previous ARecordSets associated with a Capp.
func (r ARecordSetManager) deletePreviousARecordSets(arecordsets dnsv1alpha1.ARecordSetList, resourceManager rclient.ResourceManagerClient, hostname string) error {
	for _, arecordset := range arecordsets.Items {
		if arecordset.Name != hostname {
			recordset := rclient.GetBareARecordSet(arecordset.Name)
			if err := resourceManager.DeleteResource(&recordset); err != nil {
				return err
			}
		}
	}
	return nil
}
