package e2e_tests

import (
	mock "github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Validate DNSEndpoint functionality", func() {
	It("Should create, update and delete DNSEndpoint when creating, updating and deleting a Capp instance", func() {
		By("Creating a capp with a route")
		routeCapp := mock.CreateBaseCapp()
		routeHostname := utilst.GenerateRouteHostname()
		routeCapp.Spec.RouteSpec.Hostname = routeHostname
		createdCapp := utilst.CreateCapp(k8sClient, routeCapp)

		By("Checking if the DNSEndpoint was created successfully")
		dnsEndpointObject := mock.CreateDNSEndpointObject(createdCapp.Name)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, dnsEndpointObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		By("checking if the DNSEndpoint object was updated after changing the Capp Route Hostname")
		toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
		updatedRouteHostname := utilst.GenerateRouteHostname()
		toBeUpdatedCapp.Spec.RouteSpec.Hostname = updatedRouteHostname
		utilst.UpdateCapp(k8sClient, toBeUpdatedCapp)

		updatedDNSEndpoint := dnsEndpointObject
		Eventually(func() string {
			updatedDNSEndpoint = utilst.GetDNSEndpoint(k8sClient, toBeUpdatedCapp.Name, toBeUpdatedCapp.Namespace)
			return updatedDNSEndpoint.Spec.Endpoints[0].DNSName
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(updatedRouteHostname))

		By("Deleting the capp instance")
		utilst.DeleteCapp(k8sClient, createdCapp)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, createdCapp)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the DNSEndpoint was deleted successfully")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, updatedDNSEndpoint)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")
	})
})
