package k8s_tests

import (
	_ "github.com/dana-team/container-app-operator/api/v1alpha1"
	"context"
	"testing"
	"time"
	mock "github.com/dana-team/container-app-operator/test/k8s_tests/mocks"
	utilst "github.com/dana-team/container-app-operator/test/k8s_tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	TimeoutCapp          = 60 * time.Second
	CappCreationInterval = 2 * time.Second
	EnabledState         = "enabled"
	DisabledState        = "disabled"
)

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)

	SetDefaultEventuallyTimeout(time.Second * 2)
	RunSpecs(t, "Capp Suite")
}

var _ = Describe("Validate capp creation", func() {
	It("Should validate capp spec", func() {
		baseCapp := mock.CreateBaseCapp()
		By("Creating Capp with no scale metric")
		desiredCapp := utilst.CreateCapp(k8sClient, baseCapp)
		Expect(desiredCapp.Spec.ScaleMetric).ShouldNot(Equal(nil))

		By("Creating Capp with unsupported scale metric")
		baseCapp.Spec.ScaleMetric = UnsupportedScaleMetric
		Expect(k8sClient.Create(context.Background(), baseCapp)).ShouldNot(Equal(nil))
	})
})

var _ = Describe("Validate Capp adapter", func() {

	It("Should succeed all adapter functions", func() {
		baseCapp := mock.CreateBaseCapp()
		desiredCapp := utilst.CreateCapp(k8sClient, baseCapp)

		By("Checks unique creation of Capp")
		assertionCapp := utilst.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace)
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
		}, TimeoutCapp, CappCreationInterval).ShouldNot(BeTrue(), "Should not find a resource.")
	})

	It("Validate state functionality", func() {
		By("Creating a capp instance")
		testCapp := mock.CreateBaseCapp()
		assertionCapp := createAndGetCapp(testCapp)

		By("Checking if the capp state is enabled")
		Eventually(func() string {
			capp := utilst.GetCapp(k8sClient, assertionCapp.Name, assertionCapp.Namespace)
			return capp.Status.StateStatus.State
		}, TimeoutCapp, CappCreationInterval).Should(Equal(EnabledState))

		By("Checking if the ksvc was created successfully")
		ksvcObject := mock.CreateKnativeServiceObject(assertionCapp.Name)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, ksvcObject)
		}, TimeoutCapp, CappCreationInterval).Should(BeTrue(), "Should find a resource.")

		By("Checking if the revision is ready")
		revisionName := assertionCapp.Name + firstRevisionSuffix
		checkRevisionReadiness(revisionName, true)

		By("Updating the capp status to be disabled")
		assertionCapp.Spec.State = DisabledState
		utilst.UpdateCapp(k8sClient, assertionCapp)

		By("Checking if the capp state is disabled")
		Eventually(func() string {
			capp := utilst.GetCapp(k8sClient, assertionCapp.Name, assertionCapp.Namespace)
			return capp.Status.StateStatus.State
		}, TimeoutCapp, CappCreationInterval).Should(Equal(DisabledState))

		By("Checking if the ksvc and the revision were deleted successfully")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, ksvcObject)
		}, TimeoutCapp, CappCreationInterval).ShouldNot(BeTrue(), "Should not find a resource.")
		Eventually(func() bool {
			revision := mock.CreateRevisionObject(revisionName)
			return utilst.DoesResourceExist(k8sClient, revision)
		}, TimeoutCapp, CappCreationInterval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Updating the capp status to be enabled")
		assertionCapp = utilst.GetCapp(k8sClient, assertionCapp.Name, assertionCapp.Namespace)
		assertionCapp.Spec.State = EnabledState
		utilst.UpdateCapp(k8sClient, assertionCapp)

		By("Checking if the ksvc was recreated successfully")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, ksvcObject)
		}, TimeoutCapp, CappCreationInterval).Should(BeTrue(), "Should find a resource.")

		By("Checking if the revision is ready")
		checkRevisionReadiness(revisionName, true)
	})
})
