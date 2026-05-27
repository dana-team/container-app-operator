package e2e

import (
	"fmt"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/test/e2e/consts"
	"github.com/dana-team/container-app-operator/test/e2e/mocks"
	utilst "github.com/dana-team/container-app-operator/test/e2e/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/util/retry"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
)

const (
	sourceName      = "ping"
	initialSchedule = "* * * * *"
	initialData     = `{"key":"value"}`

	updatedSchedule = "0 * * * *"
	updatedData     = `{"key":"updated"}`
)

// pingSourceObjectName returns the name of the PingSource for a given Capp and source name.
func pingSourceObjectName(cappName, srcName string) string {
	return fmt.Sprintf("%s-%s", cappName, srcName)
}

func createCappWithPingSource(eventSourceSpec cappv1alpha1.EventSourcesSpec) *cappv1alpha1.Capp {
	testCapp := mocks.CreateBaseCapp()
	testCapp.Spec.EventSourcesSpec = eventSourceSpec

	return utilst.CreateCapp(k8sClient, testCapp)
}

// createCappAndWaitForPingSource creates a Capp with the given EventSourcesSpec and waits
// until the first PingSource is created, returning the Capp and the PingSource object name.
func createCappAndWaitForPingSource(spec cappv1alpha1.EventSourcesSpec) (*cappv1alpha1.Capp, string) {
	createdCapp := createCappWithPingSource(spec)
	psName := pingSourceObjectName(createdCapp.Name, spec.Sources[0].Name)

	psObj := &sourcesv1.PingSource{}
	psObj.Name = psName
	psObj.Namespace = consts.NSName
	Eventually(func() bool {
		return utilst.DoesResourceExist(k8sClient, psObj)
	}, consts.Timeout, consts.Interval).Should(BeTrue())

	return createdCapp, psName
}

func newEventSourceSpecWithPingSource() cappv1alpha1.EventSourcesSpec {
	return cappv1alpha1.EventSourcesSpec{
		Sources: []cappv1alpha1.SourceConfiguration{
			{
				Name: sourceName,
				PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{
					Schedule: initialSchedule,
					Data:     initialData,
				},
			},
		},
	}
}

var _ = Describe("Validate PingSource functionality", func() {
	It("Should create a PingSource when adding a PingSource event source to a Capp", func() {
		createdCapp, psName := createCappAndWaitForPingSource(newEventSourceSpecWithPingSource())

		By("Verifying EventingStatus is populated with the source")
		Eventually(func() int {
			return len(utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace).Status.EventingStatus.EventSources)
		}, consts.Timeout, consts.Interval).Should(Equal(1))

		capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
		Expect(capp.Status.EventingStatus.EventSources[0].Name).To(Equal(psName))
	})

	It("Should update the PingSource when the Capp event source spec changes", func() {
		createdCapp, psName := createCappAndWaitForPingSource(newEventSourceSpecWithPingSource())

		By("Updating the Capp PingSource schedule and data")
		err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			capp.Spec.EventSourcesSpec.Sources[0].PingSourceConfiguration.Schedule = updatedSchedule
			capp.Spec.EventSourcesSpec.Sources[0].PingSourceConfiguration.Data = updatedData
			return utilst.UpdateResource(k8sClient, capp)
		})
		Expect(err).ToNot(HaveOccurred())

		By("Verifying PingSource spec is updated")
		Eventually(func() string {
			return utilst.GetPingSource(k8sClient, psName, consts.NSName).Spec.Schedule
		}, consts.Timeout, consts.Interval).Should(Equal(updatedSchedule))

		Eventually(func() string {
			return utilst.GetPingSource(k8sClient, psName, consts.NSName).Spec.Data
		}, consts.Timeout, consts.Interval).Should(Equal(updatedData))
	})

	It("Should delete the PingSource when the event source is removed from the Capp spec", func() {
		createdCapp, psName := createCappAndWaitForPingSource(newEventSourceSpecWithPingSource())

		By("Removing all event sources from Capp spec")
		err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			capp.Spec.EventSourcesSpec = cappv1alpha1.EventSourcesSpec{}
			return utilst.UpdateResource(k8sClient, capp)
		})
		Expect(err).ToNot(HaveOccurred())

		By("Verifying PingSource is deleted")
		psObj := &sourcesv1.PingSource{}
		psObj.Name = psName
		psObj.Namespace = consts.NSName
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, psObj)
		}, consts.Timeout, consts.Interval).Should(BeFalse())

		By("Verifying EventingStatus is cleared")
		Eventually(func() int {
			return len(utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace).Status.EventingStatus.EventSources)
		}, consts.Timeout, consts.Interval).Should(Equal(0))
	})

	It("Should delete owned PingSources when the Capp is deleted", func() {
		createdCapp, psName := createCappAndWaitForPingSource(newEventSourceSpecWithPingSource())

		By("Deleting the Capp")
		utilst.DeleteCapp(k8sClient, createdCapp)

		By("Verifying PingSource is deleted")
		psObj := &sourcesv1.PingSource{}
		psObj.Name = psName
		psObj.Namespace = consts.NSName
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, psObj)
		}, consts.Timeout, consts.Interval).Should(BeFalse())
	})

})
