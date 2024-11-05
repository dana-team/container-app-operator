package e2e_tests

import (
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"
	"k8s.io/client-go/util/retry"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Validate DomainMapping functionality", func() {
	It("Should create, update and delete DomainMapping when creating, updating and deleting a Capp instance", func() {
		By("Creating a capp with a route")
		createdCapp, routeHostname := utilst.CreateCappWithHTTPHostname(k8sClient)

		By("Checking if the domainMapping was created successfully")
		domainMappingName := utilst.GenerateResourceName(routeHostname, mocks.ZoneValue)
		domainMappingObject := mocks.CreateDomainMappingObject(domainMappingName)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, domainMappingObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Checking the domainMapping has the needed labels")
		domainMappingObject = utilst.GetDomainMapping(k8sClient, domainMappingName, mocks.NSName)
		Expect(domainMappingObject.Labels[testconsts.CappResourceKey]).Should(Equal(createdCapp.Name))
		Expect(domainMappingObject.Labels[testconsts.ManagedByLabelKey]).Should(Equal(testconsts.CappKey))

		By("Checking if the RouteStatus of the Capp was updated successfully")
		Eventually(func() string {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			if capp.Status.RouteStatus.DomainMappingObjectStatus.URL == nil {
				return ""
			}
			return capp.Status.RouteStatus.DomainMappingObjectStatus.URL.Host
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(domainMappingName), "Should update Route Status of Capp")

		By("Updating the Capp Route hostname and checking the status")
		updatedRouteHostname := utilst.GenerateResourceName(utilst.GenerateRouteHostname(), mocks.ZoneValue)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			toBeUpdatedCapp.Spec.RouteSpec.Hostname = updatedRouteHostname

			return utilst.UpdateResource(k8sClient, toBeUpdatedCapp)
		})
		Expect(err).To(BeNil())

		Eventually(func() string {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			if capp.Status.RouteStatus.DomainMappingObjectStatus.URL == nil {
				return ""
			}
			return capp.Status.RouteStatus.DomainMappingObjectStatus.URL.Host
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(updatedRouteHostname), "Should update Route Status of Capp")

		By("checking if the domainMapping was updated")
		updatedDomainMappingObject := mocks.CreateDomainMappingObject(updatedRouteHostname)
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
		resourceName := utilst.GenerateResourceName(routeHostname, mocks.ZoneValue)
		secretName := utilst.GenerateCertSecretName(resourceName)
		secretObject := mocks.CreateSecretObject(secretName)
		utilst.CreateSecret(k8sClient, secretObject)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, secretObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Changing Capp to be HTTPS")
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			assertionCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			assertionCapp.Spec.RouteSpec.TlsEnabled = true

			return utilst.UpdateResource(k8sClient, assertionCapp)
		})
		Expect(err).To(BeNil())

		By("Checking if the secret reference exists at the domainMapping")
		domainMappingName := utilst.GenerateResourceName(routeHostname, mocks.ZoneValue)
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

		domainMappingName := utilst.GenerateResourceName(routeHostname, mocks.ZoneValue)
		By("Checking if the RouteStatus of the Capp was updated successfully")
		Eventually(func() string {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return capp.Status.RouteStatus.DomainMappingObjectStatus.URL.Host
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(domainMappingName), "Should update Route Status of Capp")

		By("Removing the Route from the Capp and check the status and resource clean up")
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			toBeUpdatedCapp.Spec.RouteSpec = cappv1alpha1.RouteSpec{}

			return utilst.UpdateResource(k8sClient, toBeUpdatedCapp)
		})
		Expect(err).To(BeNil())

		domainMappingObject := mocks.CreateDomainMappingObject(domainMappingName)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, domainMappingObject)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		Eventually(func() cappv1alpha1.RouteStatus {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return capp.Status.RouteStatus
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(cappv1alpha1.RouteStatus{}), "Should update Route Status of Capp")
	})

	It("Should change the DomainMapping when the CappConfig is changed", func() {
		_, routeHostname := utilst.CreateCappWithHTTPHostname(k8sClient)

		By("Creating a DomainMapping instance")
		domainMappingName := utilst.GenerateResourceName(routeHostname, mocks.ZoneValue)
		domainMappingObject := mocks.CreateDomainMappingObject(domainMappingName)
		Eventually(func() bool {
			response := utilst.DoesResourceExist(k8sClient, domainMappingObject)
			return response
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Retrieving the CappConfig instance and updating it")
		zoneValue := "new" + mocks.ZoneValue
		newDNSConfig := cappv1alpha1.DNSConfig{
			Zone:     zoneValue,
			CNAME:    mocks.CNAMEValue,
			Provider: mocks.ProviderValue,
			Issuer:   mocks.IssuerValue,
		}
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			cappConfig := utilst.GetCappConfig(k8sClient, mocks.CappConfigName, mocks.ControllerNS)
			cappConfig.Spec.DNSConfig = newDNSConfig
			return utilst.UpdateResource(k8sClient, cappConfig)
		})
		Expect(err).To(BeNil())

		By("Verifying the change in the dnsConfig is reflected in the DomainMapping")
		updatedDomainMappingName := utilst.GenerateResourceName(routeHostname, zoneValue)
		Eventually(func() bool {
			updatedDomainMapping := utilst.GetDomainMapping(k8sClient, updatedDomainMappingName, mocks.NSName)
			return updatedDomainMapping.ObjectMeta.Name == updatedDomainMappingName
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "The domain mapping should reflect the updated dnsConfig data")
	})
})
