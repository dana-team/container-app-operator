package e2e_tests

import (
	"context"
	"fmt"
	"strconv"

	"k8s.io/client-go/util/retry"
	"knative.dev/pkg/kmeta"

	"github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	moreThanRevisionsToKeep = 12
	revisionsToKeep         = 10
)

var _ = Describe("Validate CappRevision creation", func() {
	It("Should validate CappRevison lifecycle based on Capp lifecycle", func() {
		baseCapp := mocks.CreateBaseCapp()
		By("Creating regular Capp")
		desiredCapp := utilst.CreateCapp(k8sClient, baseCapp)

		Eventually(func() int {
			cappRevisons, _ := utilst.GetCappRevisions(context.Background(), k8sClient, *desiredCapp)
			return len(cappRevisons)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeZero(), "Should create CappRevisions")

		cappRevisionName := kmeta.ChildName(desiredCapp.Name, fmt.Sprintf("-%05d", 1))
		utilst.GetCappRevision(k8sClient, cappRevisionName, desiredCapp.Namespace)

		By("Updating Capp")
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			desiredCapp = utilst.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace)
			desiredCapp.Annotations = make(map[string]string)
			desiredCapp.Annotations["test"] = "test"

			return utilst.UpdateResource(k8sClient, desiredCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() bool {
			cappRevisions, _ := utilst.GetCappRevisions(context.Background(), k8sClient, *desiredCapp)
			return len(cappRevisions) > 1
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should create new CappRevision")

		cappRevisionName = kmeta.ChildName(desiredCapp.Name, fmt.Sprintf("-%05d", 2))
		utilst.GetCappRevision(k8sClient, cappRevisionName, desiredCapp.Namespace)

		By("Deleting Capp")
		utilst.DeleteCapp(k8sClient, desiredCapp)
		Eventually(func() int {
			cappRevisons, _ := utilst.GetCappRevisions(context.Background(), k8sClient, *desiredCapp)
			return len(cappRevisons)
		}, testconsts.Timeout, testconsts.Interval).Should(BeZero(), "Should delete all CappRevisions")

	})

	It(fmt.Sprintf("Should limit CappRevisions to %s per Capp", strconv.Itoa(revisionsToKeep)), func() {
		baseCapp := mocks.CreateBaseCapp()

		By("Creating Capp")
		desiredCapp := utilst.CreateCapp(k8sClient, baseCapp)
		desiredCapp = utilst.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace)
		desiredCapp.Annotations = make(map[string]string)

		By("Checking many updates to Capp")
		for i := 1; i < moreThanRevisionsToKeep; i++ {
			assertValue := fmt.Sprintf("test%s", strconv.Itoa(i))
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				desiredCapp = utilst.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace)
				desiredCapp.Annotations = make(map[string]string)
				desiredCapp.Annotations["test"] = assertValue

				return utilst.UpdateResource(k8sClient, desiredCapp)
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() string {
				return utilst.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace).Annotations["test"]
			}, testconsts.Timeout, testconsts.Interval).Should(Equal(assertValue), "Should be equal to the updated value")

			desiredCapp = utilst.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace)
			if i < revisionsToKeep {
				Eventually(func() int {
					cappRevisons, _ := utilst.GetCappRevisions(context.Background(), k8sClient, *desiredCapp)
					return len(cappRevisons)
				}, testconsts.Timeout, testconsts.Interval).Should(Equal(i+1), fmt.Sprintf("Should get %v CappRevisions for %v updates", i+1, i))
			}
		}

		Eventually(func() int {
			cappRevisions, _ := utilst.GetCappRevisions(context.Background(), k8sClient, *desiredCapp)
			return len(cappRevisions)
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(revisionsToKeep),
			fmt.Sprintf("Should limit to %s CappRevision", strconv.Itoa(revisionsToKeep)))
	})
})
