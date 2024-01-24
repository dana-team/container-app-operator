package k8s_tests

import (
	"fmt"
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	mock "github.com/dana-team/container-app-operator/test/k8s_tests/mocks"
	utilst "github.com/dana-team/container-app-operator/test/k8s_tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	knativeMetricAnnotation = "autoscaling.knative.dev/metric"
	imageExample            = "danateam/autoscale-go"
	nonExistingImageExample = "example-python-app:v1"
	exampleAppName          = "new-app-name"
	newSecretKey            = "username"
	creatorAnnotation       = "serving.knative.dev/creator"
	lastModifierAnnotation  = "serving.knative.dev/lastModifier"
	cappServiceAccount      = "system:serviceaccount:capp-operator-system:capp-operator-controller-manager"
	exampleDanaAnnotation   = "rcs.dana.io/app-name"
	testContainerName       = "capp-test-container"
	firstRevisionSuffix     = "-00001"
)

// updateCapp updates the given Capp object and ensures the readiness of the latest revision
// if shouldRevisionBeReady is true. It also checks and asserts the state of the LatestReadyRevision.
func updateCapp(capp *rcsv1alpha1.Capp, shouldRevisionBeReady bool) {
	latestReadyRevisionBeforeUpdate := capp.Status.KnativeObjectStatus.ConfigurationStatusFields.LatestReadyRevisionName
	nextRevisionName := utilst.GetNextRevisionName(latestReadyRevisionBeforeUpdate)
	utilst.UpdateCapp(k8sClient, capp)
	if shouldRevisionBeReady {
		checkRevisionReadiness(nextRevisionName, true)
		By("Ensuring that the capp LatestReadyRevision is updated")
		Eventually(func() string {
			return utilst.GetCapp(k8sClient, capp.Name, capp.Namespace).Status.KnativeObjectStatus.ConfigurationStatusFields.LatestReadyRevisionName
		}, TimeoutCapp, CappCreationInterval).Should(Equal(nextRevisionName))
	} else {
		By("Ensuring that the capp LatestReadyRevision is not updated")
		checkRevisionReadiness(nextRevisionName, false)
		Eventually(func() string {
			return utilst.GetCapp(k8sClient, capp.Name, capp.Namespace).Status.KnativeObjectStatus.ConfigurationStatusFields.LatestReadyRevisionName
		}, TimeoutCapp, CappCreationInterval).Should(Equal(latestReadyRevisionBeforeUpdate))
	}
}

// checkRevisionReadiness checks the readiness of the specified revision and asserts its state based on shouldBeReady flag.
func checkRevisionReadiness(revisionName string, shouldBeReady bool) {
	By("Checking if the revision was created successfully")
	revisionObject := mock.CreateRevisionObject(revisionName)
	Eventually(func() bool {
		return utilst.DoesResourceExist(k8sClient, revisionObject)
	}, TimeoutCapp, CappCreationInterval).Should(BeTrue(), "Should find a resource.")
	if shouldBeReady {
		By("Ensuring that the new revision is ready")
		Eventually(func() bool {
			return utilst.GetRevision(k8sClient, revisionObject.Name, revisionObject.Namespace).IsReady()
		}, TimeoutCapp, CappCreationInterval).Should(BeTrue())
	} else {
		By("Ensuring that the new revision is not ready")
		Eventually(func() bool {
			return utilst.GetRevision(k8sClient, revisionObject.Name, revisionObject.Namespace).IsReady()
		}, TimeoutCapp, CappCreationInterval).ShouldNot(BeTrue())
	}
}

// createAndGetCapp creates a Capp using the provided testCapp object,
// retrieves the created Capp and returns it for further use.
func createAndGetCapp(testCapp *rcsv1alpha1.Capp) *rcsv1alpha1.Capp {
	createdCapp := utilst.CreateCapp(k8sClient, testCapp)
	assertionCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
	return assertionCapp
}

// testMetricAnnotation tests capp instance creation with a specified metric annotation.
func testMetricAnnotation(metricType string) {
	By("Creating a capp instance")
	testCapp := mock.CreateBaseCapp()
	testCapp.Spec.ScaleMetric = metricType
	createdCapp := utilst.CreateCapp(k8sClient, testCapp)

	By(fmt.Sprintf("Checking if the ksvc was created with %s metric annotation successfully", metricType))
	Eventually(func() string {
		ksvc := utilst.GetKsvc(k8sClient, createdCapp.Name, createdCapp.Namespace)
		return ksvc.Spec.Template.Annotations[knativeMetricAnnotation]
	}, TimeoutCapp, CappCreationInterval).Should(Equal(metricType))
}

var _ = Describe("Validate knative functionality", func() {

	It("Should create a ksvc with cpu metric annotation when creating a capp with cpu scale metric", func() {
		testMetricAnnotation("cpu")
	})

	It("Should create a ksvc with memory metric annotation when creating a capp with memory scale metric", func() {
		testMetricAnnotation("memory")
	})

	It("Should create a ksvc with rps metric annotation when creating a capp with rps scale metric", func() {
		testMetricAnnotation("rps")
	})

	It("Should create and delete a ksvc when creating and deleting a capp instance", func() {
		By("Creating a capp instance")
		testCapp := mock.CreateBaseCapp()
		testCapp.Spec.ScaleMetric = "cpu"
		assertionCapp := createAndGetCapp(testCapp)

		By("Checking if the ksvc was created successfully")
		ksvcObject := mock.CreateKnativeServiceObject(assertionCapp.Name)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, ksvcObject)
		}, TimeoutCapp, CappCreationInterval).Should(BeTrue(), "Should find a resource.")

		By("Checking if the ksvc has the default annotations")
		Eventually(func() string {
			return utilst.GetKsvc(k8sClient, ksvcObject.Name, ksvcObject.Namespace).Annotations[creatorAnnotation]
		}, TimeoutCapp, CappCreationInterval).Should(Equal(cappServiceAccount))
		Eventually(func() string {
			return utilst.GetKsvc(k8sClient, ksvcObject.Name, ksvcObject.Namespace).Annotations[lastModifierAnnotation]
		}, TimeoutCapp, CappCreationInterval).Should(Equal(cappServiceAccount))
		checkRevisionReadiness(assertionCapp.Name+firstRevisionSuffix, true)

		By("Deleting the capp instance")
		utilst.DeleteCapp(k8sClient, assertionCapp)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, assertionCapp)
		}, TimeoutCapp, CappCreationInterval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the ksvc exists")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, ksvcObject)
		}, TimeoutCapp, CappCreationInterval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the revision exists")
		revisionObject := mock.CreateRevisionObject(assertionCapp.Name + firstRevisionSuffix)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, revisionObject)
		}, TimeoutCapp, CappCreationInterval).ShouldNot(BeTrue(), "Should not find a resource.")
	})

	It("Should update ksvc metric annotation and create a new revision when updating the capp scale metric", func() {
		By("Creating a capp instance")
		testCapp := mock.CreateBaseCapp()
		testCapp.Spec.ScaleMetric = "cpu"
		assertionCapp := createAndGetCapp(testCapp)

		By("Updating the Capp scale metric")
		assertionCapp.Spec.ScaleMetric = "memory"
		updateCapp(assertionCapp, true)

		By("Checking if the ksvc was updated successfully")
		Eventually(func() string {
			ksvc := utilst.GetKsvc(k8sClient, assertionCapp.Name, assertionCapp.Namespace)
			return ksvc.Spec.Template.Annotations[knativeMetricAnnotation]
		}, TimeoutCapp, CappCreationInterval).Should(Equal("memory"))
	})

	It("Should update ksvc container name and create a new revision when updating a capp container name", func() {
		By("Creating a capp instance")
		testCapp := mock.CreateBaseCapp()
		assertionCapp := createAndGetCapp(testCapp)

		By("Updating the a capp container name")
		assertionCapp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Name = testContainerName
		updateCapp(assertionCapp, true)

		By("Checking if the ksvc was updated successfully")
		Eventually(func() string {
			ksvc := utilst.GetKsvc(k8sClient, assertionCapp.Name, assertionCapp.Namespace)
			return ksvc.Spec.ConfigurationSpec.Template.Spec.Containers[0].Name
		}, TimeoutCapp, CappCreationInterval).Should(Equal(testContainerName))
	})

	It("Should update ksvc container image and create a new revision when updating a capp container image", func() {
		By("Creating a capp instance")
		testCapp := mock.CreateBaseCapp()
		assertionCapp := createAndGetCapp(testCapp)

		By("Updating capp's container image")
		assertionCapp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Image = imageExample
		updateCapp(assertionCapp, true)

		By("Checking if the ksvc's container image was updated successfully")
		Eventually(func() string {
			ksvc := utilst.GetKsvc(k8sClient, assertionCapp.Name, assertionCapp.Namespace)
			return ksvc.Spec.ConfigurationSpec.Template.Spec.Containers[0].Image
		}, TimeoutCapp, CappCreationInterval).Should(Equal(imageExample))
	})

	It("Should update ksvc dana annotation when updating capp's dana annotation", func() {
		By("Creating a capp instance")
		testCapp := mock.CreateBaseCapp()
		assertionCapp := createAndGetCapp(testCapp)

		By("Updating capp's dana annotation")
		cappAnnotations := map[string]string{}
		cappAnnotations[exampleDanaAnnotation] = exampleAppName
		assertionCapp.Annotations = cappAnnotations
		updateCapp(assertionCapp, true)

		By("Checking if the ksvc's dana annotation was updated successfully")
		Eventually(func() string {
			ksvc := utilst.GetKsvc(k8sClient, assertionCapp.Name, assertionCapp.Namespace)
			return ksvc.Spec.Template.Annotations[exampleDanaAnnotation]
		}, TimeoutCapp, CappCreationInterval).Should(Equal(exampleAppName))
	})

	It("Should update ksvc environment variable and create a new revision when updating a capp container environment variable", func() {
		By("Creating a capp instance")
		testCapp := mock.CreateBaseCapp()
		assertionCapp := createAndGetCapp(testCapp)

		By("Updating capp's container environment variable")
		assertionCapp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Env[0].Value = exampleAppName
		updateCapp(assertionCapp, true)

		By("Checking if the ksvc's container environment variable was updated successfully")
		Eventually(func() string {
			ksvc := utilst.GetKsvc(k8sClient, assertionCapp.Name, assertionCapp.Namespace)
			return ksvc.Spec.ConfigurationSpec.Template.Spec.Containers[0].Env[0].Value
		}, TimeoutCapp, CappCreationInterval).Should(Equal(exampleAppName))
	})

	It("Should create a new revision in ready state when updating capp's secret environment variable", func() {
		By("Creating a secret")
		secretName := utilst.GenerateSecretName()
		secretObject := mock.CreateSecretObject(secretName)
		utilst.CreateSecret(k8sClient, secretObject)

		By("Creating a capp instance with a secret environment variable")
		testCapp := mock.CreateBaseCapp()
		testCapp.Spec.ConfigurationSpec.Template.Spec.PodSpec.Containers[0].Env = *mock.CreateEnvVarObject(secretName)
		assertionCapp := createAndGetCapp(testCapp)
		checkRevisionReadiness(assertionCapp.Name+firstRevisionSuffix, true)

		By("Updating the secret")
		secretObject.Data = map[string][]byte{newSecretKey: []byte(mock.SecretValue)}
		utilst.UpdateSecret(k8sClient, secretObject)

		By("Updating the capp secret environment variable")
		assertionCapp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Env[0].ValueFrom.SecretKeyRef.Key = newSecretKey
		updateCapp(assertionCapp, true)
	})

	It("Should create not ready revision when attempting to update to non existing image", func() {
		By("Creating a capp instance")
		testCapp := mock.CreateBaseCapp()
		assertionCapp := createAndGetCapp(testCapp)

		By("Updating capp's container image")
		assertionCapp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Image = nonExistingImageExample
		updateCapp(assertionCapp, false)

		By("Checking if the ksvc's container image was updated successfully")
		Eventually(func() string {
			ksvc := utilst.GetKsvc(k8sClient, assertionCapp.Name, assertionCapp.Namespace)
			return ksvc.Spec.ConfigurationSpec.Template.Spec.Containers[0].Image
		}, TimeoutCapp, CappCreationInterval).Should(Equal(nonExistingImageExample))
	})

	It("Should create not ready revision when attempting to update to non existing secret", func() {
		By("Creating a secret")
		secretName := utilst.GenerateSecretName()
		secretObject := mock.CreateSecretObject(secretName)
		utilst.CreateSecret(k8sClient, secretObject)

		By("Creating a capp instance with a secret environment variable")
		testCapp := mock.CreateBaseCapp()
		testCapp.Spec.ConfigurationSpec.Template.Spec.PodSpec.Containers[0].Env = *mock.CreateEnvVarObject(secretName)
		assertionCapp := createAndGetCapp(testCapp)
		checkRevisionReadiness(assertionCapp.Name+firstRevisionSuffix, true)

		By("Updating the capp secret environment variable")
		nonExistingSecretName := utilst.GenerateSecretName()
		assertionCapp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Env = *mock.CreateEnvVarObject(nonExistingSecretName)
		updateCapp(assertionCapp, false)
	})
})
