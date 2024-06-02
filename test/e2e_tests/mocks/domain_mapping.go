package mocks

import (
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
