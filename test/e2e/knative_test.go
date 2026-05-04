package e2e

import (
	"fmt"

	"k8s.io/client-go/util/retry"
	"knative.dev/serving/pkg/apis/autoscaling"

	"github.com/dana-team/container-app-operator/test/e2e/consts"

	"github.com/dana-team/container-app-operator/test/e2e/mocks"
	utilst "github.com/dana-team/container-app-operator/test/e2e/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// verifyLatestReadyRevision ensures that a new Knative Revision becomes ready
// and that LatestReadyRevisionName changes compared to the previous value.
func verifyLatestReadyRevision(name, namespace, latestReadyRevisionBeforeUpdate string) {
	Eventually(func() string {
		return utilst.GetCapp(k8sClient, name, namespace).Status.KnativeObjectStatus.ConfigurationStatusFields.LatestReadyRevisionName
	}, consts.Timeout, consts.Interval).ShouldNot(Equal(latestReadyRevisionBeforeUpdate))

	actualNewRevision := utilst.GetCapp(k8sClient, name, namespace).Status.KnativeObjectStatus.ConfigurationStatusFields.LatestReadyRevisionName
	checkRevisionReadiness(actualNewRevision)
}

// checkRevisionReadiness checks that the specified revision exists and becomes ready.
func checkRevisionReadiness(revisionName string) {
	By("Checking if the revision was created successfully")
	revisionObject := mocks.CreateRevisionObject(revisionName)
	Eventually(func() bool {
		return utilst.DoesResourceExist(k8sClient, revisionObject)
	}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")
	By("Ensuring that the new revision is ready")
	Eventually(func() bool {
		return utilst.GetRevision(k8sClient, revisionObject.Name, revisionObject.Namespace).IsReady()
	}, consts.Timeout, consts.Interval).Should(BeTrue())
}

// testMetricAnnotation tests capp instance creation with a specified metric annotation.
func testMetricAnnotation(metricType string) {
	By("Creating a capp instance")
	testCapp := mocks.CreateBaseCapp()
	testCapp.Spec.ScaleMetric = metricType
	createdCapp := utilst.CreateCapp(k8sClient, testCapp)

	By(fmt.Sprintf("Checking if the ksvc was created with %s metric annotation successfully", metricType))
	Eventually(func() string {
		ksvc := utilst.GetKSVC(k8sClient, createdCapp.Name, createdCapp.Namespace)
		return ksvc.Spec.Template.Annotations[consts.KnativeMetricAnnotation]
	}, consts.Timeout, consts.Interval).Should(Equal(metricType))
}

var _ = Describe("Validate knative functionality", func() {
	It("Should create a ksvc with cpu metric annotation when creating a capp with cpu scale metric", func() {
		testMetricAnnotation(consts.CPUScaleMetric)
	})

	It("Should create a ksvc with memory metric annotation when creating a capp with memory scale metric", func() {
		testMetricAnnotation("memory")
	})

	It("Should create a ksvc with rps metric annotation when creating a capp with rps scale metric", func() {
		testMetricAnnotation("rps")
	})

	It("Should create and delete a ksvc when creating and deleting a capp instance", func() {
		By("Creating a capp instance")
		testCapp := mocks.CreateBaseCapp()
		testCapp.Spec.ScaleMetric = consts.CPUScaleMetric
		createdCapp := utilst.CreateCapp(k8sClient, testCapp)
		assertionCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)

		By("Checking if the ksvc was created successfully")
		ksvcObject := mocks.CreateKnativeServiceObject(assertionCapp.Name)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, ksvcObject)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")
		checkRevisionReadiness(assertionCapp.Name + consts.FirstRevisionSuffix)

		By("Checking the ksvc has the needed labels")
		ksvcObject = utilst.GetKSVC(k8sClient, assertionCapp.Name, consts.NSName)
		Expect(ksvcObject.Labels[consts.CappResourceKey]).Should(Equal(assertionCapp.Name))
		Expect(ksvcObject.Labels[consts.ManagedByLabelKey]).Should(Equal(consts.CappKey))

		By("Deleting the capp instance")
		utilst.DeleteCapp(k8sClient, assertionCapp)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, assertionCapp)
		}, consts.Timeout, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the ksvc exists")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, ksvcObject)
		}, consts.Timeout, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the revision exists")
		revisionObject := mocks.CreateRevisionObject(assertionCapp.Name + consts.FirstRevisionSuffix)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, revisionObject)
		}, consts.Timeout, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")
	})

	It("Should update ksvc metric annotation and create a new revision when updating the capp scale metric", func() {
		By("Creating a capp instance")
		testCapp := mocks.CreateBaseCapp()
		testCapp.Spec.ScaleMetric = consts.CPUScaleMetric
		createdCapp := utilst.CreateCapp(k8sClient, testCapp)

		By("Updating the Capp scale metric")
		var latestReadyRevisionBeforeUpdate string
		err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			assertionCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			latestReadyRevisionBeforeUpdate = assertionCapp.Status.KnativeObjectStatus.LatestReadyRevisionName

			assertionCapp.Spec.ScaleMetric = "memory"
			return utilst.UpdateResource(k8sClient, assertionCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		verifyLatestReadyRevision(createdCapp.Name, createdCapp.Namespace, latestReadyRevisionBeforeUpdate)

		By("Checking if the ksvc was updated successfully")
		Eventually(func() string {
			ksvc := utilst.GetKSVC(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return ksvc.Spec.Template.Annotations[consts.KnativeMetricAnnotation]
		}, consts.Timeout, consts.Interval).Should(Equal("memory"))
	})

	It("Should update ksvc container name and create a new revision when updating a capp container name", func() {
		By("Creating a capp instance")
		testCapp := mocks.CreateBaseCapp()
		createdCapp := utilst.CreateCapp(k8sClient, testCapp)

		By("Updating the a capp container name")
		var latestReadyRevisionBeforeUpdate string
		err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			assertionCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			latestReadyRevisionBeforeUpdate = assertionCapp.Status.KnativeObjectStatus.LatestReadyRevisionName

			assertionCapp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Name = consts.TestContainerName
			return utilst.UpdateResource(k8sClient, assertionCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		verifyLatestReadyRevision(createdCapp.Name, createdCapp.Namespace, latestReadyRevisionBeforeUpdate)

		By("Checking if the ksvc was updated successfully")
		Eventually(func() string {
			ksvc := utilst.GetKSVC(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return ksvc.Spec.ConfigurationSpec.Template.Spec.Containers[0].Name
		}, consts.Timeout, consts.Interval).Should(Equal(consts.TestContainerName))
	})

	It("Should update ksvc container image and create a new revision when updating a capp container image", func() {
		By("Creating a capp instance")
		testCapp := mocks.CreateBaseCapp()
		createdCapp := utilst.CreateCapp(k8sClient, testCapp)

		By("Updating capp's container image")
		var latestReadyRevisionBeforeUpdate string
		err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			assertionCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			latestReadyRevisionBeforeUpdate = assertionCapp.Status.KnativeObjectStatus.LatestReadyRevisionName

			assertionCapp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Image = consts.ImageExample
			return utilst.UpdateResource(k8sClient, assertionCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		verifyLatestReadyRevision(createdCapp.Name, createdCapp.Namespace, latestReadyRevisionBeforeUpdate)

		By("Checking if the ksvc's container image was updated successfully")
		Eventually(func() string {
			ksvc := utilst.GetKSVC(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return ksvc.Spec.ConfigurationSpec.Template.Spec.Containers[0].Image
		}, consts.Timeout, consts.Interval).Should(Equal(consts.ImageExample))
	})

	It("Should update ksvc when updating a propagating Capp metadata annotation", func() {
		const propagationAnnKey = "example.com/e2e-propagation"

		By("Creating a capp instance")
		testCapp := mocks.CreateBaseCapp()
		createdCapp := utilst.CreateCapp(k8sClient, testCapp)

		By("Updating capp metadata annotation that is copied to the Knative Service")
		cappAnnotations := map[string]string{}
		cappAnnotations[propagationAnnKey] = consts.ExampleAppName

		var latestReadyRevisionBeforeUpdate string
		err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			assertionCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			latestReadyRevisionBeforeUpdate = assertionCapp.Status.KnativeObjectStatus.LatestReadyRevisionName

			assertionCapp.Annotations = cappAnnotations
			return utilst.UpdateResource(k8sClient, assertionCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		verifyLatestReadyRevision(createdCapp.Name, createdCapp.Namespace, latestReadyRevisionBeforeUpdate)

		By("Checking if the ksvc template annotation was updated successfully")
		Eventually(func() string {
			ksvc := utilst.GetKSVC(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return ksvc.Spec.Template.Annotations[propagationAnnKey]
		}, consts.Timeout, consts.Interval).Should(Equal(consts.ExampleAppName))
	})

	It("Should update ksvc environment variable and create a new revision when updating a capp container environment variable", func() {
		By("Creating a capp instance")
		testCapp := mocks.CreateBaseCapp()
		createdCapp := utilst.CreateCapp(k8sClient, testCapp)

		By("Updating capp's container environment variable")
		var latestReadyRevisionBeforeUpdate string
		err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			assertionCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			latestReadyRevisionBeforeUpdate = assertionCapp.Status.KnativeObjectStatus.LatestReadyRevisionName

			assertionCapp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Env[0].Value = consts.ExampleAppName
			return utilst.UpdateResource(k8sClient, assertionCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		verifyLatestReadyRevision(createdCapp.Name, createdCapp.Namespace, latestReadyRevisionBeforeUpdate)

		By("Checking if the ksvc's container environment variable was updated successfully")
		Eventually(func() string {
			ksvc := utilst.GetKSVC(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return ksvc.Spec.ConfigurationSpec.Template.Spec.Containers[0].Env[0].Value
		}, consts.Timeout, consts.Interval).Should(Equal(consts.ExampleAppName))
	})

	It("Should create a new revision in ready state when updating capp's secret environment variable", func() {
		By("Creating a secret")
		secretName := utilst.GenerateSecretName()
		secretObject := mocks.CreateSecretObject(secretName)
		utilst.CreateSecret(k8sClient, secretObject)

		By("Creating a capp instance with a secret environment variable")
		testCapp := mocks.CreateBaseCapp()
		testCapp.Spec.ConfigurationSpec.Template.Spec.PodSpec.Containers[0].Env = *mocks.CreateEnvVarObject(secretName)
		createdCapp := utilst.CreateCapp(k8sClient, testCapp)
		assertionCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)

		checkRevisionReadiness(assertionCapp.Name + consts.FirstRevisionSuffix)

		By("Updating the secret")
		err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			secretObject := utilst.GetSecret(k8sClient, secretObject.Name, secretObject.Namespace)
			secretObject.Data = map[string][]byte{consts.NewSecretKey: []byte(consts.SecretValue)}

			return utilst.UpdateResource(k8sClient, secretObject)
		})
		Expect(err).ToNot(HaveOccurred())

		By("Updating the capp secret environment variable")
		var latestReadyRevisionBeforeUpdate string
		err = retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			assertionCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			latestReadyRevisionBeforeUpdate = assertionCapp.Status.KnativeObjectStatus.LatestReadyRevisionName

			assertionCapp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Env[0].ValueFrom.SecretKeyRef.Key = consts.NewSecretKey
			return utilst.UpdateResource(k8sClient, assertionCapp)
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
		createdCapp := utilst.CreateCapp(k8sClient, testCapp)
		assertionCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)

		By("Checking if the ksvc's defaults annotations were overridden")
		Eventually(func() string {
			ksvc := utilst.GetKSVC(k8sClient, assertionCapp.Name, assertionCapp.Namespace)
			return ksvc.Spec.ConfigurationSpec.Template.Annotations[autoscaling.TargetAnnotationKey]
		}, consts.Timeout, consts.Interval).Should(Equal("666"))
	})

	It("Should propagate Capp labels to the underlying KSVC", func() {
		By("Creating a capp instance")
		testCapp := mocks.CreateBaseCapp()
		labels := map[string]string{
			consts.TestLabelKey:    "test",
			consts.CappResourceKey: "test",
		}
		testCapp.Labels = labels
		createdCapp := utilst.CreateCapp(k8sClient, testCapp)
		assertionCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)

		By("Checking if user-defined labels were propagated to the ksvc")
		Eventually(func() string {
			ksvc := utilst.GetKSVC(k8sClient, assertionCapp.Name, assertionCapp.Namespace)
			return ksvc.Spec.ConfigurationSpec.Template.Labels[consts.TestLabelKey]
		}, consts.Timeout, consts.Interval).Should(Equal("test"))

		By("Checking if labels set by the controller cannot be overridden by users")
		Consistently(func() string {
			ksvc := utilst.GetKSVC(k8sClient, assertionCapp.Name, assertionCapp.Namespace)
			return ksvc.Spec.ConfigurationSpec.Template.Labels[consts.CappResourceKey]
		}, consts.DefaultConsistently, consts.Interval).ShouldNot(Equal("test"))

		Eventually(func() string {
			ksvc := utilst.GetKSVC(k8sClient, assertionCapp.Name, assertionCapp.Namespace)
			return ksvc.Spec.ConfigurationSpec.Template.Labels[consts.CappResourceKey]
		}, consts.Timeout, consts.Interval).Should(Equal(assertionCapp.Name))
	})

	It("Should check the default ksvc annotation is equal to the cappConfig's concurrency value", func() {
		By("Creating a capp instance")
		testCapp := mocks.CreateBaseCapp()
		createdCapp := utilst.CreateCapp(k8sClient, testCapp)
		assertionCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)

		cappConfig := utilst.GetCappConfig(k8sClient, consts.CappConfigName, consts.ControllerNS)

		By("Checking if the ksvc's annotation is equal to the cappConfig's autoScale")
		Eventually(func() bool {
			ksvc := utilst.GetKSVC(k8sClient, assertionCapp.Name, assertionCapp.Namespace)
			return ksvc.Spec.ConfigurationSpec.Template.ObjectMeta.Annotations[autoscaling.TargetAnnotationKey] == fmt.Sprintf("%d", cappConfig.Spec.AutoscaleConfig.Concurrency) &&
				ksvc.Spec.ConfigurationSpec.Template.ObjectMeta.Annotations[autoscaling.ActivationScaleKey] == fmt.Sprintf("%d", cappConfig.Spec.AutoscaleConfig.ActivationScale)
		}, consts.Timeout, consts.Interval).Should(BeTrue())
	})
})
