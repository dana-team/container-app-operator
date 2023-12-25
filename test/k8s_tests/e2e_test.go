package k8s_tests

import (
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	mock "github.com/dana-team/container-app-operator/test/k8s_tests/mocks"
	utilst "github.com/dana-team/container-app-operator/test/k8s_tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
	"time"
)

const (
	TimeoutCapp          = 60 * time.Second
	CappCreationInterval = 2 * time.Second
)

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)

	SetDefaultEventuallyTimeout(time.Second * 2)
	RunSpecs(t, "Capp Suite")
}

var _ = Describe("Validate Capp adapter", func() {

	It("Should succeed all adapter functions", func() {
		baseCapp := mock.CreateBaseCapp()
		desiredCapp := utilst.CreateCapp(k8sClient, baseCapp)
		assertionCapp := &rcsv1alpha1.Capp{}

		By("Checks unique creation of Capp")
		assertionCapp = utilst.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace)
		Expect(assertionCapp.Name).ShouldNot(Equal(baseCapp.Name))

		By("Checks if Capp Updated successfully")
		desiredCapp = assertionCapp.DeepCopy()
		desiredCapp.Spec.ScaleMetric = mock.RPSScaleMetric
		utilst.UpdateCapp(k8sClient, desiredCapp)
		Eventually(func() string {
			assertionCapp = utilst.GetCapp(k8sClient, assertionCapp.Name, assertionCapp.Namespace)
			return assertionCapp.Spec.ScaleMetric
		}, TimeoutCapp, CappCreationInterval).Should(Equal(mock.RPSScaleMetric), "Should fetch capp.")

		By("Checks if deleted successfully")
		utilst.DeleteCapp(k8sClient, assertionCapp)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, assertionCapp)
		}, TimeoutCapp, CappCreationInterval).Should(BeFalse(), "Should not find a resource.")
	})
})
