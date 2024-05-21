package resourcemanagers

import (
	"context"
	"fmt"
	"net"
	"reflect"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	dnsv1alpha1 "sigs.k8s.io/external-dns/endpoint"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DNSEndpoint                        = "dnsEndpoint"
	eventCappDNSEndpointCreationFailed = "DNSEndpointCreationFailed"
	eventCappDNSEndpointCreated        = "DNSEndpointCreated"
	recordTypeA                        = "A"
)

type DNSEndpointManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder record.EventRecorder
}

// getTargetsForRecord performs a DNS lookup for the given URL and returns a slice of IP address strings.
func getTargetsForRecord(url string) ([]string, error) {
	ips, err := net.LookupIP(url)
	if err != nil {
		return []string{}, err
	}

	targets := make([]string, len(ips))
	for i, ip := range ips {
		targets[i] = ip.String()
	}

	return targets, err
}

// prepareResource prepares a DNSEndpoint resource based on the provided Capp.
func (d DNSEndpointManager) prepareResource(capp cappv1alpha1.Capp) (dnsv1alpha1.DNSEndpoint, error) {
	var targets []string
	var err error

	// in a normal behavior, it can be assumed that this condition will eventually be satisfied
	// since there will be another reconciliation loop after the Capp status is updated
	if capp.Status.KnativeObjectStatus.URL != nil {
		targets, err = getTargetsForRecord(capp.Status.KnativeObjectStatus.URL.Host)
		if err != nil {
			return dnsv1alpha1.DNSEndpoint{}, err
		}
	}

	dnsEndpoint := dnsv1alpha1.DNSEndpoint{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      capp.Name,
			Namespace: capp.Namespace,
			Labels: map[string]string{
				CappResourceKey: capp.Name,
			},
		},
		Spec: dnsv1alpha1.DNSEndpointSpec{
			Endpoints: []*dnsv1alpha1.Endpoint{
				{
					DNSName:    capp.Spec.RouteSpec.Hostname,
					RecordType: recordTypeA,
					Targets:    targets,
					RecordTTL:  0,
					Labels: map[string]string{
						CappResourceKey: capp.Name,
					},
				},
			},
		},
	}

	return dnsEndpoint, nil
}

// CleanUp attempts to delete the associated DNSEndpoint for a given Capp resource.
func (d DNSEndpointManager) CleanUp(capp cappv1alpha1.Capp) error {
	resourceManager := rclient.ResourceManagerClient{Ctx: d.Ctx, K8sclient: d.K8sclient, Log: d.Log}
	dnsEndpoint := rclient.PrepareDNSEndpoint(capp.Name, capp.Namespace)

	if err := resourceManager.DeleteResource(&dnsEndpoint); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

// IsRequired is responsible to determine if resource DNSEndpoint is required.
func (d DNSEndpointManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return capp.Spec.RouteSpec.Hostname != ""
}

// Manage creates or updates a DNSEndpoint resource based on the provided Capp if it's required.
// If it's not, then it cleans up the resource if it exists.
func (d DNSEndpointManager) Manage(capp cappv1alpha1.Capp) error {
	if d.IsRequired(capp) {
		return d.createOrUpdate(capp)
	}

	return d.CleanUp(capp)
}

// createOrUpdate creates or updates a DomainMapping resource.
func (d DNSEndpointManager) createOrUpdate(capp cappv1alpha1.Capp) error {
	dnsEndpointFromCapp, err := d.prepareResource(capp)
	if err != nil {
		return fmt.Errorf("failed to prepare DNSEndpoint: %w", err)
	}

	dnsEndpoint := dnsv1alpha1.DNSEndpoint{}
	resourceManager := rclient.ResourceManagerClient{Ctx: d.Ctx, K8sclient: d.K8sclient, Log: d.Log}

	if err := d.K8sclient.Get(d.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: dnsEndpointFromCapp.Name}, &dnsEndpoint); err != nil {
		if errors.IsNotFound(err) {
			return d.createDNSEndpoint(capp, dnsEndpointFromCapp, resourceManager)
		} else {
			return fmt.Errorf("failed to get DNSEndpoint %q: %w", dnsEndpointFromCapp.Name, err)
		}
	}

	return d.updateDNSEndpoint(dnsEndpoint, dnsEndpointFromCapp, resourceManager)
}

// createKSVC creates a new DNSEndpoint and emits an event.
func (d DNSEndpointManager) createDNSEndpoint(capp cappv1alpha1.Capp, dnsEndpointFromCapp dnsv1alpha1.DNSEndpoint, resourceManager rclient.ResourceManagerClient) error {
	if err := resourceManager.CreateResource(&dnsEndpointFromCapp); err != nil {
		d.EventRecorder.Event(&capp, corev1.EventTypeWarning, eventCappDNSEndpointCreationFailed,
			fmt.Sprintf("Failed to create DNSEndpoint %s", dnsEndpointFromCapp.Name))

		return err
	}

	d.EventRecorder.Event(&capp, corev1.EventTypeNormal, eventCappDNSEndpointCreated,
		fmt.Sprintf("Created DNSEndpoint %s", dnsEndpointFromCapp.Name))

	return nil
}

// updateDNSEndpoint checks if an update to the DNSEndpoint is necessary and performs the update to match desired state.
func (d DNSEndpointManager) updateDNSEndpoint(dnsEndpoint, dnsEndpointFromCapp dnsv1alpha1.DNSEndpoint, resourceManager rclient.ResourceManagerClient) error {
	if !reflect.DeepEqual(dnsEndpoint.Spec, dnsEndpointFromCapp.Spec) {
		dnsEndpoint.Spec = dnsEndpointFromCapp.Spec
		return resourceManager.UpdateResource(&dnsEndpoint)
	}

	return nil
}
