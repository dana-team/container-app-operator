package status_utils

import (
	"context"
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

// buildLoggingStatus builds the Logging status of the Capp CRD by getting the Flow and Output objects
// bundled to the Capp and adding their status. It also creates a condition in accordance with their situation.
func buildLoggingStatus(ctx context.Context, capp rcsv1alpha1.Capp, log logr.Logger, r client.Client) (rcsv1alpha1.LoggingStatus, error) {
	logger := log.WithValues("FlowName", capp.Name+"-flow", "OutputName", capp.Name+"-output")
	loggingStatus := rcsv1alpha1.LoggingStatus{}

	if capp.Spec.LogSpec == (rcsv1alpha1.LogSpec{}) {
		return loggingStatus, nil
	}

	flow := &loggingv1beta1.Flow{}
	logger.Info("Building logger status")
	logger.Info("Trying to fetch existing flow")

	if err := r.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name + "-flow"},
		flow); err != nil {
		logger.Error(err, "Failed to fetch flow")
		return loggingStatus, err
	}
	logger.Info("Fetched flow successfully.")

	logger.Info("Trying to fetch existing output")
	output := &loggingv1beta1.Output{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name + "-output"},
		output); err != nil {
		logger.Error(err, "Failed to fetch output")
		return loggingStatus, err
	}
	logger.Info("Fetched output successfully.")
	loggingStatus.Flow = flow.Status
	loggingStatus.Output = output.Status

	problems := "True"
	reason := "Ready"
	if flow.Status.ProblemsCount != 0 || output.Status.ProblemsCount != 0 {
		reason = "LoggingResourceInvalid"
		problems = "False"
	}

	condition := metav1.Condition{
		Type:               "LoggingIsReady",
		Status:             metav1.ConditionStatus(problems),
		LastTransitionTime: metav1.Time{Time: time.Now()},
		Reason:             reason,
	}

	meta.SetStatusCondition(&loggingStatus.Conditions, condition)
	logger.Info("Built logger status successfully ")
	return loggingStatus, nil
}
