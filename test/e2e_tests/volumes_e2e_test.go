package e2e_tests

import (
	"context"
	"time"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	mock "github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	NfsPvcName          = "test-volume"
	TimeoutNfs          = 180 * time.Second
	NfsCreationInterval = 5 * time.Second
)

var _ = Describe("Validate NFSPVC functionality", func() {
	It("Should create, update and delete NFSPVC when creating, updating and deleting a Capp instance", func() {
		By("Creating a capp with a NFSPVC")

		NFSPVCCapp := mock.CreateBaseCapp()
		NFSPVCCapp.Spec.VolumesSpec.NFSVolumes = []cappv1alpha1.NFSVolume{
			{
				Name:     NfsPvcName,
				Server:   "nfs-server",
				Path:     "/path",
				Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		}
		NFSPVCCapp.Spec.ConfigurationSpec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
			{
				Name:      NfsPvcName,
				MountPath: "/mnt",
			},
		}
		Expect(k8sClient.Create(context.Background(), NFSPVCCapp)).To(Succeed())
		Eventually(func() string {
			return utilst.GetCapp(k8sClient, NFSPVCCapp.Name, NFSPVCCapp.Namespace).Spec.VolumesSpec.NFSVolumes[0].Name
		}, TimeoutNfs, NfsCreationInterval).Should(Equal(NfsPvcName), "Should fetch capp with volume")

		By("Checking if the NFSPVC was created successfully")
		NFSPVCObject := mock.CreateNFSPVCObject(NfsPvcName)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, NFSPVCObject)
		}, TimeoutNfs, NfsCreationInterval).Should(BeTrue(), "Should find a resource.")

		By("Deleting the Capp instance")
		utilst.DeleteCapp(k8sClient, NFSPVCCapp)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, NFSPVCObject)
		}, TimeoutNfs, NfsCreationInterval).Should(BeFalse(), "Should find a resource.")
	})
})
