package mocks

import (
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/test/e2e/consts"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateElasticLogSpec creates a Logging Spec for Elasticsearch.
func CreateElasticLogSpec() cappv1alpha1.LogSpec {
	return cappv1alpha1.LogSpec{
		Type:           cappv1alpha1.LogTypeElastic,
		Host:           consts.ElasticHost,
		Index:          consts.MainIndex,
		User:           consts.ElasticUserName,
		PasswordSecret: consts.ElasticSecretName + "-elastic",
	}
}

// CreateElasticDataStreamLogSpec creates a Logging Spec for Elasticsearch Data Stream.
func CreateElasticDataStreamLogSpec() cappv1alpha1.LogSpec {
	return cappv1alpha1.LogSpec{
		Type:           cappv1alpha1.LogTypeElasticDataStream,
		Host:           consts.ElasticDataStreamURL,
		User:           consts.ElasticUserName,
		PasswordSecret: consts.ElasticSecretName + "-datastream",
	}
}

// CreateSyslogNGOutputObject returns a SyslogNGOutput object.
func CreateSyslogNGOutputObject(name string) *loggingv1beta1.SyslogNGOutput {
	return &loggingv1beta1.SyslogNGOutput{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: consts.NSName,
		},
	}
}

// CreateSyslogNGFlowObject returns an empty SyslogNGFlow object.
func CreateSyslogNGFlowObject(name string) *loggingv1beta1.SyslogNGFlow {
	return &loggingv1beta1.SyslogNGFlow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: consts.NSName,
		},
	}
}

// CreateElasticSecretObject returns a Secret for Elasticsearch logging.
func CreateElasticSecretObject() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      consts.ElasticSecretName + "-elastic",
			Namespace: consts.NSName,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{consts.ElasticUserName: []byte(consts.SecretValue)},
	}
}

// CreateElasticDataStreamSecretObject returns a Secret for Elasticsearch Data Stream logging.
func CreateElasticDataStreamSecretObject() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      consts.ElasticSecretName + "-datastream",
			Namespace: consts.NSName,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{consts.ElasticUserName: []byte(consts.SecretValue)},
	}
}
