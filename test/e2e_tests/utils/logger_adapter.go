package utils

import (
	"github.com/dana-team/container-app-operator/api/v1alpha1"
	mock "github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateCappWithLogger creates a Capp instance with the specified logger type and returns the created Capp object.
func CreateCappWithLogger(logType string, client client.Client) *v1alpha1.Capp {
	capp := mock.CreateBaseCapp()
	switch logType {
	case mock.ElasticType:
		capp.Spec.LogSpec = mock.CreateElasticLogSpec()
	}
	return CreateCapp(client, capp)
}

// CreateCredentialsSecret creates a Kubernetes secret containing credentials for the specified logger type.
func CreateCredentialsSecret(logType string, client client.Client) {
	switch logType {
	case mock.ElasticType:
		elasticSecret := mock.CreateElasticSecretObject()
		CreateSecret(client, elasticSecret)
	}
}

// GetSyslogNGOutput fetches existing and returns an instance of SyslogNGOutput.
func GetSyslogNGOutput(k8sClient client.Client, name string, namespace string) *loggingv1beta1.SyslogNGOutput {
	syslogNGOutput := &loggingv1beta1.SyslogNGOutput{}
	GetResource(k8sClient, syslogNGOutput, name, namespace)
	return syslogNGOutput
}

// GetSyslogNGFlow fetches existing and returns an instance of SyslogNGFlow.
func GetSyslogNGFlow(k8sClient client.Client, name string, namespace string) *loggingv1beta1.SyslogNGFlow {
	syslogNGFlow := &loggingv1beta1.SyslogNGFlow{}
	GetResource(k8sClient, syslogNGFlow, name, namespace)
	return syslogNGFlow
}
