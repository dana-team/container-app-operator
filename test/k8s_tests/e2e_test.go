package k8s_tests

import (
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	mock "github.com/dana-team/container-app-operator/test/k8s_tests/mocks"
	utilst "github.com/dana-team/container-app-operator/test/k8s_tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	knativev1alphav1 "knative.dev/serving/pkg/apis/serving/v1alpha1"
	"testing"
	"time"
)

const (
	TimeoutCapp          = 60 * time.Second
	CappCreationInterval = 2 * time.Second
	CappNamespace        = "capp-e2e-tests"
)

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)

	SetDefaultEventuallyTimeout(time.Second * 2)
	RunSpecs(t, "Capp Suite")
}

var _ = Describe("Validate Capp adapter", func() {

	It("Should succeed all adapter functions", func() {
		baseCapp := mock.CreateBaseCapp()
		desiredCapp := utilst.CreateCapp(k8sClient, baseCapp)
		assertionCapp := &rcsv1alpha1.Capp{}

		By("Checks unique creation of Capp")
		assertionCapp = utilst.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace)
		Expect(assertionCapp.Name).ShouldNot(Equal(baseCapp.Name))

		By("Checks if Capp Updated successfully")
		desiredCapp = assertionCapp.DeepCopy()
		desiredCapp.Spec.ScaleMetric = mock.RPSScaleMetric
		utilst.UpdateCapp(k8sClient, desiredCapp)
		Eventually(func() string {
			assertionCapp = utilst.GetCapp(k8sClient, assertionCapp.Name, assertionCapp.Namespace)
			return assertionCapp.Spec.ScaleMetric
		}, TimeoutCapp, CappCreationInterval).Should(Equal(mock.RPSScaleMetric), "Should fetch capp.")

		By("Checks if deleted successfully")
		utilst.DeleteCapp(k8sClient, assertionCapp)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, assertionCapp)
		}, TimeoutCapp, CappCreationInterval).ShouldNot(BeTrue(), "Should not find a resource.")
	})
})

var _ = Describe("Validate DomainMapping functionality", func() {

	It("Should create DomainMapping when Creating a Capp instance", func() {
		By("Creating a capp with a route")
		routeCapp := mock.CreateBaseCapp()
		routeHostname := utilst.GenerateRouteHostname()
		routeCapp.Spec.RouteSpec.Hostname = routeHostname
		utilst.CreateCapp(k8sClient, routeCapp)
		By("Checking if the domainMapping was created successfully")
		domainMapping := &knativev1alphav1.DomainMapping{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeHostname,
				Namespace: routeCapp.Namespace,
			},
		}
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, domainMapping)
		}, TimeoutCapp, CappCreationInterval).Should(BeTrue(), "Should find a resource.")
	})

	It("Should create DomainMapping with secret when Creating an https Capp instance", func() {
		By("Creating a secret")
		secretName := utilst.GenerateSecretName()
		secret := v1.Secret{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: CappNamespace,
			},
			Type: "Opaque",
			Data: map[string][]byte{"extra": []byte("YmFyCg==")},
		}
		utilst.CreateSecret(k8sClient, secret)
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

	It("Should delete DomainMapping when deleting a Capp instance", func() {
		By("Creating a capp with a route")
		routeCapp := mock.CreateBaseCapp()
		routeHostname := utilst.GenerateRouteHostname()
		routeCapp.Spec.RouteSpec.Hostname = routeHostname
		createdCapp := utilst.CreateCapp(k8sClient, routeCapp)
		By("Checking if the domainMapping was created successfully")
		domainMapping := &knativev1alphav1.DomainMapping{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeHostname,
				Namespace: routeCapp.Namespace,
			},
		}
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, domainMapping)
		}, TimeoutCapp, CappCreationInterval).Should(BeTrue(), "Should find a resource.")
		CreatedDomainMapping := utilst.GetDomainMapping(k8sClient, routeHostname, routeCapp.Namespace)
		By("Deleting the capp instance")
		utilst.DeleteCapp(k8sClient, createdCapp)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, createdCapp)
		}, TimeoutCapp, CappCreationInterval).ShouldNot(BeTrue(), "Should not find a resource.")
		By("Checking if the domainMapping was deleted successfully")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, CreatedDomainMapping)
		}, TimeoutCapp, CappCreationInterval).ShouldNot(BeTrue(), "Should not find a resource.")
	})

	It("Should update DomainMapping when updating route hostname", func() {
		By("Creating a capp with a route")
		routeCapp := mock.CreateBaseCapp()
		routeHostname := utilst.GenerateRouteHostname()
		routeCapp.Spec.RouteSpec.Hostname = routeHostname
		createdCapp := utilst.CreateCapp(k8sClient, routeCapp)
		By("Updating the capp route hostname")
		toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
		updatedRouteHostname := utilst.GenerateRouteHostname()
		toBeUpdatedCapp.Spec.RouteSpec.Hostname = updatedRouteHostname
		utilst.UpdateCapp(k8sClient, toBeUpdatedCapp)
		By("checking if the domainMapping was updated")
		domainMapping := &knativev1alphav1.DomainMapping{
			ObjectMeta: metav1.ObjectMeta{
				Name:      updatedRouteHostname,
				Namespace: toBeUpdatedCapp.Namespace,
			},
		}
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, domainMapping)
		}, TimeoutCapp, CappCreationInterval).Should(BeTrue(), "Should find a resource.")
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
		Eventually(func() *knativev1alphav1.SecretTLS {
			domainMapping := utilst.GetDomainMapping(k8sClient, routeHostname, httpsCapp.Namespace)
			return domainMapping.Spec.TLS
		}, TimeoutCapp, CappCreationInterval).Should(BeNil())
	})
})
