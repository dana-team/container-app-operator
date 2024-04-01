package mocks

import (
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ElasticType       = "elastic"
	SplunkType        = "splunk"
	ElasticHost       = "20.76.217.187"
	SplunkHost        = "74.234.208.141"
	MainIndex         = "main"
	ElasticUserName   = "elastic"
	SplunkUserName    = "bGFiZXI="
	SplunkPassword    = "QWExMjM0NTYh"
	HecTokenKey       = "hec_token"
	SplunkHecToken    = "ODVhZTc2YmYtYjYyMS00MDk5LWIyYzMtOGI5OTk3NTA0OTgy"
	ElasticSecretName = "quickstart-es-elastic-user"
	SplunkSecretName  = "splunk-single-standalone-secrets"
	UsernameKey       = "username"
	PasswordKey       = "password"
	SplunkHecTokenKey = "SplunkHecToken"
)

func CreateElasticLogSpec() cappv1alpha1.LogSpec {
	return cappv1alpha1.LogSpec{
		Type:               ElasticType,
		Host:               ElasticHost,
		SSLVerify:          false,
		Index:              MainIndex,
		UserName:           ElasticUserName,
		PasswordSecretName: ElasticSecretName,
	}
}

func CreateSplunkLogSpec() cappv1alpha1.LogSpec {
	return cappv1alpha1.LogSpec{
		Type:               SplunkType,
		Host:               SplunkHost,
		SSLVerify:          false,
		Index:              MainIndex,
		HecTokenSecretName: SplunkSecretName,
	}
}

func CreateOutputObject(outputName string) *loggingv1beta1.Output {
	return &loggingv1beta1.Output{
		ObjectMeta: metav1.ObjectMeta{
			Name:      outputName,
			Namespace: NSName,
		},
	}
}

func CreateFlowObject(flowName string) *loggingv1beta1.Flow {
	return &loggingv1beta1.Flow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      flowName,
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

func CreateSplunkSecretObject() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      SplunkSecretName,
			Namespace: NSName,
		},
		Type: "Opaque",
		Data: map[string][]byte{HecTokenKey: []byte(SplunkHecToken), UsernameKey: []byte(SplunkUserName), PasswordKey: []byte(SplunkPassword), SplunkHecTokenKey: []byte(SplunkHecToken)},
	}
}
