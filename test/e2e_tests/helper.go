package e2e_tests

import (
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	nfspvcv1alpha1 "github.com/dana-team/nfspvc-operator/api/v1alpha1"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	dnsv1alpha1 "sigs.k8s.io/external-dns/endpoint"
)

const externalDNSGroupVersion = "externaldns.k8s.io/v1alpha1"

var (
	k8sClient       client.Client
	targetAutoScale map[string]string
	scheme          = runtime.NewScheme()
)

func newScheme() *runtime.Scheme {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(knativev1.AddToScheme(scheme))
	utilruntime.Must(loggingv1beta1.AddToScheme(scheme))
	utilruntime.Must(knativev1beta1.AddToScheme(scheme))
	utilruntime.Must(cappv1alpha1.AddToScheme(scheme))
	utilruntime.Must(nfspvcv1alpha1.AddToScheme(scheme))
	initExternalDNSSchemes()

	return scheme
}

// initExternalDNSSchemes is needed because the ExternalDNS operator does not
// provide an AddToScheme method that can be used.
func initExternalDNSSchemes() {
	groupVersion, _ := schema.ParseGroupVersion(externalDNSGroupVersion)
	scheme.AddKnownTypes(groupVersion, &dnsv1alpha1.DNSEndpoint{}, &dnsv1alpha1.DNSEndpointList{})
	metav1.AddToGroupVersion(scheme, groupVersion)
}
