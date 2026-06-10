package mocks

import (
	"github.com/dana-team/container-app-operator/test/e2e/consts"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kafkasecurity "knative.dev/eventing-kafka-broker/control-plane/pkg/security"
)

const (
	KafkaSecretName    = "kafka-creds-e2e"
	kafkaSASLUser      = "user"
	kafkaSASLPassword  = "password"
	kafkaSASLMechanism = "SCRAM-SHA-256"
)

// CreateKafkaCredentialsSecret returns a Secret with SASL keys required by kafka sources.
func CreateKafkaCredentialsSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      KafkaSecretName,
			Namespace: consts.NSName,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			kafkasecurity.SaslUserKey:      []byte(kafkaSASLUser),
			kafkasecurity.SaslPasswordKey:  []byte(kafkaSASLPassword),
			kafkasecurity.SaslMechanismKey: []byte(kafkaSASLMechanism),
		},
	}
}
