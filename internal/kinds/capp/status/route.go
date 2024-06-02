package status

import (
	"context"

	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"

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

	domainMappingStatus, err := buildDomainMappingStatus(ctx, kubeClient, capp, isRequired[rmanagers.DomainMapping])
	if err != nil {
		return routeStatus, err
	}

	aRecordSetStatus, err := buildARecordSetStatus(ctx, kubeClient, capp, isRequired[rmanagers.ARecordSet])
	if err != nil {
		return routeStatus, err
	}

	certificateStatus, err := buildCertificateStatus(ctx, kubeClient, capp, isRequired[rmanagers.Certificate])
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
func buildDomainMappingStatus(ctx context.Context, kubeClient client.Client, capp cappv1alpha1.Capp, isRequired bool) (knativev1beta1.DomainMappingStatus, error) {
	if !isRequired {
		return knativev1beta1.DomainMappingStatus{}, nil
	}

	domainMapping := &knativev1beta1.DomainMapping{}
	domainMappingName := capp.Spec.RouteSpec.Hostname
	if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: domainMappingName}, domainMapping); err != nil {
		return knativev1beta1.DomainMappingStatus{}, err
	}

	return domainMapping.Status, nil
}

// buildCertificateStatus partly constructs the Route Status of the Capp object in accordance to the
// status of the corresponding Certificate object.
func buildCertificateStatus(ctx context.Context, kubeClient client.Client, capp cappv1alpha1.Capp, isRequired bool) (certv1alpha1.CertificateStatus, error) {
	if !isRequired {
		return certv1alpha1.CertificateStatus{}, nil
	}

	certificate := &certv1alpha1.Certificate{}
	certificateName := capp.Spec.RouteSpec.Hostname
	if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: certificateName}, certificate); err != nil {
		return certv1alpha1.CertificateStatus{}, err
	}

	return certificate.Status, nil
}

// buildARecordSetStatus partly constructs the Route Status of the Capp object in accordance to the
// status of the corresponding ARecordSet object.
func buildARecordSetStatus(ctx context.Context, kubeClient client.Client, capp cappv1alpha1.Capp, isRequired bool) (dnsv1alpha1.ARecordSetStatus, error) {
	if !isRequired {
		return dnsv1alpha1.ARecordSetStatus{}, nil
	}

	aRecordSet := &dnsv1alpha1.ARecordSet{}
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: capp.Spec.RouteSpec.Hostname}, aRecordSet); err != nil {
		return dnsv1alpha1.ARecordSetStatus{}, err
	}

	return aRecordSet.Status, nil
}
