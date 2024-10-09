package utils

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	charset        = "abcdefghijklmnopqrstuvwxyz0123456789"
	randStrLength  = 10
	routeHostname  = "test.dev"
	routeTLSSecret = "https-capp-secret"
)

var seededRand = rand.New(rand.NewSource(time.Now().UnixNano()))

// generateRandomString returns a random string of the specified length using characters from the charset.
func generateRandomString(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

// DoesResourceExist checks if a given Kubernetes object exists in the cluster.
func DoesResourceExist(k8sClient client.Client, obj client.Object) bool {
	copyObject := obj.DeepCopyObject().(client.Object)
	key := client.ObjectKeyFromObject(copyObject)
	err := k8sClient.Get(context.Background(), key, copyObject)
	if errors.IsNotFound(err) {
		return false
	} else if err != nil {
		Fail(fmt.Sprintf("The function failed with error: \n %s", err.Error()))
	}
	return true
}

// GetResource fetches an existing resource and returns an instance of it.
func GetResource(k8sClient client.Client, obj client.Object, name, namespace string) {
	Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: name, Namespace: namespace}, obj))
}

// GetClusterResource fetches an existing Cluster resource and returns an instance of it.
func GetClusterResource(k8sClient client.Client, obj client.Object, name string) {
	Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: name}, obj))
}

// generateName generates a new name by combining the given baseName
// with a randomly generated string of a specified length.
func generateName(baseName string) string {
	randString := generateRandomString(randStrLength)
	return baseName + "-" + randString
}

// GetSecret fetches and returns an existing instance of a Secret.
func GetSecret(k8sClient client.Client, name string, namespace string) *corev1.Secret {
	secret := &corev1.Secret{}
	GetResource(k8sClient, secret, name, namespace)
	return secret
}

// CreateSecret creates a new secret.
func CreateSecret(k8sClient client.Client, secret *corev1.Secret) {
	Expect(k8sClient.Create(context.Background(), secret)).To(Succeed())
}

// CreateConfigMap creates a new configMap.
func CreateConfigMap(k8sClient client.Client, configMap *corev1.ConfigMap) {
	Expect(k8sClient.Create(context.Background(), configMap)).To(SatisfyAny(BeNil(), WithTransform(errors.IsAlreadyExists, BeTrue())))
}

// GenerateRouteHostname generates a new route hostname by calling
// generateName with the predefined RouteHostname as the baseName.
func GenerateRouteHostname() string {
	return generateName(routeHostname)
}

// GenerateSecretName generates a new secret name by calling
// generateName with the predefined RouteTlsSecret as the baseName.
func GenerateSecretName() string {
	return generateName(routeTLSSecret)
}

// GenerateCertSecretName generates a capp cert secret name.
func GenerateCertSecretName(hostname string) string {
	return fmt.Sprintf("%s-tls", hostname)
}

// UpdateResource updates an existing resource.
func UpdateResource(k8sClient client.Client, object client.Object) error {
	return k8sClient.Update(context.Background(), object)
}
