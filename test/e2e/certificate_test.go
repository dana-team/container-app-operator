package e2e

import (
	"context"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/test/e2e/consts"
	"github.com/dana-team/container-app-operator/test/e2e/mocks"
	"github.com/dana-team/container-app-operator/test/e2e/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Validate Certificate functionality", func() {
	It("Should create, update and delete Certificate when creating, updating and deleting a Capp instance", func() {
		By("Creating an HTTPS Capp")
		createdCapp, routeHostname := utils.CreateHTTPSCapp(k8sClient)

		By("Checking if the Certificate was created successfully")
		certificateName := utils.GenerateResourceName(routeHostname, consts.ZoneValue)
		certificateObject := mocks.CreateCertificateObject(certificateName)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, certificateObject)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Checking the Certificate has the needed labels")
		certificateObject = utils.GetCertificate(k8sClient, certificateName, consts.NSName)
		Expect(certificateObject.Labels[consts.CappResourceKey]).Should(Equal(createdCapp.Name))
		Expect(certificateObject.Labels[consts.ManagedByLabelKey]).Should(Equal(consts.CappKey))

		By("Verifying no Certificate creation failures")
		Consistently(func() bool {
			eventList := &corev1.EventList{}
			err := k8sClient.List(context.Background(), eventList, client.InNamespace(createdCapp.Namespace))
			if err != nil {
				return false
			}
			for _, event := range eventList.Items {
				if event.InvolvedObject.Name == createdCapp.Name && event.Reason == "CertificateCreationFailed" {
					return true
				}
			}
			return false
		}, consts.DefaultConsistently, consts.Interval).Should(BeFalse(), "Should not have Certificate creation failure events")

		By("Updating the Capp Route hostname and checking the status")
		var toBeUpdatedCapp *cappv1alpha1.Capp
		updatedRouteHostname := utils.GenerateResourceName(utils.GenerateRouteHostname(), consts.ZoneValue)

		err := retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
			toBeUpdatedCapp = utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			toBeUpdatedCapp.Spec.RouteSpec.Hostname = updatedRouteHostname

			return utils.UpdateResource(k8sClient, toBeUpdatedCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() string {
			capp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			if capp.Status.RouteStatus.DomainMappingObjectStatus.URL == nil {
				return ""
			}
			return capp.Status.RouteStatus.DomainMappingObjectStatus.URL.Host
		}, consts.Timeout, consts.Interval).Should(Equal(updatedRouteHostname), "Should update Route Status of Capp")

		By("checking if the Certificate object was updated after changing the Capp Route Hostname")
		updatedCertificateObject := mocks.CreateCertificateObject(updatedRouteHostname)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, updatedCertificateObject)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")

		Eventually(func() []string {
			updatedCertificateObject = utils.GetCertificate(k8sClient, updatedRouteHostname, toBeUpdatedCapp.Namespace)
			return updatedCertificateObject.Spec.DNSNames
		}, consts.Timeout, consts.Interval).Should(Equal([]string{updatedRouteHostname}))

		By("Deleting the Capp instance and checking if the Certificate was deleted successfully")
		utils.DeleteCapp(k8sClient, createdCapp)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, updatedCertificateObject)
		}, consts.Timeout, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")
	})

	It("Should not create Certificate when creating a non-HTTPS Capp instance", func() {
		By("Creating a Capp with a route")
		_, routeHostname := utils.CreateCappWithHTTPHostname(k8sClient)

		By("Checking if the Certificate was not created")
		certificateName := utils.GenerateResourceName(routeHostname, consts.ZoneValue)
		certificateObject := mocks.CreateCertificateObject(certificateName)
		Consistently(func() bool {
			return utils.DoesResourceExist(k8sClient, certificateObject)
		}, consts.DefaultConsistently, consts.Interval).Should(BeFalse(), "Should not find a resource.")
	})

	It("Should cleanup Certificate when no longer required (tls)", func() {
		By("Creating an HTTPS Capp")
		createdCapp, routeHostname := utils.CreateHTTPSCapp(k8sClient)

		By("Checking if the Certificate was created successfully")
		certificateName := utils.GenerateResourceName(routeHostname, consts.ZoneValue)
		certificateObject := mocks.CreateCertificateObject(certificateName)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, certificateObject)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Removing the Certificate requirement from Capp Spec and checking cleanup", func() {
			err := retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
				toBeUpdatedCapp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
				toBeUpdatedCapp.Spec.RouteSpec.TlsEnabled = false

				return utils.UpdateResource(k8sClient, toBeUpdatedCapp)
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				return utils.DoesResourceExist(k8sClient, certificateObject)
			}, consts.Timeout, consts.Interval).Should(BeFalse(), "Should not find a resource.")
		})
	})

	It("Should cleanup Certificate when no longer required (hostname)", func() {
		By("Creating an HTTPS Capp")
		createdCapp, routeHostname := utils.CreateHTTPSCapp(k8sClient)

		By("Checking if the Certificate was created successfully")
		certificateName := utils.GenerateResourceName(routeHostname, consts.ZoneValue)
		certificateObject := mocks.CreateCertificateObject(certificateName)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, certificateObject)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Removing the Certificate requirement from Capp Spec and checking cleanup", func() {
			err := retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
				toBeUpdatedCapp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
				toBeUpdatedCapp.Spec.RouteSpec.Hostname = ""
				toBeUpdatedCapp.Spec.RouteSpec.TlsEnabled = false

				return utils.UpdateResource(k8sClient, toBeUpdatedCapp)
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				return utils.DoesResourceExist(k8sClient, certificateObject)
			}, consts.Timeout, consts.Interval).Should(BeFalse(), "Should not find a resource.")
		})
	})

})
