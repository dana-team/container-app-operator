package resourceclient

import (
	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	nfspvcv1alpha1 "github.com/dana-team/nfspvc-operator/api/v1alpha1"
	dnsrecordv1alpha1 "github.com/dana-team/provider-dns-v2/apis/namespaced/record/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
)

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
func GetBareCertificate(name, namespace string) cmapi.Certificate {
	return cmapi.Certificate{
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

// GetBareDNSRecord returns a DNSRecord object with only ObjectMeta set.
func GetBareDNSRecord(name, namespace string) dnsrecordv1alpha1.CNAMERecord {
	return dnsrecordv1alpha1.CNAMERecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}
