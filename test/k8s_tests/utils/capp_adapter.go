package utils

import (
	"context"
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	knativev1alphav1 "knative.dev/serving/pkg/apis/serving/v1alpha1"
	"math/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	charset              = "abcdefghijklmnopqrstuvwxyz0123456789"
	RandStrLength        = 10
	TimeoutCapp          = 30 * time.Second
	CappCreationInterval = 2 * time.Second
	RouteHostname        = "test.dev"
	RouteTlsSecret       = "https-capp-secret"
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

// CreateCapp creates a new Capp instance with a unique name and returns it.
func CreateCapp(k8sClient client.Client, capp *rcsv1alpha1.Capp) *rcsv1alpha1.Capp {
	cappName := generateName(capp.Name)
	newCapp := capp.DeepCopy()
	newCapp.Name = cappName
	Expect(k8sClient.Create(context.Background(), newCapp)).To(Succeed())
	Eventually(func() string {
		return GetCapp(k8sClient, newCapp.Name, newCapp.Namespace).Status.KnativeObjectStatus.ConfigurationStatusFields.LatestReadyRevisionName
	}, TimeoutCapp, CappCreationInterval).ShouldNot(Equal(""), "Should fetch capp")
	return newCapp

}

// UpdateCapp updates an existing Capp instance.
func UpdateCapp(k8sClient client.Client, capp *rcsv1alpha1.Capp) {
	Expect(k8sClient.Update(context.Background(), capp)).To(Succeed())
}

// DeleteCapp deletes an existing Capp instance.
func DeleteCapp(k8sClient client.Client, capp *rcsv1alpha1.Capp) {
	Expect(k8sClient.Delete(context.Background(), capp)).To(Succeed())

}

// GetCapp fetch existing and return an instance of Capp.
func GetCapp(k8sClient client.Client, name string, namespace string) *rcsv1alpha1.Capp {
	capp := &rcsv1alpha1.Capp{}
	GetResource(k8sClient, capp, name, namespace)
	return capp
}

// GetDomainMapping fetch existing and return an instance of DomainMapping.
func GetDomainMapping(k8sClient client.Client, name string, namespace string) *knativev1alphav1.DomainMapping {
	domainMapping := &knativev1alphav1.DomainMapping{}
	GetResource(k8sClient, domainMapping, name, namespace)
	return domainMapping
}

// CreateSecret creates a new secret.
func CreateSecret(k8sClient client.Client, secret v1.Secret) {
	Expect(k8sClient.Create(context.Background(), &secret)).To(Succeed())
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
