package e2e_tests

import (
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/util/retry"
)

var _ = Describe("Validate Certificate functionality", func() {
	It("Should create, update and delete Certificate when creating, updating and deleting a Capp instance", func() {
		By("Creating an HTTPS Capp")
		createdCapp, routeHostname := utilst.CreateHTTPSCapp(k8sClient)

		By("Checking if the Certificate was created successfully")
		certificateName := utilst.GenerateResourceName(routeHostname, testconsts.ZoneValue)
		certificateObject := mocks.CreateCertificateObject(certificateName)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, certificateObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Checking the Certificate has the needed labels")
		certificateObject = utilst.GetCertificate(k8sClient, certificateName, testconsts.NSName)
		Expect(certificateObject.Labels[testconsts.CappResourceKey]).Should(Equal(createdCapp.Name))
		Expect(certificateObject.Labels[testconsts.ManagedByLabelKey]).Should(Equal(testconsts.CappKey))

		By("Updating the Capp Route hostname and checking the status")
		var toBeUpdatedCapp *cappv1alpha1.Capp
		updatedRouteHostname := utilst.GenerateResourceName(utilst.GenerateRouteHostname(), testconsts.ZoneValue)

		err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			toBeUpdatedCapp = utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			toBeUpdatedCapp.Spec.RouteSpec.Hostname = updatedRouteHostname

			return utilst.UpdateResource(k8sClient, toBeUpdatedCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() string {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			if capp.Status.RouteStatus.DomainMappingObjectStatus.URL == nil {
				return ""
			}
			return capp.Status.RouteStatus.DomainMappingObjectStatus.URL.Host
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(updatedRouteHostname), "Should update Route Status of Capp")

		By("checking if the Certificate object was updated after changing the Capp Route Hostname")
		updatedCertificateObject := mocks.CreateCertificateObject(updatedRouteHostname)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, updatedCertificateObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		Eventually(func() []string {
			updatedCertificateObject = utilst.GetCertificate(k8sClient, updatedRouteHostname, toBeUpdatedCapp.Namespace)
			return updatedCertificateObject.Spec.DNSNames
		}, testconsts.Timeout, testconsts.Interval).Should(Equal([]string{updatedRouteHostname}))

		By("Deleting the Capp instance and checking if the Certificate was deleted successfully")
		utilst.DeleteCapp(k8sClient, createdCapp)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, updatedCertificateObject)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")
	})

	It("Should not create Certificate when creating a non-HTTPS Capp instance", func() {
		By("Creating a Capp with a route")
		_, routeHostname := utilst.CreateCappWithHTTPHostname(k8sClient)

		By("Checking if the Certificate was not created")
		certificateName := utilst.GenerateResourceName(routeHostname, testconsts.ZoneValue)
		certificateObject := mocks.CreateCertificateObject(certificateName)
		Consistently(func() bool {
			return utilst.DoesResourceExist(k8sClient, certificateObject)
		}, testconsts.DefaultConsistently, testconsts.Interval).Should(BeFalse(), "Should not find a resource.")
	})

	It("Should cleanup Certificate when no longer required (tls)", func() {
		By("Creating an HTTPS Capp")
		createdCapp, routeHostname := utilst.CreateHTTPSCapp(k8sClient)

		By("Checking if the Certificate was created successfully")
		certificateName := utilst.GenerateResourceName(routeHostname, testconsts.ZoneValue)
		certificateObject := mocks.CreateCertificateObject(certificateName)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, certificateObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Removing the Certificate requirement from Capp Spec and checking cleanup", func() {
			err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
				toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
				toBeUpdatedCapp.Spec.RouteSpec.TlsEnabled = false

				return utilst.UpdateResource(k8sClient, toBeUpdatedCapp)
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				return utilst.DoesResourceExist(k8sClient, certificateObject)
			}, testconsts.Timeout, testconsts.Interval).Should(BeFalse(), "Should not find a resource.")
		})
	})

	It("Should cleanup Certificate when no longer required (hostname)", func() {
		By("Creating an HTTPS Capp")
		createdCapp, routeHostname := utilst.CreateHTTPSCapp(k8sClient)

		By("Checking if the Certificate was created successfully")
		certificateName := utilst.GenerateResourceName(routeHostname, testconsts.ZoneValue)
		certificateObject := mocks.CreateCertificateObject(certificateName)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, certificateObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Removing the Certificate requirement from Capp Spec and checking cleanup", func() {
			err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
				toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
				toBeUpdatedCapp.Spec.RouteSpec.Hostname = ""

				return utilst.UpdateResource(k8sClient, toBeUpdatedCapp)
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				return utilst.DoesResourceExist(k8sClient, certificateObject)
			}, testconsts.Timeout, testconsts.Interval).Should(BeFalse(), "Should not find a resource.")
		})
	})

})
