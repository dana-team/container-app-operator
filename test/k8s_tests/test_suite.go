package k8s_tests

import (
	"context"
	"fmt"
	"time"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	mock "github.com/dana-team/container-app-operator/test/k8s_tests/mocks"
	utilst "github.com/dana-team/container-app-operator/test/k8s_tests/utils"
	nfspvcv1alpha1 "github.com/dana-team/nfspvc-operator/api/v1alpha1"
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
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	k8sClient       client.Client
	targetAutoScale map[string]string
)

const (
	TimeoutNameSpace = 5 * time.Minute

	NsFetchInterval = 5 * time.Second
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = nfspvcv1alpha1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = cappv1alpha1.AddToScheme(s)
	_ = loggingv1beta1.AddToScheme(s)
	_ = knativev1beta1.AddToScheme(s)
	_ = knativev1.AddToScheme(s)
	_ = networkingv1.Install(s)
	_ = routev1.Install(s)
	_ = scheme.AddToScheme(s)
	return s
}

var _ = SynchronizedBeforeSuite(func() {
	// Get the cluster configuration.
	// get the k8sClient or die
	config, err := config.GetConfig()
	if err != nil {
		Fail(fmt.Sprintf("Couldn't get kubeconfig %v", err))
	}

	// Create the client using the controller-runtime
	k8sClient, err = ctrl.New(config, ctrl.Options{Scheme: newScheme()})
	Expect(err).NotTo(HaveOccurred())

	targetAutoScale = map[string]string{"rps": "22", "cpu": "88", "memory": "7", "concurrency": "11"}

	cleanUp()

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: mock.NsName,
		},
	}
	autoScaleConfigMap := mock.CreateConfigMapObject(mock.ControllerNS, mock.AutoScaleCM, targetAutoScale)
	utilst.CreateConfigMap(k8sClient, autoScaleConfigMap)

	Expect(k8sClient.Create(context.Background(), namespace)).To(Succeed())
	Eventually(func() bool {
		return utilst.DoesResourceExist(k8sClient, namespace)
	}, TimeoutNameSpace, NsFetchInterval).Should(BeTrue(), "The namespace should be created")
}, func() {})

var _ = SynchronizedAfterSuite(func() {}, func() {
	cleanUp()
})

// cleanUp make sure the test environment is clean
func cleanUp() {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: mock.NsName,
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
		}, TimeoutNameSpace, NsFetchInterval).Should(HaveOccurred(), "The autoscale configMap should be deleted")
	}
	if utilst.DoesResourceExist(k8sClient, namespace) {
		Expect(k8sClient.Delete(context.Background(), namespace)).To(Succeed())
		Eventually(func() error {
			return k8sClient.Get(context.Background(), client.ObjectKey{Name: mock.NsName}, namespace)
		}, TimeoutNameSpace, NsFetchInterval).Should(HaveOccurred(), "The namespace should be deleted")
	}
}
