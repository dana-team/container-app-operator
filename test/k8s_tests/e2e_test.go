package k8s_tests

import (
	"context"
	mock "github.com/dana-team/container-app-operator/test/k8s_tests/mocks"
	utilst "github.com/dana-team/container-app-operator/test/k8s_tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"
)

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)

	SetDefaultEventuallyTimeout(time.Second * 2)
	RunSpecs(t, "Capp Suite")
}

var _ = Describe("Validate Suite acted correctly ", func() {

	It("Should have created a namespace", func() {
		ns := &corev1.Namespace{}
		err := K8sClient.Get(context.Background(), client.ObjectKey{Name: mock.NsName}, ns)
		Expect(err).NotTo(HaveOccurred())
		Expect(ns).NotTo(BeNil())
	})
})

var _ = Describe("Validate Capp adapter", func() {

	It("Should succeed all adapter functions", func() {
		baseCapp := mock.CreateBaseCapp()
		newCapp := utilst.CreateCapp(K8sClient, baseCapp)
		By("Checks unique creation of Capp")
		Expect(newCapp.Name).ShouldNot(Equal(baseCapp.Name))
		newCapp.Spec.ScaleMetric = "rps"
		utilst.UpdateCapp(K8sClient, newCapp)
		By("Checks if Capp Updated successfully")
		Eventually(func() string {
			newCapp = utilst.GetCapp(K8sClient, newCapp.Name, newCapp.Namespace)
			return newCapp.Spec.ScaleMetric
		}, 16*time.Second, 2*time.Second).Should(Equal("rps"), "Should fetch capp")
		utilst.DeleteCapp(K8sClient, newCapp)
		By("Checks if deleted successfully")
		Expect(utilst.DoesResourceExists(K8sClient, newCapp)).Should(BeFalse())
	})
})
