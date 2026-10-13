package status

import (
	"context"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	dnsrecordv1alpha1 "github.com/dana-team/provider-dns-v2/apis/namespaced/record/v1alpha1"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// buildRouteStatus constructs the Route Status of the Capp object in accordance to the
// status of the corresponding DomainMapping, DNSRecord and Certificate objects if such exist.
func buildRouteStatus(ctx context.Context, kubeClient client.Client, capp cappv1alpha1.Capp, isRequired map[string]bool, cappConfig *cappv1alpha1.CappConfig) (cappv1alpha1.RouteStatus, error) {
	routeStatus := cappv1alpha1.RouteStatus{}
	dnsConfig := cappConfig.Spec.DNSConfig

	domainMappingStatus, err := buildDomainMappingStatus(ctx, kubeClient, capp, isRequired[rmanagers.DomainMapping], dnsConfig.Zone)
	if err != nil {
		return routeStatus, err
	}

	dnsRecordStatus, err := buildDNSRecordStatus(ctx, kubeClient, capp, isRequired[rmanagers.DNSRecord], dnsConfig.Zone)
	if err != nil {
		return routeStatus, err
	}

	certificateStatus, err := buildCertificateStatus(ctx, kubeClient, capp, isRequired[rmanagers.Certificate], dnsConfig.Zone)
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
	domainMappingName := rmanagers.GenerateResourceName(capp.Spec.RouteSpec.Hostname, zone)
	if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: domainMappingName}, domainMapping); err != nil {
		if apierrors.IsNotFound(err) {
			return knativev1beta1.DomainMappingStatus{}, nil
		}
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
	certificateName := rmanagers.GenerateResourceName(capp.Spec.RouteSpec.Hostname, zone)

	if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: certificateName}, certificate); err != nil {
		if apierrors.IsNotFound(err) {
			return cmapi.CertificateStatus{}, nil
		}
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
	cnameRecordName := rmanagers.GenerateResourceName(capp.Spec.RouteSpec.Hostname, zone)
	if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: cnameRecordName}, cnameRecord); err != nil {
		if apierrors.IsNotFound(err) {
			return dnsrecordv1alpha1.CNAMERecordStatus{}, nil
		}
		return dnsrecordv1alpha1.CNAMERecordStatus{}, err
	}

	return cnameRecord.Status, nil
}

// cnameRecordConditionTypes are checked in order: Synced reports provider errors, Ready reports availability.
var cnameRecordConditionTypes = []xpv2.ConditionType{xpv2.TypeSynced, xpv2.TypeReady}

// checkRoute returns why the custom hostname is not ready, or nil when it is.
// The CNAMERecord, Certificate (when TLS is enabled) and DomainMapping are checked in that order.
func checkRoute(ctx context.Context, k8sClient client.Client, capp cappv1alpha1.Capp, cappConfig *cappv1alpha1.CappConfig) (*metav1.Condition, error) {
	zone := cappConfig.Spec.DNSConfig.Zone
	key := types.NamespacedName{Namespace: capp.Namespace, Name: rmanagers.GenerateResourceName(capp.Spec.RouteSpec.Hostname, zone)}

	if cond, err := checkCNAMERecord(ctx, k8sClient, key); cond != nil || err != nil {
		return cond, err
	}
	if capp.Spec.RouteSpec.TlsEnabled {
		if cond, err := checkCertificate(ctx, k8sClient, key); cond != nil || err != nil {
			return cond, err
		}
	}
	return checkDomainMapping(ctx, k8sClient, key)
}

// checkCNAMERecord returns why the CNAMERecord is not ready, or nil when it is.
func checkCNAMERecord(ctx context.Context, k8sClient client.Client, key types.NamespacedName) (*metav1.Condition, error) {
	record := &dnsrecordv1alpha1.CNAMERecord{}
	if cond, err := getChildResource(ctx, k8sClient, dnsrecordv1alpha1.CNAMERecord_Kind, key, record); cond != nil || err != nil {
		return cond, err
	}

	for _, conditionType := range cnameRecordConditionTypes {
		rc := record.Status.GetCondition(conditionType)
		if isConditionStale(rc.ObservedGeneration, record) {
			return newPendingCondition(dnsrecordv1alpha1.CNAMERecord_Kind, record.Name), nil
		}
		if cond := formatChildCondition(dnsrecordv1alpha1.CNAMERecord_Kind, record.Name, metav1.ConditionStatus(rc.Status), string(rc.Reason), rc.Message); cond != nil {
			return cond, nil
		}
	}
	return nil, nil
}

// checkCertificate returns why the Certificate is not ready, or nil when it is.
func checkCertificate(ctx context.Context, k8sClient client.Client, key types.NamespacedName) (*metav1.Condition, error) {
	cert := &cmapi.Certificate{}
	if cond, err := getChildResource(ctx, k8sClient, rmanagers.Certificate, key, cert); cond != nil || err != nil {
		return cond, err
	}

	var ready *cmapi.CertificateCondition
	for i := range cert.Status.Conditions {
		if cert.Status.Conditions[i].Type == cmapi.CertificateConditionReady {
			ready = &cert.Status.Conditions[i]
		}
	}
	if ready == nil || isConditionStale(ready.ObservedGeneration, cert) {
		return newPendingCondition(rmanagers.Certificate, cert.Name), nil
	}
	return formatChildCondition(rmanagers.Certificate, cert.Name, metav1.ConditionStatus(ready.Status), ready.Reason, ready.Message), nil
}

// checkDomainMapping returns why the DomainMapping is not ready, or nil when it is.
func checkDomainMapping(ctx context.Context, k8sClient client.Client, key types.NamespacedName) (*metav1.Condition, error) {
	dm := &knativev1beta1.DomainMapping{}
	if cond, err := getChildResource(ctx, k8sClient, rmanagers.DomainMapping, key, dm); cond != nil || err != nil {
		return cond, err
	}

	ready := dm.Status.GetCondition(knativev1beta1.DomainMappingConditionReady)
	if ready == nil || dm.Status.ObservedGeneration < dm.Generation {
		return newPendingCondition(rmanagers.DomainMapping, dm.Name), nil
	}
	return formatChildCondition(rmanagers.DomainMapping, dm.Name, metav1.ConditionStatus(ready.Status), ready.Reason, ready.Message), nil
}
