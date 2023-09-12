package k8s_tests

import (
	"context"
	"fmt"
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
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
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	k8sClient client.Client
	nsName    = "capp-e2e-tests"
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

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)

	SetDefaultEventuallyTimeout(time.Second * 2)
	RunSpecs(t, "Capp Suite")
}

var _ = BeforeSuite(func() {
	// Get the cluster configuration.
	// get the k8sClient or die
	config, err := config.GetConfig()
	if err != nil {
		Fail(fmt.Sprintf("Couldn't get kubeconfig %v", err))
	}
	// Create the client using the controller-runtime
	k8sClient, err = ctrl.New(config, ctrl.Options{Scheme: newScheme()})
	Expect(err).NotTo(HaveOccurred())

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	}

	Expect(k8sClient.Create(context.Background(), namespace)).To(Succeed())
})

var _ = AfterSuite(func() {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	}
	Expect(k8sClient.Delete(context.Background(), namespace)).To(Succeed())
	Eventually(func() error {
		return k8sClient.Get(context.Background(), client.ObjectKey{Name: nsName}, namespace)
	}, time.Minute, 5*time.Second).Should(HaveOccurred(), "The namespace should be deleted")
})

var _ = Describe("Validate Suite acted correctly ", func() {

	It("should have created a namespace", func() {
		ns := &corev1.Namespace{}
		err := k8sClient.Get(context.TODO(), client.ObjectKey{Name: nsName}, ns)
		Expect(err).NotTo(HaveOccurred())
		Expect(ns).NotTo(BeNil())
	})
})
