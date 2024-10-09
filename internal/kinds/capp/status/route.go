package status

import (
	"context"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"

	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	dnsrecordv1alpha1 "github.com/dana-team/provider-dns/apis/record/v1alpha1"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// buildRouteStatus constructs the Route Status of the Capp object in accordance to the
// status of the corresponding DomainMapping, DNSRecord and Certificate objects if such exist.
func buildRouteStatus(ctx context.Context, kubeClient client.Client, capp cappv1alpha1.Capp, isRequired map[string]bool) (cappv1alpha1.RouteStatus, error) {
	routeStatus := cappv1alpha1.RouteStatus{}

	dnsConfig, err := utils.GetDNSConfig(ctx, kubeClient)
	if err != nil {
		return routeStatus, err
	}

	zone, err := utils.GetZoneFromConfig(dnsConfig)
	if err != nil {
		return routeStatus, err
	}

	domainMappingStatus, err := buildDomainMappingStatus(ctx, kubeClient, capp, isRequired[rmanagers.DomainMapping], zone)
	if err != nil {
		return routeStatus, err
	}

	dnsRecordStatus, err := buildDNSRecordStatus(ctx, kubeClient, capp, isRequired[rmanagers.DNSRecord], zone)
	if err != nil {
		return routeStatus, err
	}

	certificateStatus, err := buildCertificateStatus(ctx, kubeClient, capp, isRequired[rmanagers.Certificate], zone)
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
	domainMappingName := utils.GenerateResourceName(capp.Spec.RouteSpec.Hostname, zone)
	if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: domainMappingName}, domainMapping); err != nil {
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
	certificateName := utils.GenerateResourceName(capp.Spec.RouteSpec.Hostname, zone)

	if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: certificateName}, certificate); err != nil {
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
	cnameRecordName := utils.GenerateResourceName(capp.Spec.RouteSpec.Hostname, zone)
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: cnameRecordName}, cnameRecord); err != nil {
		return dnsrecordv1alpha1.CNAMERecordStatus{}, err
	}

	return cnameRecord.Status, nil
}
