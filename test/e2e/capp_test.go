package e2e

import (
	"github.com/dana-team/container-app-operator/test/e2e/consts"
	"github.com/dana-team/container-app-operator/test/e2e/mocks"
	"github.com/dana-team/container-app-operator/test/e2e/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/util/retry"
	"knative.dev/serving/pkg/apis/autoscaling"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
)

var _ = Describe("Validate capp creation", func() {
	It("Should default scale metric when unset", func() {
		baseCapp := mocks.CreateBaseCapp()
		By("Creating Capp with no scale metric")
		desiredCapp := utils.CreateCapp(k8sClient, baseCapp)
		Expect(desiredCapp.Spec.ScaleSpec.Metric).ShouldNot(BeNil())
	})

	It("Should succeed all adapter functions", func() {
		baseCapp := mocks.CreateBaseCapp()
		desiredCapp := utils.CreateCapp(k8sClient, baseCapp)

		By("Checks unique creation of Capp")
		assertionCapp := utils.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace)
		Expect(assertionCapp.Name).ShouldNot(Equal(baseCapp.Name))

		By("Checks if Capp updated successfully")
		err := retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
			assertionCapp := utils.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace)
			assertionCapp.Spec.ScaleSpec.Metric = consts.RPSScaleMetric

			return utils.UpdateResource(k8sClient, assertionCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() string {
			assertionCapp = utils.GetCapp(k8sClient, assertionCapp.Name, assertionCapp.Namespace)
			return assertionCapp.Spec.ScaleSpec.Metric
		}, consts.Timeout, consts.Interval).Should(Equal(consts.RPSScaleMetric), "Should fetch capp.")

		By("Checks if deleted successfully")
		utils.DeleteCapp(k8sClient, assertionCapp)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, assertionCapp)
		}, consts.Timeout, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")
	})

	It("Validate state functionality", func() {
		By("Creating a capp instance")
		testCapp := mocks.CreateBaseCapp()
		createdCapp := utils.CreateCapp(k8sClient, testCapp)

		By("Checking if the capp state is enabled")
		Eventually(func() string {
			capp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return capp.Status.StateStatus.State
		}, consts.Timeout, consts.Interval).Should(Equal(consts.EnabledState))

		By("Checking if the ksvc was created successfully")
		ksvcObject := mocks.CreateKnativeServiceObject(createdCapp.Name)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, ksvcObject)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Checking if the revision is ready")
		revisionName := createdCapp.Name + consts.FirstRevisionSuffix
		checkRevisionReadiness(revisionName)

		By("Updating the capp status to be disabled")
		err := retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
			assertionCapp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			assertionCapp.Spec.State = consts.DisabledState

			return utils.UpdateResource(k8sClient, assertionCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		By("Checking if the capp state is disabled")
		Eventually(func() string {
			capp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return capp.Status.StateStatus.State
		}, consts.Timeout, consts.Interval).Should(Equal(consts.DisabledState))

		By("Checking if the ksvc and the revision were deleted successfully")
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, ksvcObject)
		}, consts.Timeout, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")
		Eventually(func() bool {
			revision := mocks.CreateRevisionObject(revisionName)
			return utils.DoesResourceExist(k8sClient, revision)
		}, consts.Timeout, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Updating the capp status to be enabled")
		err = retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
			assertionCapp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			assertionCapp.Spec.State = consts.EnabledState

			return utils.UpdateResource(k8sClient, assertionCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		By("Checking if the ksvc was recreated successfully")
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, ksvcObject)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Checking if the revision is ready")
		checkRevisionReadiness(revisionName)
	})
	It("Should validate minReplicas defaulting and validation", func() {
		baseCapp := mocks.CreateBaseCapp()
		baseCapp.Name = utils.GenerateCappName()

		By("Creating Capp with no minReplicas (should default to 0)")
		createdCapp := utils.CreateCapp(k8sClient, baseCapp)
		Eventually(func() int {
			capp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return capp.Spec.ScaleSpec.MinReplicas
		}, consts.Timeout, consts.Interval).Should(Equal(0))

		By("Verifying KSVC annotation for default minReplicas")
		Eventually(func() bool {
			ksvc := &knativev1.Service{}
			utils.GetResource(k8sClient, ksvc, createdCapp.Name, createdCapp.Namespace)
			minReplicas, found := ksvc.Spec.Template.Annotations[autoscaling.MinScaleAnnotationKey]
			if !found {
				return true
			}
			return minReplicas == "0"
		}, consts.Timeout, consts.Interval).Should(BeTrue())

		By("Updating Capp with valid minReplicas")
		err := retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
			capp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			capp.Spec.ScaleSpec.MinReplicas = 3
			return utils.UpdateResource(k8sClient, capp)
		})
		Expect(err).ToNot(HaveOccurred())

		By("Verifying Capp has minReplicas=3")
		Eventually(func() int {
			capp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return capp.Spec.ScaleSpec.MinReplicas
		}, consts.Timeout, consts.Interval).Should(Equal(3))

		By("Verifying KSVC annotation for minReplicas=3")
		Eventually(func() string {
			ksvc := &knativev1.Service{}
			utils.GetResource(k8sClient, ksvc, createdCapp.Name, createdCapp.Namespace)
			return ksvc.Spec.Template.Annotations[autoscaling.MinScaleAnnotationKey]
		}, consts.Timeout, consts.Interval).Should(Equal("3"))

		By("Updating Capp with invalid minReplicas (> GlobalMinScale)")

		err = retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
			capp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			capp.Spec.ScaleSpec.MinReplicas = 20
			return utils.UpdateResource(k8sClient, capp)
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("must be less than or equal to global min scale"))

		By("Cleaning up")
		utils.DeleteCapp(k8sClient, createdCapp)
	})
})
