package e2e

import (
	"context"
	"fmt"
	"strconv"

	"k8s.io/client-go/util/retry"
	"knative.dev/pkg/kmeta"

	"github.com/dana-team/container-app-operator/test/e2e/consts"
	"github.com/dana-team/container-app-operator/test/e2e/mocks"
	"github.com/dana-team/container-app-operator/test/e2e/utils"
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
		desiredCapp := utils.CreateCapp(k8sClient, baseCapp)

		Eventually(func() int {
			cappRevisons, _ := utils.GetCappRevisions(context.Background(), k8sClient, *desiredCapp)
			return len(cappRevisons)
		}, consts.Timeout, consts.Interval).ShouldNot(BeZero(), "Should create CappRevisions")

		cappRevisionName := kmeta.ChildName(desiredCapp.Name, fmt.Sprintf("-%05d", 1))
		utils.GetCappRevision(k8sClient, cappRevisionName, desiredCapp.Namespace)

		By("Updating Capp")
		err := retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
			desiredCapp = utils.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace)
			desiredCapp.Annotations = make(map[string]string)
			desiredCapp.Annotations["test"] = "test"

			return utils.UpdateResource(k8sClient, desiredCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() bool {
			cappRevisions, _ := utils.GetCappRevisions(context.Background(), k8sClient, *desiredCapp)
			return len(cappRevisions) > 1
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should create new CappRevision")

		cappRevisionName = kmeta.ChildName(desiredCapp.Name, fmt.Sprintf("-%05d", 2))
		utils.GetCappRevision(k8sClient, cappRevisionName, desiredCapp.Namespace)

		By("Deleting Capp")
		utils.DeleteCapp(k8sClient, desiredCapp)
		Eventually(func() int {
			cappRevisons, _ := utils.GetCappRevisions(context.Background(), k8sClient, *desiredCapp)
			return len(cappRevisons)
		}, consts.Timeout, consts.Interval).Should(BeZero(), "Should delete all CappRevisions")

	})

	It(fmt.Sprintf("Should limit CappRevisions to %s per Capp", strconv.Itoa(revisionsToKeep)), func() {
		baseCapp := mocks.CreateBaseCapp()

		By("Creating Capp")
		desiredCapp := utils.CreateCapp(k8sClient, baseCapp)
		desiredCapp = utils.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace)
		desiredCapp.Annotations = make(map[string]string)

		By("Checking many updates to Capp")
		for i := 1; i < moreThanRevisionsToKeep; i++ {
			assertValue := fmt.Sprintf("test%s", strconv.Itoa(i))
			err := retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
				desiredCapp = utils.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace)
				desiredCapp.Annotations = make(map[string]string)
				desiredCapp.Annotations["test"] = assertValue

				return utils.UpdateResource(k8sClient, desiredCapp)
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() string {
				return utils.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace).Annotations["test"]
			}, consts.Timeout, consts.Interval).Should(Equal(assertValue), "Should be equal to the updated value")

			desiredCapp = utils.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace)
		}

		Eventually(func() int {
			cappRevisions, _ := utils.GetCappRevisions(context.Background(), k8sClient, *desiredCapp)
			return len(cappRevisions)
		}, consts.Timeout, consts.Interval).Should(BeNumerically("<=", revisionsToKeep),
			fmt.Sprintf("Should limit to at most %s CappRevision", strconv.Itoa(revisionsToKeep)))
	})
})
