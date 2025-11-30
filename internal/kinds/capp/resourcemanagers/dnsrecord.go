package resourcemanagers

import (
	"context"
	"fmt"
	"reflect"

	dnsrecordv1alpha1 "github.com/dana-team/provider-dns/apis/record/v1alpha1"

	xpcommonv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DNSRecord                        = "DNSRecord"
	eventCappDNSRecordCreationFailed = "DNSRecordCreationFailed"
	eventCappDNSRecordCreated        = "DNSRecordCreated"
)

type DNSRecordManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder record.EventRecorder
}

// prepareResource prepares a DNSRecord resource based on the provided Capp.
func (r DNSRecordManager) prepareResource(capp cappv1alpha1.Capp) (dnsrecordv1alpha1.CNAMERecord, error) {
	dnsConfig, err := utils.GetDNSConfig(r.Ctx, r.K8sclient)
	if err != nil {
		return dnsrecordv1alpha1.CNAMERecord{}, err
	}

	zone, err := utils.GetZoneFromConfig(dnsConfig)
	if err != nil {
		return dnsrecordv1alpha1.CNAMERecord{}, err
	}

	cname, err := utils.GetDNSRecordFromConfig(dnsConfig)
	if err != nil {
		return dnsrecordv1alpha1.CNAMERecord{}, err
	}

	xpProvider, err := utils.GetXPProviderFromConfig(dnsConfig)
	if err != nil {
		return dnsrecordv1alpha1.CNAMERecord{}, err
	}

	resourceName := utils.GenerateResourceName(capp.Spec.RouteSpec.Hostname, zone)
	recordName := utils.GenerateRecordName(capp.Spec.RouteSpec.Hostname, zone)

	dnsRecord := dnsrecordv1alpha1.CNAMERecord{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: resourceName,
			Labels: map[string]string{
				utils.CappResourceKey:   capp.Name,
				utils.CappNamespaceKey:  capp.Namespace,
				utils.ManagedByLabelKey: utils.CappKey,
			},
		},
		Spec: dnsrecordv1alpha1.CNAMERecordSpec{
			ForProvider: dnsrecordv1alpha1.CNAMERecordParameters{
				Name:  &recordName,
				Zone:  &zone,
				Cname: &cname,
			},
			ResourceSpec: xpcommonv1.ResourceSpec{
				ProviderConfigReference: &xpcommonv1.Reference{
					Name: xpProvider,
				},
			},
		},
	}

	return dnsRecord, nil
}

// CleanUp attempts to delete all DNSRecords associated with a given Capp resource.
func (r DNSRecordManager) CleanUp(capp cappv1alpha1.Capp) error {
	resourceManager := rclient.ResourceManagerClient{Ctx: r.Ctx, K8sclient: r.K8sclient, Log: r.Log}

	dnsRecords, err := r.getPreviousDNSRecords(capp)
	if err != nil {
		return err
	}

	for _, dnsRecord := range dnsRecords.Items {
		record := rclient.GetBareDNSRecord(dnsRecord.Name)
		if err := resourceManager.DeleteResource(&record); err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return err
		}
	}

	return nil
}

// IsRequired is responsible to determine if resource DNSRecord is required.
func (r DNSRecordManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return capp.Spec.RouteSpec.Hostname != ""
}

// Manage creates or updates a DNSRecord resource based on the provided Capp if it's required.
// If it's not, then it cleans up the resource if it exists.
func (r DNSRecordManager) Manage(capp cappv1alpha1.Capp) error {
	if r.IsRequired(capp) {
		return r.createOrUpdate(capp)
	}

	return r.CleanUp(capp)
}

// createOrUpdate creates or updates a DomainMapping resource.
func (r DNSRecordManager) createOrUpdate(capp cappv1alpha1.Capp) error {
	dnsRecordFromCapp, err := r.prepareResource(capp)
	if err != nil {
		return fmt.Errorf("failed to prepare DNSRecord: %w", err)
	}

	dnsRecord := dnsrecordv1alpha1.CNAMERecord{}
	resourceManager := rclient.ResourceManagerClient{Ctx: r.Ctx, K8sclient: r.K8sclient, Log: r.Log}

	if err := r.K8sclient.Get(r.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: dnsRecordFromCapp.Name}, &dnsRecord); err != nil {
		if errors.IsNotFound(err) {
			if err := r.createDNSRecord(capp, dnsRecordFromCapp, resourceManager); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("failed to get DNSRecord %q: %w", dnsRecordFromCapp.Name, err)
		}
	}

	if capp.Status.RouteStatus.DomainMappingObjectStatus.URL != nil {
		if err := r.handlePreviousDNSRecords(capp, resourceManager, dnsRecordFromCapp.Name); err != nil {
			return fmt.Errorf("failed to delete previous Certificates: %w", err)
		}
	}

	return r.updateDNSRecord(dnsRecord, dnsRecordFromCapp, resourceManager)
}

// createDNSRecord creates a new DNSRecord and emits an event.
func (r DNSRecordManager) createDNSRecord(capp cappv1alpha1.Capp, dnsRecordFromCapp dnsrecordv1alpha1.CNAMERecord, resourceManager rclient.ResourceManagerClient) error {
	if err := resourceManager.CreateResource(&dnsRecordFromCapp); err != nil {
		r.EventRecorder.Event(&capp, corev1.EventTypeWarning, eventCappDNSRecordCreationFailed,
			fmt.Sprintf("Failed to create DNSRecord %s", dnsRecordFromCapp.Name))

		return err
	}

	r.EventRecorder.Event(&capp, corev1.EventTypeNormal, eventCappDNSRecordCreated,
		fmt.Sprintf("Created DNSRecord %s", dnsRecordFromCapp.Name))

	return nil
}

// updateDNSRecord checks if an update to the DNSRecord is necessary and performs the update to match desired state.
func (r DNSRecordManager) updateDNSRecord(dnsRecord, dnsRecordFromCapp dnsrecordv1alpha1.CNAMERecord, resourceManager rclient.ResourceManagerClient) error {
	if !reflect.DeepEqual(dnsRecord.Spec, dnsRecordFromCapp.Spec) {
		dnsRecord.Spec = dnsRecordFromCapp.Spec
		return resourceManager.UpdateResource(&dnsRecord)
	}

	return nil
}

// handlePreviousDNSRecords takes care of removing unneeded DNSRecord objects. If the new DNSRecord
// is not yet available then return early and do not delete the previous Records.
func (r DNSRecordManager) handlePreviousDNSRecords(capp cappv1alpha1.Capp, resourceManager rclient.ResourceManagerClient, name string) error {
	available, err := utils.IsDNSRecordAvailable(r.Ctx, r.K8sclient, name, capp.Namespace)
	if err != nil {
		return err
	}

	if !available {
		return nil
	}

	dnsRecords, err := r.getPreviousDNSRecords(capp)
	if err != nil {
		return err
	}

	return r.deletePreviousDNSRecords(dnsRecords, resourceManager, capp.Spec.RouteSpec.Hostname)
}

// getPreviousDNSRecords returns a list of all DNSRecord objects that are related to the given Capp.
func (r DNSRecordManager) getPreviousDNSRecords(capp cappv1alpha1.Capp) (dnsrecordv1alpha1.CNAMERecordList, error) {
	dnsRecords := dnsrecordv1alpha1.CNAMERecordList{}

	set := labels.Set{
		utils.CappResourceKey:  capp.Name,
		utils.CappNamespaceKey: capp.Namespace,
	}
	listOptions := utils.GetListOptions(set)

	if err := r.K8sclient.List(r.Ctx, &dnsRecords, &listOptions); err != nil {
		return dnsRecords, fmt.Errorf("unable to list DNSRecords of Capp %q: %w", capp.Name, err)
	}

	return dnsRecords, nil
}

// deletePreviousDNSRecords deletes all previous DNSRecords associated with a Capp.
func (r DNSRecordManager) deletePreviousDNSRecords(dnsRecords dnsrecordv1alpha1.CNAMERecordList, resourceManager rclient.ResourceManagerClient, hostname string) error {
	for _, dnsRecord := range dnsRecords.Items {
		if dnsRecord.Name != hostname {
			recordset := rclient.GetBareDNSRecord(dnsRecord.Name)
			if err := resourceManager.DeleteResource(&recordset); err != nil {
				return err
			}
		}
	}
	return nil
}
