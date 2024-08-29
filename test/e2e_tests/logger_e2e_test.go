package e2e_tests

import (
	"fmt"

	"k8s.io/client-go/util/retry"

	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"

	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"

	"github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// checkOutputIndexValue checks if the SyslogLogOutput index value matches the desired value based on the logger type.
func checkOutputIndexValue(logType string, syslogNGOutputName string, syslogNGOutputNamespace string, IndexDesiredValue string) {
	switch logType {
	case mocks.ElasticType:
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
		syslogNGOutputObject := mocks.CreateSyslogNGOutputObject(syslogNGOutputName)

		By(fmt.Sprintf("Creating a secret containing %s credentials", logType))
		utilst.CreateCredentialsSecret(logType, k8sClient)

		By("Checking if the SyslogNGOutput is active and has no problems")
		syslogNGOutput := &loggingv1beta1.SyslogNGOutput{}
		Eventually(func() bool {
			syslogNGOutput = utilst.GetSyslogNGOutput(k8sClient, syslogNGOutputName, createdCapp.Namespace)
			return *syslogNGOutput.Status.Active
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue())

		By("Checking the SyslogNGOutput has the needed labels")
		Expect(syslogNGOutput.Labels[testconsts.CappResourceKey]).Should(Equal(createdCapp.Name))
		Expect(syslogNGOutput.Labels[testconsts.ManagedByLabelKey]).Should(Equal(testconsts.CappKey))

		Eventually(func() int {
			syslogNGOutput = utilst.GetSyslogNGOutput(k8sClient, syslogNGOutputName, createdCapp.Namespace)
			return syslogNGOutput.Status.ProblemsCount
		}, testconsts.Timeout, testconsts.Interval).Should(Equal(0))

		By("Checking if the SyslogNGFlow was created successfully and active")
		syslogNGFlowName := createdCapp.Name
		syslogNGFlowObject := mocks.CreateSyslogNGFlowObject(syslogNGFlowName)

		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, syslogNGFlowObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Checking the SyslogNGFlow has the needed labels")
		syslogNGFlowObject = utilst.GetSyslogNGFlow(k8sClient, syslogNGFlowName, mocks.NSName)
		Expect(syslogNGFlowObject.Labels[testconsts.CappResourceKey]).Should(Equal(createdCapp.Name))
		Expect(syslogNGFlowObject.Labels[testconsts.ManagedByLabelKey]).Should(Equal(testconsts.CappKey))

		Eventually(func() bool {
			syslogNGFlow := utilst.GetSyslogNGFlow(k8sClient, syslogNGFlowName, createdCapp.Namespace)
			return *syslogNGFlow.Status.Active
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue())

		By(fmt.Sprintf("Updating the capp %s logger index", logType))
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			toBeUpdatedCapp.Spec.LogSpec.Index = testconsts.TestIndex

			return utilst.UpdateResource(k8sClient, toBeUpdatedCapp)
		})
		Expect(err).To(BeNil())

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

	It("Should cleanup SyslogNGFlow and SyslogNGOutput when they are no longer required", func() {
		By(fmt.Sprintf("Creating a Capp with %s logger", logType))
		createdCapp := utilst.CreateCappWithLogger(logType, k8sClient)

		By("Checking if the SyslogNGFlow and SyslogNGOutput were created successfully")
		syslogNGFlowName := createdCapp.Name
		syslogNGFlowObject := mocks.CreateSyslogNGFlowObject(syslogNGFlowName)

		syslogNGOutputName := createdCapp.Name
		syslogNGOutputObject := mocks.CreateSyslogNGOutputObject(syslogNGOutputName)

		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, syslogNGFlowObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, syslogNGOutputObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Removing the logging requirement from Capp Spec and checking cleanup")
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			toBeUpdatedCapp.Spec.LogSpec = cappv1alpha1.LogSpec{}

			return utilst.UpdateResource(k8sClient, toBeUpdatedCapp)
		})
		Expect(err).To(BeNil())

		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, syslogNGFlowObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeFalse(), "Should not find a resource.")

		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, syslogNGOutputObject)
		}, testconsts.Timeout, testconsts.Interval).Should(BeFalse(), "Should not find a resource.")
	})
}

var _ = Describe("Validate Logger functionality", func() {
	testCappWithLogger(mocks.ElasticType)
})
