package utils

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"math/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	charset        = "abcdefghijklmnopqrstuvwxyz0123456789"
	RandStrLength  = 10
	RouteHostname  = "test.dev"
	RouteTlsSecret = "https-capp-secret"
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

// generateName generates a new name by combining the given baseName
// with a randomly generated string of a specified length.
func generateName(baseName string) string {
	randString := generateRandomString(RandStrLength)
	return baseName + "-" + randString
}

// GetDomainMapping fetch existing and return an instance of DomainMapping.
func GetDomainMapping(k8sClient client.Client, name string, namespace string) *knativev1beta1.DomainMapping {
	domainMapping := &knativev1beta1.DomainMapping{}
	GetResource(k8sClient, domainMapping, name, namespace)
	return domainMapping
}

// CreateSecret creates a new secret.
func CreateSecret(k8sClient client.Client, secret *v1.Secret) {
	Expect(k8sClient.Create(context.Background(), secret)).To(Succeed())
}

// GenerateRouteHostname generates a new route hostname by calling
// generateName with the predefined RouteHostname as the baseName.
func GenerateRouteHostname() string {
	return generateName(RouteHostname)
}

// GenerateSecretName generates a new secret name by calling
// generateName with the predefined RouteTlsSecret as the baseName.
func GenerateSecretName() string {
	return generateName(RouteTlsSecret)
}
