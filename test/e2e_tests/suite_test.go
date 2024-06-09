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

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)

	SetDefaultEventuallyTimeout(testconsts.DefaultEventually)
	RunSpecs(t, "Capp Suite")
}

var _ = SynchronizedBeforeSuite(func() {
	initClient()
	cleanUp()
	createE2ETestNamespace()
	initE2ETestAutoScaleConfigMap()
	createE2ETestAutoScaleConfigMap()
	createE2ETestZoneConfigMap()
}, func() {
	initClient()
	initE2ETestAutoScaleConfigMap()
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

// initE2ETestAutoScaleConfigMap initializes values in the
func initE2ETestAutoScaleConfigMap() {
	rps := "22"
	cpu := "88"
	memory := "7"
	concurrency := "11"

	targetAutoScale = map[string]string{"rps": rps, "cpu": cpu, "memory": memory, "concurrency": concurrency}
}

// createE2ETestAutoScaleConfigMap creates an Autoscale ConfigMap for the e2e tests.
func createE2ETestAutoScaleConfigMap() {
	autoScaleConfigMap := mock.CreateConfigMapObject(mock.ControllerNS, mock.AutoScaleCM, targetAutoScale)
	utilst.CreateConfigMap(k8sClient, autoScaleConfigMap)
}

// createE2ETestZoneConfigMap creates a Zone ConfigMap for the e2e tests.
func createE2ETestZoneConfigMap() {
	zone := map[string]string{
		mock.ZoneKey: mock.ZoneValue,
	}

	zoneConfigMap := mock.CreateConfigMapObject(mock.ControllerNS, mock.ZoneCM, zone)
	utilst.CreateConfigMap(k8sClient, zoneConfigMap)
}

// cleanUp make sure the test environment is clean.
func cleanUp() {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: mock.NSName,
		},
	}

	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name:      mock.AutoScaleCM,
		Namespace: mock.ControllerNS,
	}}

	if utilst.DoesResourceExist(k8sClient, configMap) {
		Expect(k8sClient.Delete(context.Background(), configMap)).To(Succeed())
		Eventually(func() error {
			return k8sClient.Get(context.Background(), client.ObjectKey{Name: mock.AutoScaleCM, Namespace: mock.ControllerNS}, configMap)
		}, testconsts.Timeout, testconsts.Interval).Should(HaveOccurred(), "The autoscale configMap should be deleted")
	}

	configMap = &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name:      mock.ZoneCM,
		Namespace: mock.ControllerNS,
	}}

	if utilst.DoesResourceExist(k8sClient, configMap) {
		Expect(k8sClient.Delete(context.Background(), configMap)).To(Succeed())
		Eventually(func() error {
			return k8sClient.Get(context.Background(), client.ObjectKey{Name: mock.ZoneCM, Namespace: mock.ControllerNS}, configMap)
		}, testconsts.Timeout, testconsts.Interval).Should(HaveOccurred(), "The autoscale configMap should be deleted")
	}

	if utilst.DoesResourceExist(k8sClient, namespace) {
		Expect(k8sClient.Delete(context.Background(), namespace)).To(Succeed())
		Eventually(func() error {
			return k8sClient.Get(context.Background(), client.ObjectKey{Name: mock.NSName}, namespace)
		}, testconsts.Timeout, testconsts.Interval).Should(HaveOccurred(), "The namespace should be deleted")
	}
}
