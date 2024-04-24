package mocks

import (
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ElasticType       = "elastic"
	ElasticHost       = "1.2.3.4"
	MainIndex         = "main"
	ElasticUserName   = "elastic"
	ElasticSecretName = "credentials"
)

// CreateElasticLogSpec creates a Logging Spec for Elasticsearch.
func CreateElasticLogSpec() cappv1alpha1.LogSpec {
	return cappv1alpha1.LogSpec{
		Type:           ElasticType,
		Host:           ElasticHost,
		Index:          MainIndex,
		User:           ElasticUserName,
		PasswordSecret: ElasticSecretName,
	}
}

// CreateSyslogNGOutputObject returns a SyslogNGOutput object.
func CreateSyslogNGOutputObject(name string) *loggingv1beta1.SyslogNGOutput {
	return &loggingv1beta1.SyslogNGOutput{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: NSName,
		},
	}
}

// CreateSyslogNGFlowObject returns a SyslogNGFlow object.
func CreateSyslogNGFlowObject(name string) *loggingv1beta1.SyslogNGFlow {
	return &loggingv1beta1.SyslogNGFlow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: NSName,
		},
	}
}

func CreateElasticSecretObject() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ElasticSecretName,
			Namespace: NSName,
		},
		Type: "Opaque",
		Data: map[string][]byte{ElasticUserName: []byte(SecretValue)},
	}
}
