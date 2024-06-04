package mocks

import (
	certv1alpha1 "github.com/dana-team/certificate-operator/api/v1alpha1"
	dnsv1alpha1 "github.com/dana-team/provider-dns/apis/recordset/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
)

// CreateDomainMappingObject returns an empty DomainMapping object.
func CreateDomainMappingObject(name string) *knativev1beta1.DomainMapping {
	return &knativev1beta1.DomainMapping{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: NSName,
		},
	}
}

// CreateCertificateObject returns an empty DomainMapping object.
func CreateCertificateObject(name string) *certv1alpha1.Certificate {
	return &certv1alpha1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: NSName,
		},
	}
}

// CreateARecordSetObject returns an empty ARecordSet object.
func CreateARecordSetObject(name string) *dnsv1alpha1.ARecordSet {
	return &dnsv1alpha1.ARecordSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}
