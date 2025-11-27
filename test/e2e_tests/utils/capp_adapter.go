package utils

import (
	"context"

	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateCapp creates a new Capp instance with a unique name and returns it.
func CreateCapp(k8sClient client.Client, capp *cappv1alpha1.Capp) *cappv1alpha1.Capp {
	cappName := GenerateCappName()
	newCapp := capp.DeepCopy()
	newCapp.Name = cappName
	Expect(k8sClient.Create(context.Background(), newCapp)).To(Succeed())

	Eventually(func() bool {
		return DoesResourceExist(k8sClient, newCapp)
	}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Capp should exist")

	return newCapp
}

// DeleteCapp deletes an existing Capp instance.
func DeleteCapp(k8sClient client.Client, capp *cappv1alpha1.Capp) {
	Expect(k8sClient.Delete(context.Background(), capp)).To(Succeed())
	Eventually(func() bool {
		return DoesResourceExist(k8sClient, capp)
	}, testconsts.TimeoutCapp, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")
}

// GenerateCappName generates a new name for Capp.
func GenerateCappName() string {
	return generateName(testconsts.CappName)
}

// GetCapp fetches and returns an existing instance of a Capp.
func GetCapp(k8sClient client.Client, name string, namespace string) *cappv1alpha1.Capp {
	capp := &cappv1alpha1.Capp{}
	GetResource(k8sClient, capp, name, namespace)
	return capp
}

// GenerateUniqueCappName generates a unique Capp name.
func GenerateUniqueCappName(baseCappName string) string {
	randString := generateRandomString(testconsts.RandStrLength)
	return baseCappName + "-" + randString
}

// UpdateCapp updates the provided Capp instance in the Kubernetes cluster, and returns it.
func UpdateCapp(k8sClient client.Client, capp *cappv1alpha1.Capp) {
	Expect(k8sClient.Update(context.Background(), capp)).To(Succeed())
}
