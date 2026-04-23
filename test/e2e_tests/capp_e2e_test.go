package e2e_tests

import (
	"context"

	"github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/util/retry"
	"knative.dev/serving/pkg/apis/autoscaling"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
)

var _ = Describe("Validate capp creation", func() {
	It("Should validate capp spec", func() {
		baseCapp := mocks.CreateBaseCapp()
		By("Creating Capp with no scale metric")
		desiredCapp := utilst.CreateCapp(k8sClient, baseCapp)
		Expect(desiredCapp.Spec.ScaleMetric).ShouldNot(BeNil())

		By("Creating Capp with unsupported scale metric")
		baseCapp.Spec.ScaleMetric = testconsts.UnsupportedScaleMetric
		Expect(k8sClient.Create(context.Background(), baseCapp)).ShouldNot(Succeed())
	})

	It("Should succeed all adapter functions", func() {
		baseCapp := mocks.CreateBaseCapp()
		desiredCapp := utilst.CreateCapp(k8sClient, baseCapp)

		By("Checks unique creation of Capp")
		assertionCapp := utilst.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace)
		Expect(assertionCapp.Name).ShouldNot(Equal(baseCapp.Name))

		By("Checks if Capp updated successfully")
		err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			assertionCapp := utilst.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace)
			assertionCapp.Spec.ScaleMetric = testconsts.RPSScaleMetric

			return utilst.UpdateResource(k8sClient, assertionCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() string {
			assertionCapp = utilst.GetCapp(k8sClient, assertionCapp.Name, assertionCapp.Namespace)
			return assertionCapp.Spec.ScaleMetric
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(testconsts.RPSScaleMetric), "Should fetch capp.")

		By("Checks if deleted successfully")
		utilst.DeleteCapp(k8sClient, assertionCapp)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, assertionCapp)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")
	})

	It("Validate state functionality", func() {
		By("Creating a capp instance")
		testCapp := mocks.CreateBaseCapp()
		createdCapp := utilst.CreateCapp(k8sClient, testCapp)

		By("Checking if the capp state is enabled")
		Eventually(func() string {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return capp.Status.StateStatus.State
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(testconsts.EnabledState))

		By("Checking if the ksvc was created successfully")
		ksvcObject := mocks.CreateKnativeServiceObject(createdCapp.Name)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, ksvcObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Checking if the revision is ready")
		revisionName := createdCapp.Name + testconsts.FirstRevisionSuffix
		checkRevisionReadiness(revisionName)

		By("Updating the capp status to be disabled")
		err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			assertionCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			assertionCapp.Spec.State = testconsts.DisabledState

			return utilst.UpdateResource(k8sClient, assertionCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		By("Checking if the capp state is disabled")
		Eventually(func() string {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return capp.Status.StateStatus.State
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(testconsts.DisabledState))

		By("Checking if the ksvc and the revision were deleted successfully")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, ksvcObject)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")
		Eventually(func() bool {
			revision := mocks.CreateRevisionObject(revisionName)
			return utilst.DoesResourceExist(k8sClient, revision)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Updating the capp status to be enabled")
		err = retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			assertionCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			assertionCapp.Spec.State = testconsts.EnabledState

			return utilst.UpdateResource(k8sClient, assertionCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		By("Checking if the ksvc was recreated successfully")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, ksvcObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Checking if the revision is ready")
		checkRevisionReadiness(revisionName)
	})
	It("Should validate minReplicas defaulting and validation", func() {
		baseCapp := mocks.CreateBaseCapp()
		baseCapp.Name = utilst.GenerateCappName()

		By("Creating Capp with no minReplicas (should default to 0)")
		createdCapp := utilst.CreateCapp(k8sClient, baseCapp)
		Eventually(func() int {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return capp.Spec.MinReplicas
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(0))

		By("Verifying KSVC annotation for default minReplicas")
		Eventually(func() bool {
			ksvc := &knativev1.Service{}
			utilst.GetResource(k8sClient, ksvc, createdCapp.Name, createdCapp.Namespace)
			minReplicas, found := ksvc.Spec.Template.Annotations[autoscaling.MinScaleAnnotationKey]
			if !found {
				return true
			}
			return minReplicas == "0"
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue())

		By("Updating Capp with valid minReplicas")
		err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			capp.Spec.MinReplicas = 3
			return utilst.UpdateResource(k8sClient, capp)
		})
		Expect(err).ToNot(HaveOccurred())

		By("Verifying Capp has minReplicas=3")
		Eventually(func() int {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return capp.Spec.MinReplicas
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(3))

		By("Verifying KSVC annotation for minReplicas=3")
		Eventually(func() string {
			ksvc := &knativev1.Service{}
			utilst.GetResource(k8sClient, ksvc, createdCapp.Name, createdCapp.Namespace)
			return ksvc.Spec.Template.Annotations[autoscaling.MinScaleAnnotationKey]
		}, testconsts.Timeout, testconsts.Interval).Should(Equal("3"))

		By("Updating Capp with invalid minReplicas (> GlobalMinScale)")

		err = retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			capp.Spec.MinReplicas = 20
			return utilst.UpdateResource(k8sClient, capp)
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("must be less than or equal to global min scale"))

		By("Cleaning up")
		utilst.DeleteCapp(k8sClient, createdCapp)
	})
})
