package utils

import (
	"context"
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	TimeoutCapp          = 30 * time.Second
	CappCreationInterval = 2 * time.Second
	CappName             = "capp-default-test"
)

// CreateCapp creates a new Capp instance with a unique name and returns it.
func CreateCapp(k8sClient client.Client, capp *rcsv1alpha1.Capp) *rcsv1alpha1.Capp {
	cappName := GenerateCappName()
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

// GenerateCappName generates a new secret name by calling
// generateName with the predefined RouteTlsSecret as the baseName.
func GenerateCappName() string {
	return generateName(CappName)
}

// GetCapp fetch existing and return an instance of Capp.
func GetCapp(k8sClient client.Client, name string, namespace string) *rcsv1alpha1.Capp {
	capp := &rcsv1alpha1.Capp{}
	GetResource(k8sClient, capp, name, namespace)
	return capp
}
