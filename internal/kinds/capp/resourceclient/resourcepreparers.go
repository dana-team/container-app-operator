package resourceclient

import (
	nfspvcv1alpha1 "github.com/dana-team/nfspvc-operator/api/v1alpha1"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	dnsv1alpha1 "sigs.k8s.io/external-dns/endpoint"
)

// PrepareKSVC returns a KSVC object.
func PrepareKSVC(name, namespace string) knativev1.Service {
	return knativev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// PrepareDomainMapping returns a DomainMapping object.
func PrepareDomainMapping(name, namespace string) knativev1beta1.DomainMapping {
	return knativev1beta1.DomainMapping{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// PrepareNFSPVC returns a NFSPVC object.
func PrepareNFSPVC(name, namespace string) nfspvcv1alpha1.NfsPvc {
	return nfspvcv1alpha1.NfsPvc{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// PrepareSyslogNGFlow returns a SyslogNGFlow object.
func PrepareSyslogNGFlow(name, namespace string) loggingv1beta1.SyslogNGFlow {
	return loggingv1beta1.SyslogNGFlow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// PrepareSyslogNGOutput returns a SyslogNGOutput object.
func PrepareSyslogNGOutput(name, namespace string) loggingv1beta1.SyslogNGOutput {
	return loggingv1beta1.SyslogNGOutput{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// PrepareDNSEndpoint returns a DNSEndpoint object.
func PrepareDNSEndpoint(name, namespace string) dnsv1alpha1.DNSEndpoint {
	return dnsv1alpha1.DNSEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}
