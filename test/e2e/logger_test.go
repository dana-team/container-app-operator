package e2e

import (
	"fmt"

	"k8s.io/client-go/util/retry"

	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"

	"github.com/dana-team/container-app-operator/test/e2e/consts"

	"github.com/dana-team/container-app-operator/test/e2e/mocks"
	"github.com/dana-team/container-app-operator/test/e2e/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// checkOutputParameters verifies that the SyslogNGOutput contains the expected Elasticsearch value for the logger type.
func checkOutputParameters(logType cappv1alpha1.LogType, syslogNGOutputName string, syslogNGOutputNamespace string, targetDesiredValue string) {
	switch logType {
	case cappv1alpha1.LogTypeElastic:
		Eventually(func() string {
			syslogNGOutput := utils.GetSyslogNGOutput(k8sClient, syslogNGOutputName, syslogNGOutputNamespace)
			return syslogNGOutput.Spec.Elasticsearch.Index
		}, consts.Timeout, consts.Interval).Should(Equal(targetDesiredValue))
	case cappv1alpha1.LogTypeElasticDataStream:
		expectedURL := fmt.Sprintf("https://%s/%s/_bulk", consts.ElasticHost, targetDesiredValue)
		Eventually(func() string {
			syslogNGOutput := utils.GetSyslogNGOutput(k8sClient, syslogNGOutputName, syslogNGOutputNamespace)
			return syslogNGOutput.Spec.ElasticsearchDatastream.URL
		}, consts.Timeout, consts.Interval).Should(Equal(expectedURL))
	}
}

// editCappLogSpec updates the Capp's LogSpec target.
func editCappLogSpec(capp *cappv1alpha1.Capp) {
	capp.Spec.LogSpec.Target = consts.TestTarget
}

// testCappWithLogger performs a comprehensive test for creating, updating, and deleting
// a Capp instance with a specified logger type.
func testCappWithLogger(logType cappv1alpha1.LogType) {
	It(fmt.Sprintf("Should create, update, and delete SyslogNGFlow and SyslogNGOutput when creating, updating, and deleting a Capp instance with %s logger", logType), func() {
		By(fmt.Sprintf("Creating a secret containing %s credentials", logType))
		utils.CreateCredentialsSecret(logType, k8sClient)

		By(fmt.Sprintf("Creating a Capp with %s logger", logType))
		createdCapp := utils.CreateCappWithLogger(Default, logType, k8sClient)

		syslogNGOutputName := createdCapp.Name
		syslogNGOutputObject := mocks.CreateSyslogNGOutputObject(syslogNGOutputName)

		By("Checking if the SyslogNGOutput is active and has no problems")
		syslogNGOutput := &loggingv1beta1.SyslogNGOutput{}
		Eventually(func() bool {
			syslogNGOutput = utils.GetSyslogNGOutput(k8sClient, syslogNGOutputName, createdCapp.Namespace)
			if syslogNGOutput.Status.Active == nil {
				return false
			}
			return *syslogNGOutput.Status.Active
		}, consts.Timeout, consts.Interval).Should(BeTrue())

		By("Checking the SyslogNGOutput has the needed labels")
		Expect(syslogNGOutput.Labels[consts.CappResourceKey]).Should(Equal(createdCapp.Name))
		Expect(syslogNGOutput.Labels[consts.ManagedByLabelKey]).Should(Equal(consts.CappKey))

		Eventually(func() int {
			syslogNGOutput = utils.GetSyslogNGOutput(k8sClient, syslogNGOutputName, createdCapp.Namespace)
			return syslogNGOutput.Status.ProblemsCount
		}, consts.Timeout, consts.Interval).Should(Equal(0))

		By("Checking if the SyslogNGFlow was created successfully and active")
		syslogNGFlowName := createdCapp.Name
		syslogNGFlowObject := mocks.CreateSyslogNGFlowObject(syslogNGFlowName)

		Eventually(func() (bool, error) {
			return utils.ResourceExists(k8sClient, syslogNGFlowObject)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Checking the SyslogNGFlow has the needed labels")
		syslogNGFlowObject = utils.GetSyslogNGFlow(k8sClient, syslogNGFlowName, consts.NSName)
		Expect(syslogNGFlowObject.Labels[consts.CappResourceKey]).Should(Equal(createdCapp.Name))
		Expect(syslogNGFlowObject.Labels[consts.ManagedByLabelKey]).Should(Equal(consts.CappKey))

		Eventually(func() bool {
			syslogNGFlow := utils.GetSyslogNGFlow(k8sClient, syslogNGFlowName, createdCapp.Namespace)
			if syslogNGFlow.Status.Active == nil {
				return false
			}
			return *syslogNGFlow.Status.Active
		}, consts.Timeout, consts.Interval).Should(BeTrue())

		By(fmt.Sprintf("Updating the capp %s logger target", logType))
		err := retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
			toBeUpdatedCapp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			editCappLogSpec(toBeUpdatedCapp)

			return utils.UpdateResource(k8sClient, toBeUpdatedCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		By("Checking if the SyslogNGOutput index/url was updated according to the target")
		checkOutputParameters(logType, syslogNGOutputName, createdCapp.Namespace, consts.TestTarget)

		By("Deleting the Capp instance")
		utils.DeleteCapp(Default, k8sClient, createdCapp)

		By("Checking if the SyslogNGOutput was deleted successfully")
		Eventually(func() (bool, error) {
			return utils.ResourceExists(k8sClient, syslogNGOutputObject)
		}, consts.Timeout, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the SyslogNGFlow was deleted successfully")
		Eventually(func() (bool, error) {
			return utils.ResourceExists(k8sClient, syslogNGFlowObject)
		}, consts.Timeout, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")
	})

	It("Should cleanup SyslogNGFlow and SyslogNGOutput when they are no longer required", func() {
		By(fmt.Sprintf("Creating a secret containing %s credentials", logType))
		utils.CreateCredentialsSecret(logType, k8sClient)

		By(fmt.Sprintf("Creating a Capp with %s logger", logType))
		createdCapp := utils.CreateCappWithLogger(Default, logType, k8sClient)

		By("Checking if the SyslogNGFlow and SyslogNGOutput were created successfully")
		syslogNGFlowName := createdCapp.Name
		syslogNGFlowObject := mocks.CreateSyslogNGFlowObject(syslogNGFlowName)

		syslogNGOutputName := createdCapp.Name
		syslogNGOutputObject := mocks.CreateSyslogNGOutputObject(syslogNGOutputName)

		Eventually(func() (bool, error) {
			return utils.ResourceExists(k8sClient, syslogNGFlowObject)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")

		Eventually(func() (bool, error) {
			return utils.ResourceExists(k8sClient, syslogNGOutputObject)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Removing the logging requirement from Capp Spec and checking cleanup")
		err := retry.RetryOnConflict(utils.NewRetryOnConflictBackoff(), func() error {
			toBeUpdatedCapp := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			toBeUpdatedCapp.Spec.LogSpec = cappv1alpha1.LogSpec{}

			return utils.UpdateResource(k8sClient, toBeUpdatedCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() (bool, error) {
			return utils.ResourceExists(k8sClient, syslogNGFlowObject)
		}, consts.Timeout, consts.Interval).Should(BeFalse(), "Should not find a resource.")

		Eventually(func() (bool, error) {
			return utils.ResourceExists(k8sClient, syslogNGOutputObject)
		}, consts.Timeout, consts.Interval).Should(BeFalse(), "Should not find a resource.")
	})
}

var _ = Describe("Validate Logger functionality", func() {
	testCappWithLogger(cappv1alpha1.LogTypeElastic)
	testCappWithLogger(cappv1alpha1.LogTypeElasticDataStream)
})
