package utils

import (
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetDomainMapping fetches and returns an existing instance of a DomainMapping.
func GetDomainMapping(k8sClient client.Client, name string, namespace string) *knativev1beta1.DomainMapping {
	domainMapping := &knativev1beta1.DomainMapping{}
	GetResource(k8sClient, domainMapping, name, namespace)
	return domainMapping
}
