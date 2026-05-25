package e2e

import (
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/dana-team/container-app-operator/test/e2e/consts"
	"github.com/dana-team/container-app-operator/test/e2e/mocks"
	"github.com/dana-team/container-app-operator/test/e2e/utils"
	"k8s.io/client-go/util/retry"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Validate DNSRecord functionality", func() {
	It("Should create, update and delete DNSRecord when creating, updating and deleting a Capp instance", func() {
		By("Creating a capp with a route")
		createdCapp, _ := utils.CreateCappWithHTTPHostname(k8sClient)

		By("Checking if the DNSRecord was created successfully")
		dnsRecordName := utils.GenerateResourceName(createdCapp.Spec.RouteSpec.Hostname, consts.ZoneValue)
		dnsRecordObject := mocks.CreateDNSRecordObject(dnsRecordName)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, dnsRecordObject)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Checking the DNSRecord has the needed labels")
		dnsRecordObject = utils.GetDNSRecord(k8sClient, dnsRecordName, createdCapp.Namespace)
		Expect(dnsRecordObject.Labels[consts.CappResourceKey]).Should(Equal(createdCapp.Name))
		Expect(dnsRecordObject.Labels[consts.CappNamespaceKey]).Should(Equal(createdCapp.Namespace))
		Expect(dnsRecordObject.Labels[consts.ManagedByLabelKey]).Should(Equal(consts.CappKey))

		By("checking if the DNSRecord object was updated after changing the Capp Route Hostname")
		updatedRouteHostname := utils.GenerateRouteHostname()

		err := retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
			toBeUpdatedCapp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			utils.ConfirmHostnameChange(toBeUpdatedCapp, updatedRouteHostname)

			return utils.UpdateResource(k8sClient, toBeUpdatedCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		updatedDNSRecord := dnsRecordObject
		updatedDNSRecordName := utils.GenerateResourceName(updatedRouteHostname, consts.ZoneValue)
		Eventually(func() *string {
			updatedDNSRecord = utils.GetDNSRecord(k8sClient, updatedDNSRecordName, createdCapp.Namespace)
			return updatedDNSRecord.Spec.ForProvider.Name
		}, consts.Timeout, consts.Interval).Should(Equal(&updatedRouteHostname))

		By("Deleting the Capp instance")
		utils.DeleteCapp(k8sClient, createdCapp)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, createdCapp)
		}, consts.Timeout, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the DNSRecord was deleted successfully")
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, updatedDNSRecord)
		}, consts.Timeout, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")
	})

	It("Should cleanup DNSRecord when no longer required", func() {
		By("Creating a capp with a route")
		createdCapp, _ := utils.CreateCappWithHTTPHostname(k8sClient)

		By("Checking if the DNSRecord was created successfully")
		dnsRecordName := utils.GenerateResourceName(createdCapp.Spec.RouteSpec.Hostname, consts.ZoneValue)
		dnsRecordObject := mocks.CreateDNSRecordObject(dnsRecordName)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, dnsRecordObject)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Removing the DNSRecord requirement from Capp Spec and checking cleanup", func() {
			err := retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
				toBeUpdatedCapp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
				utils.ConfirmHostnameChange(toBeUpdatedCapp, "")

				return utils.UpdateResource(k8sClient, toBeUpdatedCapp)
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				return utils.DoesResourceExist(k8sClient, dnsRecordObject)
			}, consts.Timeout, consts.Interval).Should(BeFalse(), "Should not find a resource.")
		})
	})

	It("Should not update DNSRecord when only Capp metadata changes", func() {
		By("Creating a capp with a route")
		createdCapp, _ := utils.CreateCappWithHTTPHostname(k8sClient)

		By("Checking if the DNSRecord was created successfully")
		dnsRecordName := utils.GenerateResourceName(createdCapp.Spec.RouteSpec.Hostname, consts.ZoneValue)
		dnsRecordObject := mocks.CreateDNSRecordObject(dnsRecordName)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, dnsRecordObject)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Simulating external mutation of DNSRecord spec")
		err := retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
			dnsRecord := utils.GetDNSRecord(k8sClient, dnsRecordName, createdCapp.Namespace)
			dnsRecord.Spec.ManagementPolicies = []xpv1.ManagementAction{"Create"}
			return utils.UpdateResource(k8sClient, dnsRecord)
		})
		Expect(err).ToNot(HaveOccurred())

		By("Confirming external mutation is persisted")
		Eventually(func() []xpv1.ManagementAction {
			currentDNSRecord := utils.GetDNSRecord(k8sClient, dnsRecordName, createdCapp.Namespace)
			return currentDNSRecord.Spec.ManagementPolicies
		}, consts.Timeout, consts.Interval).Should(Equal([]xpv1.ManagementAction{"Create"}))

		By("Should preserve external DNSRecord spec fields on Capp metadata-only changes")
		Consistently(func() []xpv1.ManagementAction {
			currentDNSRecord := utils.GetDNSRecord(k8sClient, dnsRecordName, createdCapp.Namespace)
			return currentDNSRecord.Spec.ManagementPolicies
		}, consts.DefaultConsistently, consts.Interval).Should(Equal([]xpv1.ManagementAction{"Create"}))
	})
})
