package mocks

import (
	certv1alpha1 "github.com/dana-team/certificate-operator/api/v1alpha1"
	dnsrecordv1alpha1 "github.com/dana-team/provider-dns/apis/record/v1alpha1"
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

// CreateCNAMERecordObject returns an empty ARecordSet object.
func CreateCNAMERecordObject(name string) *dnsrecordv1alpha1.CNAMERecord {
	return &dnsrecordv1alpha1.CNAMERecord{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}
