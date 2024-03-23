package e2e_tests

import (
	"context"
	"testing"

	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	mock "github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	networkingv1 "github.com/openshift/api/network/v1"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	k8sClient       client.Client
	targetAutoScale map[string]string
)

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)

	SetDefaultEventuallyTimeout(testconsts.DefaultEventually)
	RunSpecs(t, "Capp Suite")
}

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = rcsv1alpha1.AddToScheme(s)
	_ = loggingv1beta1.AddToScheme(s)
	_ = knativev1beta1.AddToScheme(s)
	_ = knativev1.AddToScheme(s)
	_ = networkingv1.Install(s)
	_ = routev1.Install(s)
	_ = scheme.AddToScheme(s)
	return s
}

var _ = SynchronizedBeforeSuite(func() {
	initClient()
	cleanUp()
	createE2ETestNamespace()
	initE2ETestAutoScaleConfigMap()
	createE2ETestAutoScaleConfigMap()
}, func() {
	initClient()
	initE2ETestAutoScaleConfigMap()
})

var _ = SynchronizedAfterSuite(func() {}, func() {
	cleanUp()
})

// initClient initializes a k8s client.
func initClient() {
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

	Expect(k8sClient.Create(context.Background(), namespace)).To(Succeed())
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

// createE2ETestAutoScaleConfigMap creates an Auto Scale ConfigMap for the e2e tests.
func createE2ETestAutoScaleConfigMap() {
	autoScaleConfigMap := mock.CreateConfigMapObject(mock.ControllerNS, mock.AutoScaleCM, targetAutoScale)
	utilst.CreateConfigMap(k8sClient, autoScaleConfigMap)
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
	if utilst.DoesResourceExist(k8sClient, namespace) {
		Expect(k8sClient.Delete(context.Background(), namespace)).To(Succeed())
		Eventually(func() error {
			return k8sClient.Get(context.Background(), client.ObjectKey{Name: mock.NSName}, namespace)
		}, testconsts.Timeout, testconsts.Interval).Should(HaveOccurred(), "The namespace should be deleted")
	}
}
