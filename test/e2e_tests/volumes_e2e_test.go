package e2e_tests

import (
	"context"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	nfspvcName = "test-volume"
)

var _ = Describe("Validate NFSPVC functionality", func() {
	It("Should create, update and delete NFSPVC when creating, updating and deleting a Capp instance", func() {
		By("Creating a capp with a NFSPVC")
		testCapp := mocks.CreateBaseCapp()
		testCapp.Spec.VolumesSpec.NFSVolumes = []cappv1alpha1.NFSVolume{
			{
				Name:     nfspvcName,
				Server:   "nfs-server",
				Path:     "/path",
				Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		}
		testCapp.Spec.ConfigurationSpec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
			{
				Name:      nfspvcName,
				MountPath: "/mnt",
			},
		}

		cappName := utilst.GenerateCappName()
		testCapp.Name = cappName
		Expect(k8sClient.Create(context.Background(), testCapp)).To(Succeed())

		Eventually(func() string {
			capp := utilst.GetCapp(k8sClient, testCapp.Name, testCapp.Namespace)
			if len(capp.Spec.VolumesSpec.NFSVolumes) > 0 {
				return capp.Spec.VolumesSpec.NFSVolumes[0].Name
			}
			return ""
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(nfspvcName), "Should fetch capp with volume")

		By("Checking if the NFSPVC was created successfully")
		nfspvcObject := mocks.CreateNFSPVCObject(nfspvcName)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, nfspvcObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Checking the NFSPVC has the needed labels")
		nfspvcObject = utilst.GetNFSPVC(k8sClient, nfspvcName, mocks.NSName)
		Expect(nfspvcObject.Labels[testconsts.CappResourceKey]).Should(Equal(testCapp.Name))
		Expect(nfspvcObject.Labels[testconsts.ManagedByLabelKey]).Should(Equal(testconsts.CappKey))

		By("Deleting the Capp instance")
		utilst.DeleteCapp(k8sClient, testCapp)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, nfspvcObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeFalse(), "Should find a resource.")
	})
})
