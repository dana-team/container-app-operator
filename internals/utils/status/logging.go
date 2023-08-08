package status_utils

import (
	"context"
	"time"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func SyncLoggingStatus(ctx context.Context, capp rcsv1alpha1.Capp, log logr.Logger, r client.Client) error {
	cappObject := rcsv1alpha1.Capp{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, &cappObject); err != nil {
		return err
	}

	flow := &loggingv1beta1.Flow{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name + "-flow"}, flow); err != nil {
		return err
	}

	output := &loggingv1beta1.Output{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name + "-output"}, output); err != nil {
		return err
	}
	cappObject.Status.LoggingStatus.Flow = flow.Status
	flowProblems := "True"
	flowReason := "DoesNotHaveProblems"
	if flow.Status.ProblemsCount != 0 {
		flowProblems = "False"
		flowReason = "HasProblems"
	}
	flowCondition := metav1.Condition{
		Type:               "FlowConfiguredCorrectly",
		Status:             metav1.ConditionStatus(flowProblems),
		LastTransitionTime: metav1.Time{Time: time.Now()},
		Reason:             flowReason,
	}
	meta.SetStatusCondition(&cappObject.Status.LoggingStatus.Conditions, flowCondition)
	cappObject.Status.LoggingStatus.Output = output.Status
	outputProblems := "True"
	outputReason := "DoesNotHaveProblems"
	if output.Status.ProblemsCount != 0 {
		outputProblems = "False"
		outputReason = "HasProblems"
	}
	outputCondition := metav1.Condition{
		Type:               "OutputConfiguredCorrectly",
		Status:             metav1.ConditionStatus(outputProblems),
		LastTransitionTime: metav1.Time{Time: time.Now()},
		Reason:             outputReason,
	}
	meta.SetStatusCondition(&cappObject.Status.LoggingStatus.Conditions, outputCondition)

	if err := r.Status().Update(ctx, &cappObject); err != nil {
		log.Error(err, "Cant update capp status")
		return err
	}
	return nil
}
