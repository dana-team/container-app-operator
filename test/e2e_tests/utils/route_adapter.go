package utils

import (
	"strings"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"

	dnsrecordv1alpha1 "github.com/dana-team/provider-dns/apis/record/v1alpha1"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	mock "github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
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
func CreateHTTPSCapp(k8sClient client.Client) (*cappv1alpha1.Capp, string) {
	httpsCapp := mock.CreateBaseCapp()
	hostname := GenerateRouteHostname()

	httpsCapp.Spec.RouteSpec.Hostname = hostname
	httpsCapp.Spec.RouteSpec.TlsEnabled = true

	return CreateCapp(k8sClient, httpsCapp), hostname
}

// GetDomainMapping fetches and returns an existing instance of a DomainMapping.
func GetDomainMapping(k8sClient client.Client, name string, namespace string) *knativev1beta1.DomainMapping {
	domainMapping := &knativev1beta1.DomainMapping{}
	GetResource(k8sClient, domainMapping, name, namespace)
	return domainMapping
}

// GetDNSRecord fetches and returns an existing instance of an CNAMERecord.
func GetDNSRecord(k8sClient client.Client, name string) *dnsrecordv1alpha1.CNAMERecord {
	cnameRecord := &dnsrecordv1alpha1.CNAMERecord{}
	GetClusterResource(k8sClient, cnameRecord, name)
	return cnameRecord
}

// GetCertificate fetches and returns an existing instance of a Certificate.
func GetCertificate(k8sClient client.Client, name string, namespace string) *cmapi.Certificate {
	certificate := &cmapi.Certificate{}
	GetResource(k8sClient, certificate, name, namespace)
	return certificate
}

// GenerateResourceName generates the hostname based on the provided suffix and a dot (".") trailing character.
// It returns the adjusted hostname, where the suffix (minus the trailing character) is added if not already present.
func GenerateResourceName(hostname, suffix string) string {
	suffixWithoutTrailingChar := suffix[:len(suffix)-len(".")]

	if !strings.HasSuffix(hostname, suffixWithoutTrailingChar) {
		return hostname + "." + suffixWithoutTrailingChar
	}

	return hostname
}
