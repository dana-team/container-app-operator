package e2e_tests

import (
	"context"
	"fmt"
	"strconv"

	"github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	testAnnotationKey   = utilst.Domain + "/test"
	testAnnotationValue = "test"
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

		By("Updating Capp")
		desiredCapp = utilst.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace)
		desiredCapp.Annotations = make(map[string]string)
		desiredCapp.Annotations["test"] = "test"
		utilst.UpdateCapp(k8sClient, desiredCapp)
		Eventually(func() bool {
			cappRevisons, _ := utilst.GetCappRevisions(context.Background(), k8sClient, *desiredCapp)
			return len(cappRevisons) > 1
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should create new CappRevision")

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
		for i := 0; i <= moreThanRevisionsToKeep; i++ {
			assertValue := fmt.Sprintf("test%s", strconv.Itoa(i))
			desiredCapp.Annotations["test"] = assertValue
			utilst.UpdateCapp(k8sClient, desiredCapp)

			Eventually(func() string {
				return utilst.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace).Annotations["test"]
			}, testconsts.Timeout, testconsts.Interval).Should(Equal(assertValue), "Should be equal to the updated value")

			desiredCapp = utilst.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace)
		}

		Eventually(func() int {
			cappRevisions, _ := utilst.GetCappRevisions(context.Background(), k8sClient, *desiredCapp)
			return len(cappRevisions)
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(revisionsToKeep),
			fmt.Sprintf("Should limit to %s CappRevision", strconv.Itoa(revisionsToKeep)))

	})

	It(fmt.Sprintf("Should copy annotations containing %s to CappRevision", utilst.Domain), func() {
		baseCapp := mocks.CreateBaseCapp()
		baseCapp.Annotations = map[string]string{testAnnotationKey: testAnnotationValue}
		By("Creating Capp")
		desiredCapp := utilst.CreateCapp(k8sClient, baseCapp)
		desiredCapp = utilst.GetCapp(k8sClient, desiredCapp.Name, desiredCapp.Namespace)

		Eventually(func() string {
			cappRevisions, _ := utilst.GetCappRevisions(context.Background(), k8sClient, *desiredCapp)
			val, ok := cappRevisions[0].Annotations[testAnnotationKey]
			if ok {
				return val
			}
			return ""
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(testAnnotationValue),
			"Should copy annotations to CappRevision")

	})
})
