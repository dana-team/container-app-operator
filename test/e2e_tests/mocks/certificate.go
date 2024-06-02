package mocks

import (
	certv1alpha1 "github.com/dana-team/certificate-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateCertificateObject returns an empty DomainMapping object.
func CreateCertificateObject(name string) *certv1alpha1.Certificate {
	return &certv1alpha1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: NSName,
		},
	}
}
