package e2e_tests

import (
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	mock "github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
)

var _ = Describe("Validate DomainMapping functionality", func() {
	It("Should create, update and delete DomainMapping when creating, updating and deleting a Capp instance", func() {
		By("Creating a capp with a route")
		routeCapp := mock.CreateBaseCapp()
		routeHostname := utilst.GenerateRouteHostname()
		routeCapp.Spec.RouteSpec.Hostname = routeHostname
		createdCapp := utilst.CreateCapp(k8sClient, routeCapp)

		By("Checking if the domainMapping was created successfully")
		domainMappingObject := mock.CreateDomainMappingObject(routeHostname)
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
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(routeHostname))

		By("Updating the Capp Route hostname and checking the status")
		toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
		updatedRouteHostname := utilst.GenerateRouteHostname()
		toBeUpdatedCapp.Spec.RouteSpec.Hostname = updatedRouteHostname
		utilst.UpdateCapp(k8sClient, toBeUpdatedCapp)

		Eventually(func() string {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			if capp.Status.RouteStatus.DomainMappingObjectStatus.URL == nil {
				return ""
			}
			return capp.Status.RouteStatus.DomainMappingObjectStatus.URL.Host
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(updatedRouteHostname))

		By("checking if the domainMapping was updated")
		updateDomainMappingObject := mock.CreateDomainMappingObject(updatedRouteHostname)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, updateDomainMappingObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Deleting the capp instance")
		utilst.DeleteCapp(k8sClient, createdCapp)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, createdCapp)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the domainMapping was deleted successfully")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, updateDomainMappingObject)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")
	})

	It("Should create DomainMapping with secret when Creating an HTTPS Capp instance", func() {
		By("Creating a secret")
		secretName := utilst.GenerateSecretName()
		secretObject := mock.CreateSecretObject(secretName)
		utilst.CreateSecret(k8sClient, secretObject)

		By("Creating an https capp")
		httpsCapp := mock.CreateBaseCapp()
		routeHostname := utilst.GenerateRouteHostname()
		httpsCapp.Spec.RouteSpec.Hostname = routeHostname
		httpsCapp.Spec.RouteSpec.TlsEnabled = true
		httpsCapp.Spec.RouteSpec.TlsSecret = secretName
		utilst.CreateCapp(k8sClient, httpsCapp)

		By("Checking if the secret reference exists at the domainMapping")
		Eventually(func() string {
			domainMapping := utilst.GetDomainMapping(k8sClient, routeHostname, httpsCapp.Namespace)
			return domainMapping.Spec.TLS.SecretName
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(secretName))
	})

	It("Should create DomainMapping without a secret reference when the secret doesn't exist", func() {
		By("Creating an HTTPS Capp")
		secretName := utilst.GenerateSecretName()
		httpsCapp := mock.CreateBaseCapp()
		routeHostname := utilst.GenerateRouteHostname()
		httpsCapp.Spec.RouteSpec.Hostname = routeHostname
		httpsCapp.Spec.RouteSpec.TlsEnabled = true
		httpsCapp.Spec.RouteSpec.TlsSecret = secretName
		utilst.CreateCapp(k8sClient, httpsCapp)

		By("Checking if the secret reference exists at the domainMapping")
		Eventually(func() *knativev1beta1.SecretTLS {
			domainMapping := utilst.GetDomainMapping(k8sClient, routeHostname, httpsCapp.Namespace)
			return domainMapping.Spec.TLS
		}, testconsts.Timeout, testconsts.Interval).Should(BeNil())
	})

	It("Should update the RouteStatus of the Capp accordingly", func() {
		By("Creating a Capp with a Route")
		routeCapp := mock.CreateBaseCapp()
		routeHostname := utilst.GenerateRouteHostname()
		routeCapp.Spec.RouteSpec.Hostname = routeHostname
		createdCapp := utilst.CreateCapp(k8sClient, routeCapp)

		By("Checking if the RouteStatus of the Capp was updated successfully")
		Eventually(func() string {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return capp.Status.RouteStatus.DomainMappingObjectStatus.URL.Host
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(routeHostname))

		By("Removing the Route from the Capp and check the status and resource clean up")
		toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
		toBeUpdatedCapp.Spec.RouteSpec = cappv1alpha1.RouteSpec{}
		utilst.UpdateCapp(k8sClient, toBeUpdatedCapp)

		domainMappingObject := mock.CreateDomainMappingObject(routeHostname)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, domainMappingObject)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		Eventually(func() cappv1alpha1.RouteStatus {
			capp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			return capp.Status.RouteStatus
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(cappv1alpha1.RouteStatus{}))
	})
})
