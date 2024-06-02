package mocks

import (
	dnsv1alpha1 "github.com/dana-team/provider-dns/apis/recordset/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateARecordSetObject returns an empty ARecordSet object.
func CreateARecordSetObject(name string) *dnsv1alpha1.ARecordSet {
	return &dnsv1alpha1.ARecordSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}
