package mocks

import (
	nfspvcv1alpha1 "github.com/dana-team/nfspvc-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	Server   = "nfs-server"
	Path     = "/nfs-path"
	Capacity = "1Gi"
)

// CreateNFSPVCObject returns an NFSPVC object.
func CreateNFSPVCObject(name string) *nfspvcv1alpha1.NfsPvc {
	return &nfspvcv1alpha1.NfsPvc{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: NSName,
		},
		Spec: nfspvcv1alpha1.NfsPvcSpec{
			Server:   Server,
			Path:     Path,
			Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(Capacity)},
		},
	}
}
