package status

import (
	"context"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	loggingResourceInvalid = "LoggingResourceInvalid"
	loggingReady           = "LoggingIsReady"
	conditionReady         = "ready"
)

// buildLoggingStatus builds the Logging status of the Capp CRD by getting the SyslogNGFlow and SyslogNGOutput objects
// bundled to the Capp and adding their status. It also creates a condition in accordance with their situation.
func buildLoggingStatus(ctx context.Context, capp cappv1alpha1.Capp, log logr.Logger, r client.Client, existing cappv1alpha1.LoggingStatus, isRequired bool) (cappv1alpha1.LoggingStatus, error) {
	logger := log.WithValues("SyslogNGFlowName", capp.Name, "SyslogNGOutputName", capp.Name)

	if !isRequired {
		return cappv1alpha1.LoggingStatus{}, nil
	}

	syslogNGFlow := &loggingv1beta1.SyslogNGFlow{}
	logger.Info("Building logger status")

	if err := r.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, syslogNGFlow); err != nil {
		if apierrors.IsNotFound(err) {
			return cappv1alpha1.LoggingStatus{}, nil
		}
		logger.Error(err, "Failed to fetch SyslogNGFlow")
		return cappv1alpha1.LoggingStatus{}, err
	}

	syslogNGOutput := &loggingv1beta1.SyslogNGOutput{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, syslogNGOutput); err != nil {
		if apierrors.IsNotFound(err) {
			return cappv1alpha1.LoggingStatus{}, nil
		}
		logger.Error(err, "Failed to fetch SyslogNGOutput")
		return cappv1alpha1.LoggingStatus{}, err
	}

	loggingStatus := *existing.DeepCopy()
	loggingStatus.SyslogNGFlow = syslogNGFlow.Status
	loggingStatus.SyslogNGOutput = syslogNGOutput.Status

	isLoggingHealthy := syslogNGFlow.Status.ProblemsCount == 0 && syslogNGOutput.Status.ProblemsCount == 0
	status := metav1.ConditionTrue
	reason := conditionReady
	if !isLoggingHealthy {
		status = metav1.ConditionFalse
		reason = loggingResourceInvalid
	}

	condition := metav1.Condition{
		Type:   loggingReady,
		Status: status,
		Reason: reason,
	}

	meta.SetStatusCondition(&loggingStatus.Conditions, condition)
	logger.Info("Successfully built logger status")

	return loggingStatus, nil
}
