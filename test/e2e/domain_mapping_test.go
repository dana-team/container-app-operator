package e2e

import (
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/test/e2e/consts"
	"github.com/dana-team/container-app-operator/test/e2e/mocks"
	"github.com/dana-team/container-app-operator/test/e2e/utils"
	"k8s.io/client-go/util/retry"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Validate DomainMapping functionality", func() {
	It("Should create, update and delete DomainMapping when creating, updating and deleting a Capp instance", func() {
		By("Creating a capp with a route")
		createdCapp, routeHostname := utils.CreateCappWithHTTPHostname(k8sClient)

		By("Checking if the domainMapping was created successfully")
		domainMappingName := utils.GenerateResourceName(routeHostname, consts.ZoneValue)
		domainMappingObject := mocks.CreateDomainMappingObject(domainMappingName)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, domainMappingObject)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Checking the domainMapping has the needed labels")
		domainMappingObject = utils.GetDomainMapping(k8sClient, domainMappingName, consts.NSName)
		Expect(domainMappingObject.Labels[consts.CappResourceKey]).Should(Equal(createdCapp.Name))
		Expect(domainMappingObject.Labels[consts.ManagedByLabelKey]).Should(Equal(consts.CappKey))

		By("Checking if the RouteStatus of the Capp was updated successfully")
		Eventually(func() string {
			capp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			if capp.Status.RouteStatus.DomainMappingObjectStatus.URL == nil {
				return ""
			}
			return capp.Status.RouteStatus.DomainMappingObjectStatus.URL.Host
		}, consts.Timeout, consts.Interval).Should(Equal(domainMappingName), "Should update Route Status of Capp")

		By("Updating the Capp Route hostname and checking the status")
		updatedRouteHostname := utils.GenerateResourceName(utils.GenerateRouteHostname(), consts.ZoneValue)
		err := retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
			toBeUpdatedCapp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			utils.ConfirmHostnameChange(toBeUpdatedCapp, updatedRouteHostname)

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

		By("checking if the domainMapping was updated")
		updatedDomainMappingObject := mocks.CreateDomainMappingObject(updatedRouteHostname)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, updatedDomainMappingObject)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Deleting the Capp instance")
		utils.DeleteCapp(k8sClient, createdCapp)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, createdCapp)
		}, consts.Timeout, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the domainMapping was deleted successfully")
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, updatedDomainMappingObject)
		}, consts.Timeout, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")
	})

	It("Should create DomainMapping with secret when Creating an HTTPS Capp instance", func() {
		By("Creating an HTTP Capp")
		createdCapp, routeHostname := utils.CreateCappWithHTTPHostname(k8sClient)

		By("Making sure the tls secret exists in advance")
		resourceName := utils.GenerateResourceName(routeHostname, consts.ZoneValue)
		secretName := utils.GenerateCertSecretName(resourceName)
		secretObject := mocks.CreateSecretObject(secretName)
		utils.CreateSecret(k8sClient, secretObject)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, secretObject)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Changing Capp to be HTTPS")
		err := retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
			assertionCapp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			assertionCapp.Spec.RouteSpec.TlsEnabled = true

			return utils.UpdateResource(k8sClient, assertionCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		By("Checking if the secret reference exists at the domainMapping")
		domainMappingName := utils.GenerateResourceName(routeHostname, consts.ZoneValue)
		Eventually(func() string {
			domainMapping := utils.GetDomainMapping(k8sClient, domainMappingName, createdCapp.Namespace)
			if domainMapping.Spec.TLS != nil {
				return domainMapping.Spec.TLS.SecretName
			}
			return ""
		}, consts.Timeout, consts.Interval).Should(Equal(secretName))
	})

	It("Should update the RouteStatus of the Capp accordingly", func() {
		By("Creating a capp with a route")
		createdCapp, routeHostname := utils.CreateCappWithHTTPHostname(k8sClient)

		domainMappingName := utils.GenerateResourceName(routeHostname, consts.ZoneValue)
		By("Checking if the RouteStatus of the Capp was updated successfully")
		Eventually(func() string {
			capp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			if capp.Status.RouteStatus.DomainMappingObjectStatus.URL == nil {
				return ""
			}
			return capp.Status.RouteStatus.DomainMappingObjectStatus.URL.Host
		}, consts.Timeout, consts.Interval).Should(Equal(domainMappingName), "Should update Route Status of Capp")

		By("Removing the Route from the Capp and check the status and resource clean up")
		err := retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
			toBeUpdatedCapp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			utils.ConfirmHostnameChange(toBeUpdatedCapp, "")

			return utils.UpdateResource(k8sClient, toBeUpdatedCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		domainMappingObject := mocks.CreateDomainMappingObject(domainMappingName)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, domainMappingObject)
		}, consts.Timeout, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		Eventually(func() cappv1alpha1.RouteStatus {
			capp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return capp.Status.RouteStatus
		}, consts.Timeout, consts.Interval).Should(Equal(cappv1alpha1.RouteStatus{}), "Should update Route Status of Capp")
	})
})
