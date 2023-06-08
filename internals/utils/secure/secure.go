package secure

import (
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internals/wrappers"
	knativev1alphav1 "knative.dev/serving/pkg/apis/serving/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

const TlsSecretPrefix = "secure-knativedm-"

// SetHttpsKnativeDomainMapping takes a Capp, Knative Domain Mapping and a ResourceBaseManager Client and sets the Knative Domain Mapping Tls based on the Capp's Https field.
func SetHttpsKnativeDomainMapping(capp rcsv1alpha1.Capp, knativeDomainMapping *knativev1alphav1.DomainMapping, resourceManager rclient.ResourceBaseManager) {
	isHttps := capp.Spec.RouteSpec.Https
	if isHttps {
		tlsSecret := corev1.Secret{}
		if err := resourceManager.K8sclient.Get(resourceManager.Ctx, types.NamespacedName{Name: TlsSecretPrefix + capp.Name, Namespace: capp.Namespace}, &tlsSecret); err != nil {
			if !errors.IsNotFound(err) {
				resourceManager.Log.Error(err, "unable to get resource")
			}
		} else {
			knativeDomainMapping.Spec.TLS = &knativev1alphav1.SecretTLS{
				SecretName: TlsSecretPrefix + capp.Name,
			}
		}
	}
}
