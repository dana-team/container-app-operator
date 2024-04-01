package utils

import (
	nfspvcv1alpha1 "github.com/dana-team/nfspvc-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetNfspvc retrieves existing instance of Nfspvc and returns it.
func GetNfspvc(k8sClient client.Client, name string, namespace string) *nfspvcv1alpha1.NfsPvc {
	nfspvc := &nfspvcv1alpha1.NfsPvc{}
	GetResource(k8sClient, nfspvc, name, namespace)
	return nfspvc
}
