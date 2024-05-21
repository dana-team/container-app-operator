package utils

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	dnsv1alpha1 "sigs.k8s.io/external-dns/endpoint"
)

// GetDNSEndpoint fetches and returns an existing instance of a DNSEndpoint.
func GetDNSEndpoint(k8sClient client.Client, name string, namespace string) *dnsv1alpha1.DNSEndpoint {
	dnsEndpoint := &dnsv1alpha1.DNSEndpoint{}
	GetResource(k8sClient, dnsEndpoint, name, namespace)
	return dnsEndpoint
}
