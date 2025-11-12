package e2e_tests

import (
	"github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"
	"k8s.io/client-go/util/retry"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Validate DNSRecord functionality", func() {
	It("Should create, update and delete DNSRecord when creating, updating and deleting a Capp instance", func() {
		By("Creating a capp with a route")
		createdCapp, _ := utilst.CreateCappWithHTTPHostname(k8sClient)

		By("Checking if the DNSRecord was created successfully")
		dnsRecordName := utilst.GenerateResourceName(createdCapp.Spec.RouteSpec.Hostname, testconsts.ZoneValue)
		dnsRecordObject := mocks.CreateDNSRecordObject(dnsRecordName)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, dnsRecordObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Checking the DNSRecord has the needed labels")
		dnsRecordObject = utilst.GetDNSRecord(k8sClient, dnsRecordName, createdCapp.Namespace)
		Expect(dnsRecordObject.Labels[testconsts.CappResourceKey]).Should(Equal(createdCapp.Name))
		Expect(dnsRecordObject.Labels[testconsts.CappNamespaceKey]).Should(Equal(createdCapp.Namespace))
		Expect(dnsRecordObject.Labels[testconsts.ManagedByLabelKey]).Should(Equal(testconsts.CappKey))

		By("checking if the DNSRecord object was updated after changing the Capp Route Hostname")
		updatedRouteHostname := utilst.GenerateRouteHostname()

		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			toBeUpdatedCapp.Spec.RouteSpec.Hostname = updatedRouteHostname

			return utilst.UpdateResource(k8sClient, toBeUpdatedCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		updatedDNSRecord := dnsRecordObject
		updatedDNSRecordName := utilst.GenerateResourceName(updatedRouteHostname, testconsts.ZoneValue)
		Eventually(func() *string {
			updatedDNSRecord = utilst.GetDNSRecord(k8sClient, updatedDNSRecordName, createdCapp.Namespace)
			return updatedDNSRecord.Spec.ForProvider.Name
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(&updatedRouteHostname))

		By("Deleting the Capp instance")
		utilst.DeleteCapp(k8sClient, createdCapp)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, createdCapp)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the DNSRecord was deleted successfully")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, updatedDNSRecord)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")
	})

	It("Should cleanup DNSRecord when no longer required", func() {
		By("Creating a capp with a route")
		createdCapp, _ := utilst.CreateCappWithHTTPHostname(k8sClient)

		By("Checking if the DNSRecord was created successfully")
		dnsRecordName := utilst.GenerateResourceName(createdCapp.Spec.RouteSpec.Hostname, testconsts.ZoneValue)
		dnsRecordObject := mocks.CreateDNSRecordObject(dnsRecordName)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, dnsRecordObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Removing the DNSRecord requirement from Capp Spec and checking cleanup", func() {
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
				toBeUpdatedCapp.Spec.RouteSpec.Hostname = ""

				return utilst.UpdateResource(k8sClient, toBeUpdatedCapp)
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				return utilst.DoesResourceExist(k8sClient, dnsRecordObject)
			}, testconsts.Timeout, testconsts.Interval).Should(BeFalse(), "Should not find a resource.")
		})
	})
})
