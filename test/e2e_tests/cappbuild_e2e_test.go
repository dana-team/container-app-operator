package e2e_tests

import (
	"context"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Validate CappBuild controller", func() {
	It("Should update status.observedGeneration", func() {
		ctx := context.Background()
		name := utilst.RandomName("cappbuild")
		cb := &cappv1alpha1.CappBuild{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testconsts.NSName,
			},
			Spec: cappv1alpha1.CappBuildSpec{
				Source: cappv1alpha1.CappBuildSource{
					Type: cappv1alpha1.CappBuildSourceTypeGit,
					Git: cappv1alpha1.CappBuildGitSource{
						URL: "https://github.com/dana-team/container-app-operator",
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, cb)).To(Succeed())

		Eventually(func(g Gomega) {
			latest := &cappv1alpha1.CappBuild{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: testconsts.NSName}, latest)).To(Succeed())
			g.Expect(latest.Generation).ToNot(BeZero())
			g.Expect(latest.Status.ObservedGeneration).To(Equal(latest.Generation))
		}, testconsts.Timeout, testconsts.Interval).Should(Succeed())
	})
})