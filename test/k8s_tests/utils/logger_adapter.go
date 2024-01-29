package utils

import (
	"github.com/dana-team/container-app-operator/api/v1alpha1"
	mock "github.com/dana-team/container-app-operator/test/k8s_tests/mocks"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateCappWithLogger creates a Capp instance with the specified logger type and returns the created Capp object.
func CreateCappWithLogger(logType string, client client.Client) *v1alpha1.Capp {
	capp := mock.CreateBaseCapp()
	switch logType {
	case mock.ElasticType:
		capp.Spec.LogSpec = mock.CreateElasticLogSpec()
	case mock.SplunkType:
		capp.Spec.LogSpec = mock.CreateSplunkLogSpec()
	}
	return CreateCapp(client, capp)
}

// CreateCredentialsSecret creates a Kubernetes secret containing credentials for the specified logger type.
func CreateCredentialsSecret(logType string, client client.Client) {
	switch logType {
	case mock.ElasticType:
		elasticSecret := mock.CreateElasticSecretObject()
		CreateSecret(client, elasticSecret)
	case mock.SplunkType:
		splunkSecret := mock.CreateSplunkSecretObject()
		CreateSecret(client, splunkSecret)
	}
}

// GetOutput fetches existing and returns an instance of Output.
func GetOutput(k8sClient client.Client, name string, namespace string) *loggingv1beta1.Output {
	output := &loggingv1beta1.Output{}
	GetResource(k8sClient, output, name, namespace)
	return output
}

// GetFlow fetches existing and returns an instance of Flow.
func GetFlow(k8sClient client.Client, name string, namespace string) *loggingv1beta1.Flow {
	flow := &loggingv1beta1.Flow{}
	GetResource(k8sClient, flow, name, namespace)
	return flow
}
