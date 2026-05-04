package e2e

import (
	"fmt"

	"k8s.io/client-go/util/retry"

	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"

	"github.com/dana-team/container-app-operator/test/e2e/consts"

	"github.com/dana-team/container-app-operator/test/e2e/mocks"
	utilst "github.com/dana-team/container-app-operator/test/e2e/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// checkOutputParameters checks if the SyslogNGOutput index value matches the desired value based on the logger type.
func checkOutputParameters(logType cappv1alpha1.LogType, syslogNGOutputName string, syslogNGOutputNamespace string, indexDesiredValue string, urlDesiredValue string) {
	switch logType {
	case cappv1alpha1.LogTypeElastic:
		Eventually(func() string {
			syslogNGOutput := utilst.GetSyslogNGOutput(k8sClient, syslogNGOutputName, syslogNGOutputNamespace)
			return syslogNGOutput.Spec.Elasticsearch.Index
		}, consts.Timeout, consts.Interval).Should(Equal(indexDesiredValue))
	case cappv1alpha1.LogTypeElasticDataStream:
		Eventually(func() string {
			syslogNGOutput := utilst.GetSyslogNGOutput(k8sClient, syslogNGOutputName, syslogNGOutputNamespace)
			return syslogNGOutput.Spec.ElasticsearchDatastream.URL
		}, consts.Timeout, consts.Interval).Should(Equal(urlDesiredValue))
	}
}

// editCappLogSpec updates the Capp's LogSpec based on the logger type.
func editCappLogSpec(capp *cappv1alpha1.Capp, logType cappv1alpha1.LogType) {
	switch logType {
	case cappv1alpha1.LogTypeElastic:
		capp.Spec.LogSpec.Index = consts.TestIndex
	case cappv1alpha1.LogTypeElasticDataStream:
		capp.Spec.LogSpec.Host = consts.ElasticDataStreamURL
	}
}

// testCappWithLogger performs a comprehensive test for creating, updating, and deleting
// a Capp instance with a specified logger type.
func testCappWithLogger(logType cappv1alpha1.LogType) {
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
			if syslogNGOutput.Status.Active == nil {
				return false
			}
			return *syslogNGOutput.Status.Active
		}, consts.Timeout, consts.Interval).Should(BeTrue())

		By("Checking the SyslogNGOutput has the needed labels")
		Expect(syslogNGOutput.Labels[consts.CappResourceKey]).Should(Equal(createdCapp.Name))
		Expect(syslogNGOutput.Labels[consts.ManagedByLabelKey]).Should(Equal(consts.CappKey))

		Eventually(func() int {
			syslogNGOutput = utilst.GetSyslogNGOutput(k8sClient, syslogNGOutputName, createdCapp.Namespace)
			return syslogNGOutput.Status.ProblemsCount
		}, consts.Timeout, consts.Interval).Should(Equal(0))

		By("Checking if the SyslogNGFlow was created successfully and active")
		syslogNGFlowName := createdCapp.Name
		syslogNGFlowObject := mocks.CreateSyslogNGFlowObject(syslogNGFlowName)

		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, syslogNGFlowObject)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Checking the SyslogNGFlow has the needed labels")
		syslogNGFlowObject = utilst.GetSyslogNGFlow(k8sClient, syslogNGFlowName, consts.NSName)
		Expect(syslogNGFlowObject.Labels[consts.CappResourceKey]).Should(Equal(createdCapp.Name))
		Expect(syslogNGFlowObject.Labels[consts.ManagedByLabelKey]).Should(Equal(consts.CappKey))

		Eventually(func() bool {
			syslogNGFlow := utilst.GetSyslogNGFlow(k8sClient, syslogNGFlowName, createdCapp.Namespace)
			if syslogNGFlow.Status.Active == nil {
				return false
			}
			return *syslogNGFlow.Status.Active
		}, consts.Timeout, consts.Interval).Should(BeTrue())

		By(fmt.Sprintf("Updating the capp %s logger index/url", logType))
		err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			editCappLogSpec(toBeUpdatedCapp, logType)

			return utilst.UpdateResource(k8sClient, toBeUpdatedCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		By("Checking if the SyslogNGOutput index/url was updated")
		checkOutputParameters(logType, syslogNGOutputName, createdCapp.Namespace, consts.TestIndex, consts.ElasticDataStreamURL)

		By("Deleting the Capp instance")
		utilst.DeleteCapp(k8sClient, createdCapp)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, createdCapp)
		}, consts.Timeout, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the SyslogNGOutput was deleted successfully")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, syslogNGOutputObject)
		}, consts.Timeout, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the SyslogNGFlow was deleted successfully")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, syslogNGFlowObject)
		}, consts.Timeout, consts.Interval).ShouldNot(BeTrue(), "Should not find a resource.")
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
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")

		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, syslogNGOutputObject)
		}, consts.Timeout, consts.Interval).Should(BeTrue(), "Should find a resource.")

		By("Removing the logging requirement from Capp Spec and checking cleanup")
		err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
			toBeUpdatedCapp.Spec.LogSpec = cappv1alpha1.LogSpec{}

			return utilst.UpdateResource(k8sClient, toBeUpdatedCapp)
		})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, syslogNGFlowObject)
		}, consts.Timeout, consts.Interval).Should(BeFalse(), "Should not find a resource.")

		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, syslogNGOutputObject)
		}, consts.Timeout, consts.Interval).Should(BeFalse(), "Should not find a resource.")
	})
}

var _ = Describe("Validate Logger functionality", func() {
	testCappWithLogger(cappv1alpha1.LogTypeElastic)
	testCappWithLogger(cappv1alpha1.LogTypeElasticDataStream)
})
