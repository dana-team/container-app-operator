package mocks

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	dnsv1alpha1 "sigs.k8s.io/external-dns/endpoint"
)

// CreateDNSEndpointObject returns an empty DNSEndpoint object.
func CreateDNSEndpointObject(dnsEndpointName string) *dnsv1alpha1.DNSEndpoint {
	return &dnsv1alpha1.DNSEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dnsEndpointName,
			Namespace: NSName,
		},
	}
}
