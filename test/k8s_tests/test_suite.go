package k8s_tests

import (
	"context"
	"fmt"
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	mock "github.com/dana-team/container-app-operator/test/k8s_tests/mocks"
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
	knativev1alphav1 "knative.dev/serving/pkg/apis/serving/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"time"
)

var (
	K8sClient client.Client
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = rcsv1alpha1.AddToScheme(s)
	_ = loggingv1beta1.AddToScheme(s)
	_ = knativev1alphav1.AddToScheme(s)
	_ = knativev1.AddToScheme(s)
	_ = networkingv1.AddToScheme(s)
	_ = routev1.AddToScheme(s)
	_ = scheme.AddToScheme(s)
	return s
}

var _ = SynchronizedBeforeSuite(func() {
	// Get the cluster configuration.
	// get the K8sClient or die
	config, err := config.GetConfig()
	if err != nil {
		Fail(fmt.Sprintf("Couldn't get kubeconfig %v", err))
	}

	// Create the client using the controller-runtime
	K8sClient, err = ctrl.New(config, ctrl.Options{Scheme: newScheme()})
	Expect(err).NotTo(HaveOccurred())

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: mock.NsName,
		},
	}

	Expect(K8sClient.Create(context.Background(), namespace)).To(Succeed())
}, func() {})

var _ = SynchronizedAfterSuite(func() {}, func() {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: mock.NsName,
		},
	}
	Expect(K8sClient.Delete(context.Background(), namespace)).To(Succeed())
	Eventually(func() error {
		return K8sClient.Get(context.Background(), client.ObjectKey{Name: mock.NsName}, namespace)
	}, time.Minute, 5*time.Second).Should(HaveOccurred(), "The namespace should be deleted")
})
