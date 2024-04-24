package e2e_tests

import (
	"fmt"

	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"

	mock "github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// checkOutputIndexValue checks if the SyslogLogOutput index value matches the desired value based on the logger type.
func checkOutputIndexValue(logType string, syslogNGOutputName string, syslogNGOutputNamespace string, IndexDesiredValue string) {
	switch logType {
	case mock.ElasticType:
		Eventually(func() string {
			syslogNGOutput := utilst.GetSyslogNGOutput(k8sClient, syslogNGOutputName, syslogNGOutputNamespace)
			return syslogNGOutput.Spec.Elasticsearch.Index
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(IndexDesiredValue))
	}
}

// testCappWithLogger performs a comprehensive test for creating, updating, and deleting
// a Capp instance with a specified logger type.
func testCappWithLogger(logType string) {
	It(fmt.Sprintf("Should create, update, and delete SyslogNGFlow and SyslogNGOutput when creating, updating, and deleting a Capp instance with %s logger", logType), func() {
		By(fmt.Sprintf("Creating a Capp with %s logger", logType))
		createdCapp := utilst.CreateCappWithLogger(logType, k8sClient)

		syslogNGOutputName := createdCapp.Name
		syslogNGOutputObject := mock.CreateSyslogNGOutputObject(syslogNGOutputName)

		By(fmt.Sprintf("Creating a secret containing %s credentials", logType))
		utilst.CreateCredentialsSecret(logType, k8sClient)

		By("Checking if the SyslogNGOutput is active and has no problems")
		Eventually(func() bool {
			syslogNGOutput := utilst.GetSyslogNGOutput(k8sClient, syslogNGOutputName, createdCapp.Namespace)
			return *syslogNGOutput.Status.Active
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue())

		Eventually(func() int {
			syslogNGOutput := utilst.GetSyslogNGOutput(k8sClient, syslogNGOutputName, createdCapp.Namespace)
			return syslogNGOutput.Status.ProblemsCount
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(0))

		By("Checking if the SyslogNGFlow was created successfully and active")
		syslogNGFlowName := createdCapp.Name
		syslogNGFlowObject := mock.CreateSyslogNGFlowObject(syslogNGFlowName)

		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, syslogNGFlowObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		Eventually(func() bool {
			syslogNGFlow := utilst.GetSyslogNGFlow(k8sClient, syslogNGFlowName, createdCapp.Namespace)
			return *syslogNGFlow.Status.Active
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue())

		By(fmt.Sprintf("Updating the capp %s logger index", logType))
		toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
		toBeUpdatedCapp.Spec.LogSpec.Index = testconsts.TestIndex
		utilst.UpdateCapp(k8sClient, toBeUpdatedCapp)

		By("Checking if the SyslogNGOutput index was updated")
		checkOutputIndexValue(logType, syslogNGOutputName, createdCapp.Namespace, testconsts.TestIndex)

		By("Deleting the Capp instance")
		utilst.DeleteCapp(k8sClient, createdCapp)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, createdCapp)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the SyslogNGOutput was deleted successfully")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, syslogNGOutputObject)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the SyslogNGFlow was deleted successfully")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, syslogNGFlowObject)
		}, testconsts.Timeout, testconsts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")
	})
}

var _ = Describe("Validate Logger functionality", func() {
	testCappWithLogger(mock.ElasticType)
})
