package utils

import (
	"context"
	"time"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	mock "github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	timeoutCapp          = 120 * time.Second
	cappCreationInterval = 2 * time.Second
)

// CreateCapp creates a new Capp instance with a unique name and returns it.
func CreateCapp(k8sClient client.Client, capp *cappv1alpha1.Capp) *cappv1alpha1.Capp {
	cappName := GenerateCappName()
	newCapp := capp.DeepCopy()
	newCapp.Name = cappName
	Expect(k8sClient.Create(context.Background(), newCapp)).To(Succeed())
	Eventually(func() string {
		return GetCapp(k8sClient, newCapp.Name, newCapp.Namespace).Status.KnativeObjectStatus.ConfigurationStatusFields.LatestReadyRevisionName
	}, timeoutCapp, cappCreationInterval).ShouldNot(Equal(""), "Should fetch capp")
	return newCapp
}

// DeleteCapp deletes an existing Capp instance.
func DeleteCapp(k8sClient client.Client, capp *cappv1alpha1.Capp) {
	Expect(k8sClient.Delete(context.Background(), capp)).To(Succeed())
	Eventually(func() bool {
		return DoesResourceExist(k8sClient, capp)
	}, timeoutCapp, cappCreationInterval).ShouldNot(BeTrue(), "Should not find a resource.")
}

// GenerateCappName generates a new name for Capp.
func GenerateCappName() string {
	return generateName(mock.CappName)
}

// GetCapp fetches and returns an existing instance of a Capp.
func GetCapp(k8sClient client.Client, name string, namespace string) *cappv1alpha1.Capp {
	capp := &cappv1alpha1.Capp{}
	GetResource(k8sClient, capp, name, namespace)
	return capp
}
