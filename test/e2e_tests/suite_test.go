package e2e_tests

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"

	mock "github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"
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

	SetDefaultEventuallyTimeout(testconsts.DefaultEventually)
	RunSpecs(t, "Capp Suite")
}

var _ = SynchronizedBeforeSuite(func() {
	initClient()
	cleanUp()
	createE2ETestNamespace()
}, func() {
	initClient()
})

var _ = SynchronizedAfterSuite(func() {}, func() {
	cleanUp()
})

// initClient initializes a k8s client.
func initClient() {
	log.SetLogger(logger)

	cfg, err := config.GetConfig()
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: newScheme()})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
}

// createE2ETestNamespace creates a namespace for the e2e tests.
func createE2ETestNamespace() {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: mock.NSName,
		},
	}

	Expect(k8sClient.Create(context.Background(), namespace)).To(SatisfyAny(BeNil(), WithTransform(errors.IsAlreadyExists, BeTrue())))
	Eventually(func() bool {
		return utilst.DoesResourceExist(k8sClient, namespace)
	}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "The namespace should be created")
}

// cleanUp make sure the test environment is clean.
func cleanUp() {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: mock.NSName,
		},
	}

	if utilst.DoesResourceExist(k8sClient, namespace) {
		Expect(k8sClient.Delete(context.Background(), namespace)).To(Succeed())
		Eventually(func() error {
			return k8sClient.Get(context.Background(), client.ObjectKey{Name: mock.NSName}, namespace)
		}, testconsts.Timeout, testconsts.Interval).Should(HaveOccurred(), "The namespace should be deleted")
	}
}
