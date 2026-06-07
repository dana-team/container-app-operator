package resourcemanagers

import (
	"context"
	"fmt"

	dnsrecordv1alpha1 "github.com/dana-team/provider-dns-v2/apis/namespaced/record/v1alpha1"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/retry"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	DNSRecord                        = "DNSRecord"
	eventCappDNSRecordCreationFailed = "DNSRecordCreationFailed"
	eventCappDNSRecordCreated        = "DNSRecordCreated"
	ClusterProviderConfigKind        = "ClusterProviderConfig"
)

type DNSRecordManager struct {
	rclient.ResourceManagerClient
	EventRecorder events.EventRecorder
}

// prepareResource prepares a DNSRecord resource based on the provided Capp.
func (r DNSRecordManager) prepareResource(ctx context.Context, capp cappv1alpha1.Capp) (dnsrecordv1alpha1.CNAMERecord, error) {
	dnsConfig, err := utils.GetDNSConfig(ctx, r.K8sclient)
	if err != nil {
		return dnsrecordv1alpha1.CNAMERecord{}, err
	}

	resourceName := utils.GenerateResourceName(capp.Spec.RouteSpec.Hostname, dnsConfig.Zone)
	recordName := utils.GenerateRecordName(capp.Spec.RouteSpec.Hostname, dnsConfig.Zone)

	dnsRecord := dnsrecordv1alpha1.CNAMERecord{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: capp.Namespace,
			Labels: utils.MergeMaps(utils.ManagedResourceLabels(capp.Name), map[string]string{
				utils.CappNamespaceKey: capp.Namespace,
			}),
		},
		Spec: dnsrecordv1alpha1.CNAMERecordSpec{
			ForProvider: dnsrecordv1alpha1.CNAMERecordParameters{
				Name:  &recordName,
				Zone:  &dnsConfig.Zone,
				Cname: &dnsConfig.CNAME,
			},
		},
	}
	dnsRecord.Spec.ProviderConfigReference = &xpv1.ProviderConfigReference{
		Name: dnsConfig.Provider,
		Kind: ClusterProviderConfigKind,
	}

	return dnsRecord, nil
}

// CleanUp attempts to delete all DNSRecords associated with a given Capp resource.
func (r DNSRecordManager) CleanUp(ctx context.Context, capp cappv1alpha1.Capp) error {
	dnsRecords, err := r.getPreviousDNSRecords(ctx, capp)
	if err != nil {
		return err
	}

	for _, dnsRecord := range dnsRecords.Items {
		if capp.DeletionTimestamp != nil {
			ok, err := controllerutil.HasOwnerReference(dnsRecord.OwnerReferences, &capp, r.K8sclient.Scheme())
			if err != nil {
				return err
			}
			if ok {
				continue
			}
		}
		bareRecord := rclient.GetBareDNSRecord(dnsRecord.Name, dnsRecord.Namespace)
		if err := r.DeleteResource(ctx, &bareRecord); err != nil {
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
func (r DNSRecordManager) Manage(ctx context.Context, capp cappv1alpha1.Capp) error {
	if r.IsRequired(capp) {
		return r.createOrUpdate(ctx, capp)
	}

	return r.CleanUp(ctx, capp)
}

// createOrUpdate creates or updates a DNSRecord resource.
func (r DNSRecordManager) createOrUpdate(ctx context.Context, capp cappv1alpha1.Capp) error {
	dnsRecordFromCapp, err := r.prepareResource(ctx, capp)
	if err != nil {
		return fmt.Errorf("failed to prepare DNSRecord: %w", err)
	}

	dnsRecord := dnsrecordv1alpha1.CNAMERecord{}

	if err := r.K8sclient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: dnsRecordFromCapp.Name}, &dnsRecord); err != nil {
		if errors.IsNotFound(err) {
			return createManagedResource(ctx, r.K8sclient, r.CreateResource, r.EventRecorder, &capp, &dnsRecordFromCapp,
				"DNSRecord", eventCappDNSRecordCreated, eventCappDNSRecordCreationFailed)
		}
		return fmt.Errorf("failed to get DNSRecord %q: %w", dnsRecordFromCapp.Name, err)
	}

	needs, err := r.dnsRecordNeedsUpdate(dnsRecord, dnsRecordFromCapp, &capp)
	if err != nil {
		return err
	}
	if !needs {
		return nil
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latestRecord := dnsrecordv1alpha1.CNAMERecord{}
		if err := r.K8sclient.Get(ctx, types.NamespacedName{Namespace: dnsRecord.Namespace, Name: dnsRecord.Name}, &latestRecord); err != nil {
			return err
		}

		needs, err := r.dnsRecordNeedsUpdate(latestRecord, dnsRecordFromCapp, &capp)
		if err != nil {
			return err
		}
		if !needs {
			return nil
		}

		orig := latestRecord.DeepCopy()
		if err := ensureOwnerReference(r.K8sclient, &capp, &latestRecord, "DNSRecord"); err != nil {
			return err
		}
		latestRecord.Spec.ForProvider = *dnsRecordFromCapp.Spec.ForProvider.DeepCopy()
		if dnsRecordFromCapp.Spec.ProviderConfigReference != nil {
			latestRecord.Spec.ProviderConfigReference = dnsRecordFromCapp.Spec.ProviderConfigReference.DeepCopy()
		} else {
			latestRecord.Spec.ProviderConfigReference = nil
		}

		if !managedResourceNeedsUpdate(orig.Spec, latestRecord.Spec, orig.OwnerReferences, latestRecord.OwnerReferences) {
			return nil
		}

		return r.UpdateResource(ctx, &latestRecord)
	})
}

func (r DNSRecordManager) dnsRecordNeedsUpdate(current, desired dnsrecordv1alpha1.CNAMERecord, capp *cappv1alpha1.Capp) (bool, error) {
	if !equality.Semantic.DeepEqual(current.Spec.ForProvider, desired.Spec.ForProvider) ||
		!equality.Semantic.DeepEqual(current.Spec.ProviderConfigReference, desired.Spec.ProviderConfigReference) {
		return true, nil
	}
	ok, err := controllerutil.HasOwnerReference(current.OwnerReferences, capp, r.K8sclient.Scheme())
	if err != nil {
		return false, err
	}
	return !ok, nil
}

// getPreviousDNSRecords returns a list of all DNSRecord objects that are related to the given Capp.
func (r DNSRecordManager) getPreviousDNSRecords(ctx context.Context, capp cappv1alpha1.Capp) (dnsrecordv1alpha1.CNAMERecordList, error) {
	dnsRecords := dnsrecordv1alpha1.CNAMERecordList{}
	if err := listManagedResources(ctx, r.K8sclient, capp, &dnsRecords, "DNSRecord", labels.Set{
		utils.CappNamespaceKey: capp.Namespace,
	}); err != nil {
		return dnsRecords, err
	}
	return dnsRecords, nil
}
