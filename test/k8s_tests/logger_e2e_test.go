package k8s_tests

import (
	"fmt"

	mock "github.com/dana-team/container-app-operator/test/k8s_tests/mocks"
	utilst "github.com/dana-team/container-app-operator/test/k8s_tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	outputSuffix = "-output"
	flowSuffix   = "-flow"
	testIndex    = "test"
)

// checkOutputIndexValue checks if the output index value matches the desired value based on the logger type.
func checkOutputIndexValue(logType string, outputName string, outputNamespace string, IndexDesiredValue string) {
	switch logType {
	case mock.ElasticType:
		Eventually(func() string {
			output := utilst.GetOutput(k8sClient, outputName, outputNamespace)
			return output.Spec.ElasticsearchOutput.IndexName
		}, TimeoutCapp, CappCreationInterval).Should(Equal(IndexDesiredValue))
	case mock.SplunkType:
		Eventually(func() string {
			output := utilst.GetOutput(k8sClient, outputName, outputNamespace)
			return output.Spec.SplunkHecOutput.Index
		}, TimeoutCapp, CappCreationInterval).Should(Equal(IndexDesiredValue))
	}
}

// testCappWithLogger performs a comprehensive test for creating, updating, and deleting a Capp instance with a specified logger type.
func testCappWithLogger(logType string) {
	It(fmt.Sprintf("Should create, update, and delete flow and output when creating, updating, and deleting a Capp instance with %s logger", logType), func() {
		By(fmt.Sprintf("Creating a capp with %s logger", logType))
		createdCapp := utilst.CreateCappWithLogger(logType, k8sClient)

		By("Checking if the output is reporting a problem when secret credentials secrert is missing")
		outputName := createdCapp.Name + outputSuffix
		outputObject := mock.CreateOutputObject(outputName)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, outputObject)
		}, TimeoutCapp, CappCreationInterval).Should(BeTrue(), "Should find a resource.")
		Eventually(func() int {
			output := utilst.GetOutput(k8sClient, outputName, createdCapp.Namespace)
			return output.Status.ProblemsCount
		}, TimeoutCapp, CappCreationInterval).Should(Equal(1))

		By(fmt.Sprintf("Creating a secret containing %s credentials", logType))
		utilst.CreateCredentialsSecret(logType, k8sClient)

		By("Checking if the output is active and has no problems")
		Eventually(func() bool {
			output := utilst.GetOutput(k8sClient, outputName, createdCapp.Namespace)
			return *output.Status.Active
		}, TimeoutCapp, CappCreationInterval).Should(BeTrue())
		Eventually(func() int {
			output := utilst.GetOutput(k8sClient, outputName, createdCapp.Namespace)
			return output.Status.ProblemsCount
		}, TimeoutCapp, CappCreationInterval).Should(Equal(0))

		By("Checking if the flow was created successfully and active")
		flowName := createdCapp.Name + flowSuffix
		flowObject := mock.CreateFlowObject(flowName)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, flowObject)
		}, TimeoutCapp, CappCreationInterval).Should(BeTrue(), "Should find a resource.")
		Eventually(func() bool {
			flow := utilst.GetFlow(k8sClient, flowName, createdCapp.Namespace)
			return *flow.Status.Active
		}, TimeoutCapp, CappCreationInterval).Should(BeTrue())

		By(fmt.Sprintf("Updating the capp %s logger index", logType))
		toBeUpdatedCapp := utilst.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace)
		toBeUpdatedCapp.Spec.LogSpec.Index = testIndex
		utilst.UpdateCapp(k8sClient, toBeUpdatedCapp)

		By("checking if the output index was updated")
		checkOutputIndexValue(logType, outputName, createdCapp.Namespace, testIndex)

		By("Deleting the capp instance")
		utilst.DeleteCapp(k8sClient, createdCapp)
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, createdCapp)
		}, TimeoutCapp, CappCreationInterval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the output was deleted successfully")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, outputObject)
		}, TimeoutCapp, CappCreationInterval).ShouldNot(BeTrue(), "Should not find a resource.")

		By("Checking if the flow was deleted successfully")
		Eventually(func() bool {
			return utilst.DoesResourceExist(k8sClient, flowObject)
		}, TimeoutCapp, CappCreationInterval).ShouldNot(BeTrue(), "Should not find a resource.")
	})
}

var _ = Describe("Validate Logger functionality", func() {
	testCappWithLogger(mock.ElasticType)
	testCappWithLogger(mock.SplunkType)
})
