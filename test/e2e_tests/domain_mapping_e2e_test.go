package e2e_tests

import (
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	mock "github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Validate DomainMapping functionality", func() {
	It("Should create, update and delete DomainMapping when creating, updating and deleting a Capp instance", func() {
		By("Creating a capp with a route")
		createdCapp, routeHostname := utilst.CreateCappWithHTTPHostname(k8sClient)

		By("Checking if the domainMapping was created successfully")
		domainMappingName := utilst.GenerateResourceName(routeHostname, mock.ZoneValue)
		domainMappingObject := mock.CreateDomainMappingObject(domainMappingName)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, domainMappingObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Checking if the RouteStatus of the Capp was updated successfully")
		Eventually(func() string {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			if capp.Status.RouteStatus.DomainMappingObjectStatus.URL == nil {
				return ""
			}
			return capp.Status.RouteStatus.DomainMappingObjectStatus.URL.Host
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(domainMappingName), "Should update Route Status of Capp")

		By("Updating the Capp Route hostname and checking the status")
		toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
		updatedRouteHostname := utilst.GenerateResourceName(utilst.GenerateRouteHostname(), mock.ZoneValue)
		toBeUpdatedCapp.Spec.RouteSpec.Hostname = updatedRouteHostname
		utilst.UpdateCapp(k8sClient, toBeUpdatedCapp)

		Eventually(func() string {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			if capp.Status.RouteStatus.DomainMappingObjectStatus.URL == nil {
				return ""
			}
			return capp.Status.RouteStatus.DomainMappingObjectStatus.URL.Host
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(updatedRouteHostname), "Should update Route Status of Capp")

		By("checking if the domainMapping was updated")
		updatedDomainMappingObject := mock.CreateDomainMappingObject(updatedRouteHostname)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, updatedDomainMappingObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Deleting the Capp instance")
		utilst.DeleteCapp(k8sClient, createdCapp)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, createdCapp)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the domainMapping was deleted successfully")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, updatedDomainMappingObject)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")
	})

	It("Should create DomainMapping with secret when Creating an HTTPS Capp instance", func() {
		By("Creating an HTTP Capp")
		createdCapp, routeHostname := utilst.CreateCappWithHTTPHostname(k8sClient)

		By("Making sure the tls secret exists in advance")
		secretName := utilst.GenerateCertSecretName(createdCapp.Name)
		secretObject := mock.CreateSecretObject(secretName)
		utilst.CreateSecret(k8sClient, secretObject)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, secretObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Changing Capp to be HTTPS")
		assertionCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
		assertionCapp.Spec.RouteSpec.TlsEnabled = true
		utilst.UpdateCapp(k8sClient, assertionCapp)

		By("Checking if the secret reference exists at the domainMapping")
		domainMappingName := utilst.GenerateResourceName(routeHostname, mock.ZoneValue)
		Eventually(func() string {
			domainMapping := utilst.GetDomainMapping(k8sClient, domainMappingName, createdCapp.Namespace)
			if domainMapping.Spec.TLS != nil {
				return domainMapping.Spec.TLS.SecretName
			}
			return ""
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(secretName))
	})

	It("Should update the RouteStatus of the Capp accordingly", func() {
		By("Creating a capp with a route")
		createdCapp, routeHostname := utilst.CreateCappWithHTTPHostname(k8sClient)

		domainMappingName := utilst.GenerateResourceName(routeHostname, mock.ZoneValue)
		By("Checking if the RouteStatus of the Capp was updated successfully")
		Eventually(func() string {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return capp.Status.RouteStatus.DomainMappingObjectStatus.URL.Host
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(domainMappingName), "Should update Route Status of Capp")

		By("Removing the Route from the Capp and check the status and resource clean up")
		toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
		toBeUpdatedCapp.Spec.RouteSpec = cappv1alpha1.RouteSpec{}
		utilst.UpdateCapp(k8sClient, toBeUpdatedCapp)

		domainMappingObject := mock.CreateDomainMappingObject(domainMappingName)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, domainMappingObject)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		Eventually(func() cappv1alpha1.RouteStatus {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return capp.Status.RouteStatus
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(cappv1alpha1.RouteStatus{}), "Should update Route Status of Capp")
	})
})
