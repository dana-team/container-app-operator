package utils

import (
	"context"

	"github.com/dana-team/container-app-operator/test/e2e/consts"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateCapp creates a new Capp instance with a unique name and returns it.
func CreateCapp(g Gomega, k8sClient client.Client, capp *cappv1alpha1.Capp) *cappv1alpha1.Capp {
	GinkgoHelper()
	cappName := GenerateCappName()
	newCapp := capp.DeepCopy()
	newCapp.Name = cappName
	Expect(k8sClient.Create(context.Background(), newCapp)).To(Succeed())

	g.Eventually(func() (bool, error) {
		return ResourceExists(k8sClient, newCapp)
	}, consts.Timeout, consts.Interval).Should(BeTrue(), "Capp should exist")

	return newCapp
}

// DeleteCapp deletes an existing Capp instance.
func DeleteCapp(g Gomega, k8sClient client.Client, capp *cappv1alpha1.Capp) {
	GinkgoHelper()
	Expect(k8sClient.Delete(context.Background(), capp)).To(Succeed())
	g.Eventually(func() (bool, error) {
		return ResourceExists(k8sClient, capp)
	}, consts.TimeoutCapp, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")
}

// GenerateCappName generates a new name for Capp.
func GenerateCappName() string {
	return generateName(consts.CappName)
}

// GetCapp fetches and returns an existing instance of a Capp.
func GetCapp(k8sClient client.Client, name string, namespace string) *cappv1alpha1.Capp {
	GinkgoHelper()
	capp := &cappv1alpha1.Capp{}
	GetResource(k8sClient, capp, name, namespace)
	return capp
}

// GenerateUniqueCappName generates a unique Capp name.
func GenerateUniqueCappName(baseCappName string) string {
	randString := generateRandomString(consts.RandStrLength)
	return baseCappName + "-" + randString
}
