package e2e

import (
	"context"
	"fmt"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/test/e2e/consts"
	"github.com/dana-team/container-app-operator/test/e2e/mocks"
	utilst "github.com/dana-team/container-app-operator/test/e2e/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Validate PingSource functionality", func() {
	It("Should create, update, and delete PingSource when managing a Capp with event sources", func() {
		By("Creating a Capp with a PingSource")
		testCapp := mocks.CreateBaseCapp()
		cappName := utilst.GenerateCappName()
		testCapp.Name = cappName
		mocks.AddPingSource(testCapp, "heartbeat", "*/5 * * * *", `{"msg":"hello"}`)
		Expect(k8sClient.Create(context.Background(), testCapp)).To(Succeed())

		pingSourceName := fmt.Sprintf("%s-heartbeat", cappName)

		By("Checking PingSource was created")
		psObj := mocks.CreatePingSourceObject(pingSourceName)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, psObj)
		}, consts.Timeout, consts.Interval).Should(BeTrue())

		By("Checking PingSource has correct labels and schedule")
		Eventually(func() string {
			ps := mocks.CreatePingSourceObject(pingSourceName)
			if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: consts.NSName, Name: pingSourceName}, ps); err != nil {
				return ""
			}
			return ps.Spec.Schedule
		}, consts.Timeout, consts.Interval).Should(Equal("*/5 * * * *"))

		Eventually(func() string {
			ps := mocks.CreatePingSourceObject(pingSourceName)
			if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: consts.NSName, Name: pingSourceName}, ps); err != nil {
				return ""
			}
			return ps.Labels[consts.CappResourceKey]
		}, consts.Timeout, consts.Interval).Should(Equal(cappName))

		By("Updating the PingSource schedule")
		updatedCapp := utilst.GetCapp(k8sClient, cappName, consts.NSName)
		updatedCapp.Spec.EventSourcesSpec.Sources[0].PingSource.Schedule = "0 * * * *"
		Expect(k8sClient.Update(context.Background(), updatedCapp)).To(Succeed())

		Eventually(func() string {
			ps := mocks.CreatePingSourceObject(pingSourceName)
			if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: consts.NSName, Name: pingSourceName}, ps); err != nil {
				return ""
			}
			return ps.Spec.Schedule
		}, consts.Timeout, consts.Interval).Should(Equal("0 * * * *"))

		By("Removing the PingSource from spec — orphan should be deleted")
		updatedCapp = utilst.GetCapp(k8sClient, cappName, consts.NSName)
		updatedCapp.Spec.EventSourcesSpec.Sources = nil
		Expect(k8sClient.Update(context.Background(), updatedCapp)).To(Succeed())

		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, mocks.CreatePingSourceObject(pingSourceName))
		}, consts.Timeout, consts.Interval).Should(BeFalse())

		By("Deleting the Capp")
		utilst.DeleteCapp(k8sClient, testCapp)
	})

	It("Should clean up all PingSources on Capp deletion", func() {
		By("Creating a Capp with two named PingSources")
		testCapp := mocks.CreateBaseCapp()
		cappName := utilst.GenerateCappName()
		testCapp.Name = cappName
		mocks.AddPingSource(testCapp, "ping-a", "*/1 * * * *", `{"src":"a"}`)
		mocks.AddPingSource(testCapp, "ping-b", "*/2 * * * *", `{"src":"b"}`)
		Expect(k8sClient.Create(context.Background(), testCapp)).To(Succeed())

		psA := mocks.CreatePingSourceObject(fmt.Sprintf("%s-ping-a", cappName))
		psB := mocks.CreatePingSourceObject(fmt.Sprintf("%s-ping-b", cappName))

		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, psA) && utilst.DoesResourceExist(k8sClient, psB)
		}, consts.Timeout, consts.Interval).Should(BeTrue())

		By("Deleting the Capp — both PingSources should be removed")
		utilst.DeleteCapp(k8sClient, testCapp)

		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, psA)
		}, consts.Timeout, consts.Interval).Should(BeFalse())
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, psB)
		}, consts.Timeout, consts.Interval).Should(BeFalse())
	})

	It("Should use index-based name when no explicit name is provided", func() {
		By("Creating a Capp with an unnamed PingSource")
		testCapp := mocks.CreateBaseCapp()
		cappName := utilst.GenerateCappName()
		testCapp.Name = cappName
		testCapp.Spec.EventSourcesSpec.Sources = []cappv1alpha1.EventSource{
			{
				PingSource: &cappv1alpha1.PingSourceSpec{
					Schedule: "*/3 * * * *",
				},
			},
		}
		Expect(k8sClient.Create(context.Background(), testCapp)).To(Succeed())

		derivedName := fmt.Sprintf("%s-pingsource-0", cappName)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, mocks.CreatePingSourceObject(derivedName))
		}, consts.Timeout, consts.Interval).Should(BeTrue())

		By("Deleting the Capp")
		utilst.DeleteCapp(k8sClient, testCapp)
	})
})
