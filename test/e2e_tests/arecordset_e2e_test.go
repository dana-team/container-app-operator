package e2e_tests

import (
	mock "github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Validate ARecordSet functionality", func() {
	It("Should create, update and delete ARecordSet when creating, updating and deleting a Capp instance", func() {
		By("Creating a capp with a route")
		createdCapp, _ := utilst.CreateCappWithHTTPHostname(k8sClient)

		By("Checking if the ARecordSet was created successfully")
		aRecordSetObject := mock.CreateARecordSetObject(createdCapp.Spec.RouteSpec.Hostname)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, aRecordSetObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		By("checking if the ARecordSet object was updated after changing the Capp Route Hostname")
		toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
		updatedRouteHostname := utilst.GenerateRouteHostname()
		toBeUpdatedCapp.Spec.RouteSpec.Hostname = updatedRouteHostname
		utilst.UpdateCapp(k8sClient, toBeUpdatedCapp)

		updatedARecordSet := aRecordSetObject
		Eventually(func() *string {
			updatedARecordSet = utilst.GetARecordSet(k8sClient, updatedRouteHostname)
			return updatedARecordSet.Spec.ForProvider.Name
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(&updatedRouteHostname))

		By("Deleting the Capp instance")
		utilst.DeleteCapp(k8sClient, createdCapp)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, createdCapp)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the ARecordSet was deleted successfully")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, updatedARecordSet)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")
	})

	It("Should cleanup ARecordSet when no longer required", func() {
		By("Creating a capp with a route")
		createdCapp, _ := utilst.CreateCappWithHTTPHostname(k8sClient)

		By("Checking if the ARecordSet was created successfully")
		aRecordSetObject := mock.CreateARecordSetObject(createdCapp.Spec.RouteSpec.Hostname)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, aRecordSetObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Removing the ARecordSet requirement from Capp Spec and checking cleanup", func() {
			toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			toBeUpdatedCapp.Spec.RouteSpec.Hostname = ""
			utilst.UpdateCapp(k8sClient, toBeUpdatedCapp)

			Eventually(func() bool {
				return utilst.DoesResourceExist(k8sClient, aRecordSetObject)
			}, testconsts.Timeout, testconsts.Interval).Should(BeFalse(), "Should not find a resource.")
		})
	})
})
