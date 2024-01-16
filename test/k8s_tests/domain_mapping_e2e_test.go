package k8s_tests

import (
	mock "github.com/dana-team/container-app-operator/test/k8s_tests/mocks"
	utilst "github.com/dana-team/container-app-operator/test/k8s_tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
)

var _ = Describe("Validate DomainMapping functionality", func() {

	It("Should create, update and delete DomainMapping when Creating, updating and deleting a Capp instance", func() {
		By("Creating a capp with a route")
		routeCapp := mock.CreateBaseCapp()
		routeHostname := utilst.GenerateRouteHostname()
		routeCapp.Spec.RouteSpec.Hostname = routeHostname
		createdCapp := utilst.CreateCapp(k8sClient, routeCapp)

		By("Checking if the domainMapping was created successfully")
		domainMappingObject := mock.CreateDomainMappingObject(routeHostname)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, domainMappingObject)
		}, TimeoutCapp, CappCreationInterval).Should(BeTrue(), "Should find a resource.")

		By("Updating the capp route hostname")
		toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
		updatedRouteHostname := utilst.GenerateRouteHostname()
		toBeUpdatedCapp.Spec.RouteSpec.Hostname = updatedRouteHostname
		utilst.UpdateCapp(k8sClient, toBeUpdatedCapp)

		By("checking if the domainMapping was updated")
		updateDomainMappingObject := mock.CreateDomainMappingObject(updatedRouteHostname)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, updateDomainMappingObject)
		}, TimeoutCapp, CappCreationInterval).Should(BeTrue(), "Should find a resource.")

		By("Deleting the capp instance")
		utilst.DeleteCapp(k8sClient, createdCapp)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, createdCapp)
		}, TimeoutCapp, CappCreationInterval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the domainMapping was deleted successfully")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, updateDomainMappingObject)
		}, TimeoutCapp, CappCreationInterval).ShouldNot(BeTrue(), "Should not find a resource.")
	})

	It("Should create DomainMapping with secret when Creating an https Capp instance", func() {
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
		}, TimeoutCapp, CappCreationInterval).Should(Equal(secretName))
	})

	It("Should create DomainMapping without a secret reference when the secret doesn't exist", func() {
		By("Creating an https capp")
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
		}, TimeoutCapp, CappCreationInterval).Should(BeNil())
	})
})
