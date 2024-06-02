package utils

import (
	certv1alpha1 "github.com/dana-team/certificate-operator/api/v1alpha1"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	mock "github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	dnsv1alpha1 "github.com/dana-team/provider-dns/apis/recordset/v1alpha1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateCappWithHTTPHostname creates a Capp with a Hostname.
func CreateCappWithHTTPHostname(k8sClient client.Client) (*cappv1alpha1.Capp, string) {
	httpsCapp := mock.CreateBaseCapp()
	hostname := GenerateRouteHostname()

	httpsCapp.Spec.RouteSpec.Hostname = hostname

	return CreateCapp(k8sClient, httpsCapp), hostname
}

// CreateHTTPSCapp creates a Capp with a Hostname, TLS Enabled and TLSSecret.
func CreateHTTPSCapp(k8sClient client.Client) (*cappv1alpha1.Capp, string, string) {
	httpsCapp := mock.CreateBaseCapp()
	hostname := GenerateRouteHostname()
	secretName := GenerateSecretName()

	httpsCapp.Spec.RouteSpec.Hostname = hostname
	httpsCapp.Spec.RouteSpec.TlsSecret = secretName
	httpsCapp.Spec.RouteSpec.TlsEnabled = true

	return CreateCapp(k8sClient, httpsCapp), hostname, secretName
}

// GetDomainMapping fetches and returns an existing instance of a DomainMapping.
func GetDomainMapping(k8sClient client.Client, name string, namespace string) *knativev1beta1.DomainMapping {
	domainMapping := &knativev1beta1.DomainMapping{}
	GetResource(k8sClient, domainMapping, name, namespace)
	return domainMapping
}

// GetARecordSet fetches and returns an existing instance of an ARecordSet.
func GetARecordSet(k8sClient client.Client, name string) *dnsv1alpha1.ARecordSet {
	aRecordSet := &dnsv1alpha1.ARecordSet{}
	GetClusterResource(k8sClient, aRecordSet, name)
	return aRecordSet
}

// GetCertificate fetches and returns an existing instance of a Certificate.
func GetCertificate(k8sClient client.Client, name string, namespace string) *certv1alpha1.Certificate {
	certificate := &certv1alpha1.Certificate{}
	GetResource(k8sClient, certificate, name, namespace)
	return certificate
}
