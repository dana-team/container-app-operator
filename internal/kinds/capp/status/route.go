package status

import (
	"context"

	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	certv1alpha1 "github.com/dana-team/certificate-operator/api/v1alpha1"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	dnsv1alpha1 "github.com/dana-team/provider-dns/apis/recordset/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// buildRouteStatus constructs the Route Status of the Capp object in accordance to the
// status of the corresponding DomainMapping, ARecordSet and Certificate objects if such exist.
func buildRouteStatus(ctx context.Context, kubeClient client.Client, capp cappv1alpha1.Capp, isRequired map[string]bool) (cappv1alpha1.RouteStatus, error) {
	routeStatus := cappv1alpha1.RouteStatus{}

	zone, err := utils.GetZoneFromConfig(ctx, kubeClient)
	if err != nil {
		return routeStatus, err
	}

	domainMappingStatus, err := buildDomainMappingStatus(ctx, kubeClient, capp, isRequired[rmanagers.DomainMapping], zone)
	if err != nil {
		return routeStatus, err
	}

	aRecordSetStatus, err := buildARecordSetStatus(ctx, kubeClient, capp, isRequired[rmanagers.ARecordSet], zone)
	if err != nil {
		return routeStatus, err
	}

	certificateStatus, err := buildCertificateStatus(ctx, kubeClient, capp, isRequired[rmanagers.Certificate], zone)
	if err != nil {
		return routeStatus, err
	}

	routeStatus.DomainMappingObjectStatus = domainMappingStatus
	routeStatus.ARecordSetObjectStatus = aRecordSetStatus
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
	domainMappingName := utils.GenerateResourceName(capp.Spec.RouteSpec.Hostname, zone)
	if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: domainMappingName}, domainMapping); err != nil {
		return knativev1beta1.DomainMappingStatus{}, err
	}

	return domainMapping.Status, nil
}

// buildCertificateStatus partly constructs the Route Status of the Capp object in accordance to the
// status of the corresponding Certificate object.
func buildCertificateStatus(ctx context.Context, kubeClient client.Client, capp cappv1alpha1.Capp, isRequired bool, zone string) (certv1alpha1.CertificateStatus, error) {
	if !isRequired {
		return certv1alpha1.CertificateStatus{}, nil
	}

	certificate := &certv1alpha1.Certificate{}
	certificateName := utils.GenerateResourceName(capp.Spec.RouteSpec.Hostname, zone)
	if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: certificateName}, certificate); err != nil {
		return certv1alpha1.CertificateStatus{}, err
	}

	return certificate.Status, nil
}

// buildARecordSetStatus partly constructs the Route Status of the Capp object in accordance to the
// status of the corresponding ARecordSet object.
func buildARecordSetStatus(ctx context.Context, kubeClient client.Client, capp cappv1alpha1.Capp, isRequired bool, zone string) (dnsv1alpha1.ARecordSetStatus, error) {
	if !isRequired {
		return dnsv1alpha1.ARecordSetStatus{}, nil
	}

	aRecordSet := &dnsv1alpha1.ARecordSet{}
	aRecordSetName := utils.GenerateResourceName(capp.Spec.RouteSpec.Hostname, zone)
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: aRecordSetName}, aRecordSet); err != nil {
		return dnsv1alpha1.ARecordSetStatus{}, err
	}

	return aRecordSet.Status, nil
}
