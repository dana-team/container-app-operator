package utils

import (
	nfspvcv1alpha1 "github.com/dana-team/nfspvc-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetNFSPVC fetches and returns an existing instance of a NFSPVC.
func GetNFSPVC(k8sClient client.Client, name string, namespace string) *nfspvcv1alpha1.NfsPvc {
	revision := &nfspvcv1alpha1.NfsPvc{}
	GetResource(k8sClient, revision, name, namespace)
	return revision
}
