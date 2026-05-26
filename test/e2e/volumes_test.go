package e2e

import (
	"context"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/test/e2e/consts"
	"github.com/dana-team/container-app-operator/test/e2e/mocks"
	"github.com/dana-team/container-app-operator/test/e2e/utils"
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

		cappName := utils.GenerateCappName()
		testCapp.Name = cappName
		Expect(k8sClient.Create(context.Background(), testCapp)).To(Succeed())

		Eventually(func() string {
			capp := utils.GetCapp(k8sClient, testCapp.Name, testCapp.Namespace)
			if len(capp.Spec.VolumesSpec.NFSVolumes) > 0 {
				return capp.Spec.VolumesSpec.NFSVolumes[0].Name
			}
			return ""
		}, consts.Timeout, consts.Interval).Should(Equal(nfspvcName), "Should fetch capp with volume")

		By("Checking if the NFSPVC was created successfully")
		nfspvcObject := mocks.CreateNFSPVCObject(nfspvcName)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, nfspvcObject)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Checking the NFSPVC has the needed labels")
		nfspvcObject = utils.GetNFSPVC(k8sClient, nfspvcName, consts.NSName)
		Expect(nfspvcObject.Labels[consts.CappResourceKey]).Should(Equal(testCapp.Name))
		Expect(nfspvcObject.Labels[consts.ManagedByLabelKey]).Should(Equal(consts.CappKey))

		By("Deleting the Capp instance")
		utils.DeleteCapp(k8sClient, testCapp)
		Eventually(func() bool {
			return utils.DoesResourceExist(k8sClient, nfspvcObject)
		}, consts.Timeout, consts.Interval).Should(BeFalse(), "Should find a resource.")
	})
})
