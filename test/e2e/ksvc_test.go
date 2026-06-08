package e2e

import (
	"fmt"

	"k8s.io/client-go/util/retry"
	"knative.dev/serving/pkg/apis/autoscaling"

	"github.com/dana-team/container-app-operator/test/e2e/consts"

	"github.com/dana-team/container-app-operator/test/e2e/mocks"
	"github.com/dana-team/container-app-operator/test/e2e/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// verifyLatestReadyRevision ensures that a new Knative Revision becomes ready
// and that LatestReadyRevisionName changes compared to the previous value.
func verifyLatestReadyRevision(name, namespace, latestReadyRevisionBeforeUpdate string) {
	Eventually(func() string {
		return utils.GetCapp(k8sClient, name, namespace).Status.KnativeObjectStatus.ConfigurationStatusFields.LatestReadyRevisionName
	}, consts.Timeout, consts.Interval).ShouldNot(Equal(latestReadyRevisionBeforeUpdate))

	actualNewRevision := utils.GetCapp(k8sClient, name, namespace).Status.KnativeObjectStatus.ConfigurationStatusFields.LatestReadyRevisionName
	checkRevisionReadiness(actualNewRevision)
}

// checkRevisionReadiness checks that the specified revision exists and becomes ready.
func checkRevisionReadiness(revisionName string) {
	By("Checking if the revision was created successfully")
	revisionObject := mocks.CreateRevisionObject(revisionName)
	Eventually(func() bool {
		return utils.DoesResourceExist(k8sClient, revisionObject)
	}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")
	By("Ensuring that the new revision is ready")
	Eventually(func() bool {
		return utils.GetRevision(k8sClient, revisionObject.Name, revisionObject.Namespace).IsReady()
	}, consts.Timeout, consts.Interval).Should(BeTrue())
}

var _ = Describe("Validate KSVC functionality", func() {
	It("Should create a ksvc with memory metric annotation when creating a capp with memory scale metric", func() {
		By("Creating a capp instance with memory scale metric")
		testCapp := mocks.CreateBaseCapp()
		testCapp.Spec.ScaleSpec.Metric = consts.MemoryScaleMetric
		createdCapp := utils.CreateCapp(k8sClient, testCapp)

		By("Checking if the ksvc was created with memory metric annotation successfully")
		Eventually(func() string {
			ksvc := utils.GetKSVC(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return ksvc.Spec.Template.Annotations[consts.KnativeMetricAnnotation]
		}, consts.Timeout, consts.Interval).Should(Equal(consts.MemoryScaleMetric))
	})

	It("Should create and delete a ksvc when creating and deleting a capp instance", func() {
		By("Creating a capp instance")
		testCapp := mocks.CreateBaseCapp()
		testCapp.Spec.ScaleSpec.Metric = consts.CPUScaleMetric
		createdCapp := utils.CreateCapp(k8sClient, testCapp)
		assertionCapp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)

		By("Checking if the ksvc was created successfully")
		ksvcObject := mocks.CreateKnativeServiceObject(assertionCapp.Name)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, ksvcObject)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")
		checkRevisionReadiness(assertionCapp.Name + consts.FirstRevisionSuffix)

		By("Deleting the capp instance")
		utils.DeleteCapp(k8sClient, assertionCapp)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, assertionCapp)
		}, consts.Timeout, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the ksvc exists")
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, ksvcObject)
		}, consts.Timeout, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the revision exists")
		revisionObject := mocks.CreateRevisionObject(assertionCapp.Name + consts.FirstRevisionSuffix)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, revisionObject)
		}, consts.Timeout, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")
	})

	It("Should create a new ready revision when updating capp spec", func() {
		By("Creating a capp instance")
		testCapp := mocks.CreateBaseCapp()
		createdCapp := utils.CreateCapp(k8sClient, testCapp)

		By("Updating capp container image")
		var latestReadyRevisionBeforeUpdate string
		err := retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
			assertionCapp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			latestReadyRevisionBeforeUpdate = assertionCapp.Status.KnativeObjectStatus.LatestReadyRevisionName

			assertionCapp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Image = consts.ImageExample
			return utils.UpdateResource(k8sClient, assertionCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		verifyLatestReadyRevision(createdCapp.Name, createdCapp.Namespace, latestReadyRevisionBeforeUpdate)
	})

	It("Should create a new revision in ready state when updating capp's secret environment variable", func() {
		By("Creating a secret")
		secretName := utils.GenerateSecretName()
		secretObject := mocks.CreateSecretObject(secretName)
		utils.CreateSecret(k8sClient, secretObject)

		By("Creating a capp instance with a secret environment variable")
		testCapp := mocks.CreateBaseCapp()
		testCapp.Spec.ConfigurationSpec.Template.Spec.PodSpec.Containers[0].Env = *mocks.CreateEnvVarObject(secretName)
		createdCapp := utils.CreateCapp(k8sClient, testCapp)
		assertionCapp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)

		checkRevisionReadiness(assertionCapp.Name + consts.FirstRevisionSuffix)

		By("Updating the secret")
		err := retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
			secretObject := utils.GetSecret(k8sClient, secretObject.Name, secretObject.Namespace)
			secretObject.Data = map[string][]byte{consts.NewSecretKey: []byte(consts.SecretValue)}

			return utils.UpdateResource(k8sClient, secretObject)
		})
		Expect(err).ToNot(HaveOccurred())

		By("Updating the capp secret environment variable")
		var latestReadyRevisionBeforeUpdate string
		err = retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
			assertionCapp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			latestReadyRevisionBeforeUpdate = assertionCapp.Status.KnativeObjectStatus.LatestReadyRevisionName

			assertionCapp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Env[0].ValueFrom.SecretKeyRef.Key = consts.NewSecretKey
			return utils.UpdateResource(k8sClient, assertionCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		verifyLatestReadyRevision(createdCapp.Name, createdCapp.Namespace, latestReadyRevisionBeforeUpdate)
	})

	It("Should create capp with autoscale annotation. The default annotation in the ksvc should be overridden", func() {
		By("Creating a capp instance")
		testCapp := mocks.CreateBaseCapp()
		annotations := map[string]string{
			autoscaling.TargetAnnotationKey: "666",
		}
		testCapp.Spec.ConfigurationSpec.Template.Annotations = annotations
		createdCapp := utils.CreateCapp(k8sClient, testCapp)
		assertionCapp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)

		By("Checking if the ksvc's defaults annotations were overridden")
		Eventually(func() string {
			ksvc := utils.GetKSVC(k8sClient, assertionCapp.Name, assertionCapp.Namespace)
			return ksvc.Spec.ConfigurationSpec.Template.Annotations[autoscaling.TargetAnnotationKey]
		}, consts.Timeout, consts.Interval).Should(Equal("666"))
	})

	It("Should check the default ksvc annotation is equal to the cappConfig's concurrency value", func() {
		By("Creating a capp instance")
		testCapp := mocks.CreateBaseCapp()
		createdCapp := utils.CreateCapp(k8sClient, testCapp)
		assertionCapp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)

		cappConfig := utils.GetCappConfig(k8sClient, consts.CappConfigName, consts.ControllerNS)

		By("Checking if the ksvc's annotation is equal to the cappConfig's autoScale")
		Eventually(func() bool {
			ksvc := utils.GetKSVC(k8sClient, assertionCapp.Name, assertionCapp.Namespace)
			return ksvc.Spec.ConfigurationSpec.Template.ObjectMeta.Annotations[autoscaling.TargetAnnotationKey] == fmt.Sprintf("%d", cappConfig.Spec.AutoscaleConfig.Concurrency) &&
				ksvc.Spec.ConfigurationSpec.Template.ObjectMeta.Annotations[autoscaling.ActivationScaleKey] == fmt.Sprintf("%d", cappConfig.Spec.AutoscaleConfig.ActivationScale)
		}, consts.Timeout, consts.Interval).Should(BeTrue())
	})
})
