package e2e

import (
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/test/e2e/consts"
	"github.com/dana-team/container-app-operator/test/e2e/mocks"
	utilst "github.com/dana-team/container-app-operator/test/e2e/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/retry"
	kafkasourcev1 "knative.dev/eventing-kafka-broker/control-plane/pkg/apis/sources/v1"
)

const (
	kafkaSourceName      = "kafka"
	kafkaBootstrapServer = "kafka.example:9092"
	kafkaInitialTopic    = "orders"
	kafkaUpdatedTopic    = "payments"
)

func createCappAndWaitForKafkaSource(spec cappv1alpha1.EventSourcesSpec) (*cappv1alpha1.Capp, string) {
	utilst.CreateSecret(k8sClient, mocks.CreateKafkaCredentialsSecret())
	testCapp := mocks.CreateBaseCapp()
	testCapp.Spec.EventSourcesSpec = spec
	createdCapp := utilst.CreateCapp(k8sClient, testCapp)
	ksName := createdCapp.Name + "-" + spec.Sources[0].Name

	ksObj := &kafkasourcev1.KafkaSource{}
	ksObj.Name = ksName
	ksObj.Namespace = consts.NSName
	Eventually(func() bool {
		return utilst.DoesResourceExist(k8sClient, ksObj)
	}, consts.Timeout, consts.Interval).Should(BeTrue())

	return createdCapp, ksName
}

func newEventSourceSpec() cappv1alpha1.EventSourcesSpec {
	return cappv1alpha1.EventSourcesSpec{
		Sources: []cappv1alpha1.SourceConfiguration{
			{
				Name: kafkaSourceName,
				KafkaSourceConfiguration: &cappv1alpha1.KafkaSourceConfiguration{
					BootstrapServers: []string{kafkaBootstrapServer},
					Topics:           []string{kafkaInitialTopic},
					SecretRef:        corev1.LocalObjectReference{Name: mocks.KafkaSecretName},
				},
			},
		},
	}
}

var _ = Describe("Validate KafkaSource functionality", func() {
	It("Should create a KafkaSource when adding a Kafka event source to a Capp", func() {
		createdCapp, ksName := createCappAndWaitForKafkaSource(newEventSourceSpec())

		By("Verifying EventingStatus is populated with the source")
		Eventually(func(g Gomega) {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			g.Expect(capp.Status.EventingStatus.EventSources).To(HaveLen(1))
			g.Expect(capp.Status.EventingStatus.EventSources[0].Name).To(Equal(ksName))
		}, consts.Timeout, consts.Interval).Should(Succeed())
	})

	It("Should update the KafkaSource when the Capp event source spec changes", func() {
		createdCapp, ksName := createCappAndWaitForKafkaSource(newEventSourceSpec())

		By("Updating the Capp KafkaSource topics")
		err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			capp.Spec.EventSourcesSpec.Sources[0].KafkaSourceConfiguration.Topics = []string{kafkaUpdatedTopic}
			return utilst.UpdateResource(k8sClient, capp)
		})
		Expect(err).ToNot(HaveOccurred())

		By("Verifying KafkaSource spec is updated")
		Eventually(func() []string {
			ks := &kafkasourcev1.KafkaSource{}
			utilst.GetResource(k8sClient, ks, ksName, consts.NSName)
			return ks.Spec.Topics
		}, consts.Timeout, consts.Interval).Should(Equal([]string{kafkaUpdatedTopic}))
	})

	It("Should delete the KafkaSource when the event source is removed from the Capp spec", func() {
		createdCapp, ksName := createCappAndWaitForKafkaSource(newEventSourceSpec())

		By("Removing all event sources from Capp spec")
		err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			capp.Spec.EventSourcesSpec = cappv1alpha1.EventSourcesSpec{}
			return utilst.UpdateResource(k8sClient, capp)
		})
		Expect(err).ToNot(HaveOccurred())

		By("Verifying KafkaSource is deleted")
		ksObj := &kafkasourcev1.KafkaSource{}
		ksObj.Name = ksName
		ksObj.Namespace = consts.NSName
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, ksObj)
		}, consts.Timeout, consts.Interval).Should(BeFalse())

		By("Verifying EventingStatus is cleared")
		Eventually(func() int {
			return len(utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace).Status.EventingStatus.EventSources)
		}, consts.Timeout, consts.Interval).Should(Equal(0))
	})
})
