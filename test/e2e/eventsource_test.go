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
	pingSourceName     = "ping"
	pingSourceSchedule = "* * * * *"
	pingSourceData     = `{"key":"value"}`

	updatedSchedule = "0 * * * *"
	updatedData     = `{"key":"updated"}`
)

// pingSourceObjectName returns the name of the PingSource for a given Capp and source name.
func pingSourceObjectName(cappName, sourceName string) string {
	return fmt.Sprintf("%s-%s", cappName, sourceName)
}

var _ = Describe("Validate event source functionality", func() {
	It("Should create a PingSource when adding a PingSource event source to a Capp", func() {
		By("Creating a Capp with a PingSource event source")
		testCapp := mocks.CreateBaseCapp()
		testCapp.Spec.EventSourcesSpec = cappv1alpha1.EventSourcesSpec{
			Sources: []cappv1alpha1.SourceConfiguration{
				{
					Name: pingSourceName,
					PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{
						Schedule: pingSourceSchedule,
						Data:     pingSourceData,
					},
				},
			},
		}
		createdCapp := utilst.CreateCapp(k8sClient, testCapp)

		By("Verifying PingSource is created with the correct name")
		psName := pingSourceObjectName(createdCapp.Name, pingSourceName)
		pingSourceObj := &sourcesv1.PingSource{}
		pingSourceObj.Name = psName
		pingSourceObj.Namespace = consts.NSName

		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, pingSourceObj)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "PingSource should be created")

		By("Verifying PingSource spec has the correct schedule and data")
		ps := utilst.GetPingSource(k8sClient, psName, consts.NSName)
		Expect(ps.Spec.Schedule).To(Equal(pingSourceSchedule))
		Expect(ps.Spec.Data).To(Equal(pingSourceData))

		By("Verifying PingSource sink points to the Capp's Knative Service")
		Expect(ps.Spec.Sink.Ref).NotTo(BeNil())
		Expect(ps.Spec.Sink.Ref.Name).To(Equal(createdCapp.Name))
		Expect(ps.Spec.Sink.Ref.Namespace).To(Equal(createdCapp.Namespace))
		Expect(ps.Spec.Sink.Ref.Kind).To(Equal("Service"))
	})

	It("Should populate EventingStatus in Capp status after creating a PingSource", func() {
		By("Creating a Capp with a PingSource event source")
		testCapp := mocks.CreateBaseCapp()
		testCapp.Spec.EventSourcesSpec = cappv1alpha1.EventSourcesSpec{
			Sources: []cappv1alpha1.SourceConfiguration{
				{
					Name: pingSourceName,
					PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{
						Schedule: pingSourceSchedule,
						Data:     pingSourceData,
					},
				},
			},
		}
		createdCapp := utilst.CreateCapp(k8sClient, testCapp)

		By("Verifying EventingStatus is populated with one entry")
		psName := pingSourceObjectName(createdCapp.Name, pingSourceName)
		Eventually(func() int {
			return len(utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace).Status.EventingStatus.EventSources)
		}, consts.Timeout, consts.Interval).Should(Equal(1))

		By("Verifying EventSourceStatus contains the correct source name")
		capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
		Expect(capp.Status.EventingStatus.EventSources[0].Name).To(Equal(psName))
	})

	It("Should update the PingSource when the Capp event source spec changes", func() {
		By("Creating a Capp with a PingSource event source")
		testCapp := mocks.CreateBaseCapp()
		testCapp.Spec.EventSourcesSpec = cappv1alpha1.EventSourcesSpec{
			Sources: []cappv1alpha1.SourceConfiguration{
				{
					Name: pingSourceName,
					PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{
						Schedule: pingSourceSchedule,
						Data:     pingSourceData,
					},
				},
			},
		}
		createdCapp := utilst.CreateCapp(k8sClient, testCapp)
		psName := pingSourceObjectName(createdCapp.Name, pingSourceName)

		By("Waiting for PingSource to be created")
		pingSourceObj := &sourcesv1.PingSource{}
		pingSourceObj.Name = psName
		pingSourceObj.Namespace = consts.NSName
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, pingSourceObj)
		}, consts.Timeout, consts.Interval).Should(BeTrue())

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
		By("Creating a Capp with a PingSource event source")
		testCapp := mocks.CreateBaseCapp()
		testCapp.Spec.EventSourcesSpec = cappv1alpha1.EventSourcesSpec{
			Sources: []cappv1alpha1.SourceConfiguration{
				{
					Name: pingSourceName,
					PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{
						Schedule: pingSourceSchedule,
						Data:     pingSourceData,
					},
				},
			},
		}
		createdCapp := utilst.CreateCapp(k8sClient, testCapp)
		psName := pingSourceObjectName(createdCapp.Name, pingSourceName)

		By("Waiting for PingSource to be created")
		pingSourceObj := &sourcesv1.PingSource{}
		pingSourceObj.Name = psName
		pingSourceObj.Namespace = consts.NSName
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, pingSourceObj)
		}, consts.Timeout, consts.Interval).Should(BeTrue())

		By("Removing all event sources from Capp spec")
		err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			capp.Spec.EventSourcesSpec = cappv1alpha1.EventSourcesSpec{}
			return utilst.UpdateResource(k8sClient, capp)
		})
		Expect(err).ToNot(HaveOccurred())

		By("Verifying PingSource is deleted")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, pingSourceObj)
		}, consts.Timeout, consts.Interval).Should(BeFalse(), "PingSource should be deleted after source removed from Capp spec")

		By("Verifying EventingStatus is cleared")
		Eventually(func() int {
			return len(utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace).Status.EventingStatus.EventSources)
		}, consts.Timeout, consts.Interval).Should(Equal(0))
	})

	It("Should delete owned PingSources when the Capp is deleted", func() {
		By("Creating a Capp with a PingSource event source")
		testCapp := mocks.CreateBaseCapp()
		testCapp.Spec.EventSourcesSpec = cappv1alpha1.EventSourcesSpec{
			Sources: []cappv1alpha1.SourceConfiguration{
				{
					Name: pingSourceName,
					PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{
						Schedule: pingSourceSchedule,
						Data:     pingSourceData,
					},
				},
			},
		}
		createdCapp := utilst.CreateCapp(k8sClient, testCapp)
		psName := pingSourceObjectName(createdCapp.Name, pingSourceName)

		By("Waiting for PingSource to be created")
		pingSourceObj := &sourcesv1.PingSource{}
		pingSourceObj.Name = psName
		pingSourceObj.Namespace = consts.NSName
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, pingSourceObj)
		}, consts.Timeout, consts.Interval).Should(BeTrue())

		By("Deleting the Capp")
		utilst.DeleteCapp(k8sClient, createdCapp)

		By("Verifying PingSource is deleted")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, pingSourceObj)
		}, consts.Timeout, consts.Interval).Should(BeFalse(), "PingSource should be deleted when Capp is deleted")
	})

	It("Should create multiple PingSources and track all in EventingStatus", func() {
		const secondSourceName = "ping2"

		By("Creating a Capp with two PingSource event sources")
		testCapp := mocks.CreateBaseCapp()
		testCapp.Spec.EventSourcesSpec = cappv1alpha1.EventSourcesSpec{
			Sources: []cappv1alpha1.SourceConfiguration{
				{
					Name: pingSourceName,
					PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{
						Schedule: pingSourceSchedule,
						Data:     pingSourceData,
					},
				},
				{
					Name: secondSourceName,
					PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{
						Schedule: updatedSchedule,
						Data:     updatedData,
					},
				},
			},
		}
		createdCapp := utilst.CreateCapp(k8sClient, testCapp)

		By("Verifying both PingSources are created")
		ps1Name := pingSourceObjectName(createdCapp.Name, pingSourceName)
		ps2Name := pingSourceObjectName(createdCapp.Name, secondSourceName)

		ps1Obj := &sourcesv1.PingSource{}
		ps1Obj.Name = ps1Name
		ps1Obj.Namespace = consts.NSName
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, ps1Obj)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "First PingSource should be created")

		ps2Obj := &sourcesv1.PingSource{}
		ps2Obj.Name = ps2Name
		ps2Obj.Namespace = consts.NSName
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, ps2Obj)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Second PingSource should be created")

		By("Verifying EventingStatus contains both sources")
		Eventually(func() int {
			return len(utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace).Status.EventingStatus.EventSources)
		}, consts.Timeout, consts.Interval).Should(Equal(2))
	})
})
