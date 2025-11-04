package mocks

import (
	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"
	nfspvcv1alpha1 "github.com/dana-team/nfspvc-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateNFSPVCObject returns an NFSPVC object.
func CreateNFSPVCObject(name string) *nfspvcv1alpha1.NfsPvc {
	return &nfspvcv1alpha1.NfsPvc{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testconsts.NSName,
		},
		Spec: nfspvcv1alpha1.NfsPvcSpec{
			Server:   testconsts.Server,
			Path:     testconsts.Path,
			Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(testconsts.Capacity)},
		},
	}
}
