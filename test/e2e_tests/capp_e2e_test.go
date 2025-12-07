package e2e_tests

import (
	"context"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"

	"github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
	It("Should create a Capp with a Keda source", func() {
		By("Creating a Capp instance with a Keda source")

		testCapp := mocks.CreateBaseCapp()

		kedaSource := cappv1alpha1.KedaSource{
			Name:       "kafka-source",
			ScalarType: "kafka",
			ScalarMetadata: map[string]string{
				"topic": "demo-topic",
			},
			MinReplicas: pointer.Int32(0),
			MaxReplicas: pointer.Int32(2),
			TriggerAuth: &cappv1alpha1.TriggerAuth{
				Type: "triggerAuthentication",
				Name: "kafka-auth",
				SecretTargets: []cappv1alpha1.AuthSecretTarget{
					{
						Parameter: "username",
						SecretRef: corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "kafka-secret",
							},
							Key: "username",
						},
					},
				},
			},
		}

		testCapp.Spec.Sources = append(testCapp.Spec.Sources, kedaSource)

		createdCapp := utilst.CreateCapp(k8sClient, testCapp)

		By("Checking if the Capp instance has a Keda source")
		Expect(createdCapp.Spec.Sources).Should(HaveLen(1))
		Expect(createdCapp.Spec.Sources[0].Name).Should(Equal("kafka-source"))
		Expect(createdCapp.Spec.Sources[0].ScalarType).Should(Equal("kafka"))
		Expect(createdCapp.Spec.Sources[0].ScalarMetadata).Should(HaveKeyWithValue("topic", "demo-topic"))
		Expect(createdCapp.Spec.Sources[0].MinReplicas).Should(Equal(pointer.Int32(0)))
		Expect(createdCapp.Spec.Sources[0].MaxReplicas).Should(Equal(pointer.Int32(2)))
		Expect(createdCapp.Spec.Sources[0].TriggerAuth).NotTo(BeNil())
		Expect(createdCapp.Spec.Sources[0].TriggerAuth.Name).Should(Equal("kafka-auth"))
		Expect(createdCapp.Spec.Sources[0].TriggerAuth.SecretTargets).Should(HaveLen(1))
		Expect(createdCapp.Spec.Sources[0].TriggerAuth.SecretTargets[0].Parameter).Should(Equal("username"))
		Expect(createdCapp.Spec.Sources[0].TriggerAuth.SecretTargets[0].SecretRef.Key).Should(Equal("username"))
		Expect(createdCapp.Spec.Sources[0].TriggerAuth.SecretTargets[0].SecretRef.Name).Should(Equal("kafka-secret"))
	})

})
