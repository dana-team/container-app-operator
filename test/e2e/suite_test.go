package e2e

import (
	"context"
	"os"
	"testing"

	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/dana-team/container-app-operator/test/e2e/consts"

	"github.com/dana-team/container-app-operator/test/e2e/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)

	SetDefaultEventuallyTimeout(consts.DefaultEventually)
	RunSpecs(t, "Capp Suite")
}

var _ = SynchronizedBeforeSuite(func() {
	initClient()
	cleanUp()
	createE2ETestNamespace()
	utils.CreateTestUser(k8sClient, consts.NSName)
	utils.CreateExcludedServiceAccount(k8sClient)
}, func() {
	initClient()
})

var _ = SynchronizedAfterSuite(func() {}, func() {
	utils.DeleteTestUser(k8sClient, consts.NSName)
	utils.DeleteExcludedServiceAccount(k8sClient)

	if os.Getenv("E2E_SKIP_CLEANUP") == "true" {
		return
	}

	cleanUp()
})

// initClient initializes a k8s client.
func initClient() {
	log.SetLogger(logger)

	var err error
	cfg, err = config.GetConfig()
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: newScheme()})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
}

// createE2ETestNamespace creates a namespace for the e2e tests.
func createE2ETestNamespace() {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: consts.NSName,
		},
	}

	Expect(k8sClient.Create(context.Background(), namespace)).To(SatisfyAny(BeNil(), WithTransform(errors.IsAlreadyExists, BeTrue())))
	Eventually(func() bool {
		return utils.DoesResourceExist(k8sClient, namespace)
	}, consts.Timeout, consts.Interval).Should(BeTrue(), "The namespace should be created")
}

// cleanUp make sure the test environment is clean.
func cleanUp() {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: consts.NSName,
		},
	}

	if utils.DoesResourceExist(k8sClient, namespace) {
		Expect(k8sClient.Delete(context.Background(), namespace)).To(Succeed())
		Eventually(func() error {
			return k8sClient.Get(context.Background(), client.ObjectKey{Name: consts.NSName}, namespace)
		}, consts.Timeout, consts.Interval).Should(HaveOccurred(), "The namespace should be deleted")
	}
}
