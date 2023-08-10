package secure

import (
	"fmt"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internals/wrappers"
	knativev1alphav1 "knative.dev/serving/pkg/apis/serving/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// SetHttpsKnativeDomainMapping takes a Capp, Knative Domain Mapping and a ResourceBaseManager Client and sets the Knative Domain Mapping Tls based on the Capp's Https field.
func SetHttpsKnativeDomainMapping(capp rcsv1alpha1.Capp, knativeDomainMapping *knativev1alphav1.DomainMapping, resourceManager rclient.ResourceBaseManager) {
	isHttps := capp.Spec.RouteSpec.TlsEnabled
	if isHttps {
		tlsSecret := corev1.Secret{}
		if err := resourceManager.K8sclient.Get(resourceManager.Ctx, types.NamespacedName{Name: capp.Spec.RouteSpec.TlsSecret, Namespace: capp.Namespace}, &tlsSecret); err != nil {
			if errors.IsNotFound(err) {
				resourceManager.Log.Error(err, fmt.Sprintf("the tls secret %s for DomainMapping does not exist", capp.Spec.RouteSpec.TlsSecret))
				return
			}
			resourceManager.Log.Error(err, fmt.Sprintf("unable to get tls secret %s for DomainMapping", capp.Spec.RouteSpec.TlsSecret))
		} else {
			knativeDomainMapping.Spec.TLS = &knativev1alphav1.SecretTLS{
				SecretName: capp.Spec.RouteSpec.TlsSecret,
			}
		}
	}
}
