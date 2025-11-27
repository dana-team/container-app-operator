package e2e_tests

import (
	"fmt"

	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/util/retry"

	mock "github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const adminAnnotationValue = "kubernetes-admin"

var _ = Describe("Validate the mutating webhook", func() {
	AfterEach(func() {
		// Revert k8sClient back to use the original configuration
		utilst.SwitchUser(&k8sClient, cfg, testconsts.NSName, newScheme(), "")
	})

	It("Should add annotation on create", func() {
		baseCapp := mock.CreateBaseCapp()
		capp := utilst.CreateCapp(k8sClient, baseCapp)

		annotation := capp.ObjectMeta.Annotations[testconsts.LastUpdatedByAnnotationKey]
		Expect(annotation).To(Equal(adminAnnotationValue))
	})

	It("Should add annotation on update", func() {
		baseCapp := mock.CreateBaseCapp()
		capp := utilst.CreateCapp(k8sClient, baseCapp)

		annotation := capp.ObjectMeta.Annotations[testconsts.LastUpdatedByAnnotationKey]
		Expect(annotation).To(Equal(adminAnnotationValue))

		utilst.SwitchUser(&k8sClient, cfg, testconsts.NSName, newScheme(), testconsts.ServiceAccountName)

		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			capp = utilst.GetCapp(k8sClient, capp.Name, capp.Namespace)
			if capp.ObjectMeta.Annotations == nil {
				capp.ObjectMeta.Annotations = map[string]string{}
			}
			capp.ObjectMeta.Annotations["test"] = "test"
			return utilst.UpdateResource(k8sClient, capp)
		})
		Expect(err).ToNot(HaveOccurred())

		updatedCapp := utilst.GetCapp(k8sClient, capp.Name, capp.Namespace)

		// Check if the annotation has changed
		updatedAnnotation := updatedCapp.ObjectMeta.Annotations[testconsts.LastUpdatedByAnnotationKey]
		Expect(updatedAnnotation).To(Equal(fmt.Sprintf(testconsts.ServiceAccountNameFormat, testconsts.NSName, testconsts.ServiceAccountName)))
	})

	It("Should add default resources to Capp", func() {
		baseCapp := mock.CreateBaseCapp()
		capp := utilst.CreateCapp(k8sClient, baseCapp)

		cpuRequest := capp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Resources.Requests.Cpu()
		Expect(cpuRequest.String()).ToNot(Equal(""))

		memoryRequest := capp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Resources.Requests.Memory()
		Expect(memoryRequest.String()).ToNot(Equal(""))
	})

	It("Should not override existing resources of Capp with default values", func() {
		baseCapp := mock.CreateBaseCapp()
		baseCapp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("123m"),
				corev1.ResourceMemory: resource.MustParse("123Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("23m"),
				corev1.ResourceMemory: resource.MustParse("23Mi"),
			},
		}
		capp := utilst.CreateCapp(k8sClient, baseCapp)

		cpuRequest := capp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Resources.Requests.Cpu()
		Expect(cpuRequest.String()).To(Equal("23m"))

		memoryRequest := capp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Resources.Requests.Memory()
		Expect(memoryRequest.String()).To(Equal("23Mi"))

		cpuLimit := capp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Resources.Limits.Cpu()
		Expect(cpuLimit.String()).To(Equal("123m"))

		memoryLimit := capp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Resources.Limits.Memory()
		Expect(memoryLimit.String()).To(Equal("123Mi"))
	})
})
