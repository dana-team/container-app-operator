package resourceclient

import (
	certv1alpha1 "github.com/dana-team/certificate-operator/api/v1alpha1"
	nfspvcv1alpha1 "github.com/dana-team/nfspvc-operator/api/v1alpha1"
	dnsvrecord1alpha1 "github.com/dana-team/provider-dns/apis/record/v1alpha1"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
)

// GetBareKSVC returns a KSVC object with only ObjectMeta set.
func GetBareKSVC(name, namespace string) knativev1.Service {
	return knativev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// GetBareDomainMapping returns a DomainMapping object with only ObjectMeta set.
func GetBareDomainMapping(name, namespace string) knativev1beta1.DomainMapping {
	return knativev1beta1.DomainMapping{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// GetBareCertificate returns a Certificate object with only ObjectMeta set.
func GetBareCertificate(name, namespace string) certv1alpha1.Certificate {
	return certv1alpha1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// GetBareNFSPVC returns a NFSPVC object with only ObjectMeta set.
func GetBareNFSPVC(name, namespace string) nfspvcv1alpha1.NfsPvc {
	return nfspvcv1alpha1.NfsPvc{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// GetBareSyslogNGFlow returns a SyslogNGFlow object with only ObjectMeta set.
func GetBareSyslogNGFlow(name, namespace string) loggingv1beta1.SyslogNGFlow {
	return loggingv1beta1.SyslogNGFlow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// GetBareSyslogNGOutput returns a SyslogNGOutput object with only ObjectMeta set.
func GetBareSyslogNGOutput(name, namespace string) loggingv1beta1.SyslogNGOutput {
	return loggingv1beta1.SyslogNGOutput{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// GetBareDNSRecord returns a DNSRecord object with only ObjectMeta set.
func GetBareDNSRecord(name string) dnsvrecord1alpha1.CNAMERecord {
	return dnsvrecord1alpha1.CNAMERecord{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}
