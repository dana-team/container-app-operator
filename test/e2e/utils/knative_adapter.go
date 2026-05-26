package utils

import (
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetKSVC fetches and returns an existing instance of a Knative Serving.
func GetKSVC(k8sClient client.Client, name string, namespace string) *knativev1.Service {
	ksvc := &knativev1.Service{}
	GetResource(k8sClient, ksvc, name, namespace)
	return ksvc
}

// GetRevision fetches and returns an existing instance of a Knative Revision.
func GetRevision(k8sClient client.Client, name string, namespace string) *knativev1.Revision {
	revision := &knativev1.Revision{}
	GetResource(k8sClient, revision, name, namespace)
	return revision
}
