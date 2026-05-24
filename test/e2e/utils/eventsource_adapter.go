package utils

import (
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetPingSource fetches and returns an existing instance of a Knative PingSource.
func GetPingSource(k8sClient client.Client, name string, namespace string) *sourcesv1.PingSource {
	pingSource := &sourcesv1.PingSource{}
	GetResource(k8sClient, pingSource, name, namespace)
	return pingSource
}
