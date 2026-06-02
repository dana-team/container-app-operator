package autoscale

import (
	"fmt"
	"slices"
	"time"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	kautoscaling "knative.dev/serving/pkg/apis/autoscaling"
)

const (
	AutoScalerSubString = "autoscaling"
	rpsScaleKey         = "rps"
	cpuScaleKey         = "cpu"
	memoryScaleKey      = "memory"
	concurrencyScaleKey = "concurrency"
)

var KPAMetrics = []string{"rps", "concurrency"}

// SetAutoScaler takes a Capp and a Knative Service and sets the autoscaler annotations based on the Capp's ScaleSpec.Metric value.
// Returns a map of the autoscaler annotations that were set.
func SetAutoScaler(capp cappv1alpha1.Capp, defaults cappv1alpha1.AutoscaleConfig) map[string]string {
	scaleMetric := capp.Spec.ScaleSpec.Metric
	autoScaleAnnotations := make(map[string]string)
	if scaleMetric == "" {
		return autoScaleAnnotations
	}

	activationScale := defaults.ActivationScale

	givenAutoScaleAnnotation := utils.FilterMap(capp.Spec.ConfigurationSpec.Template.Annotations, AutoScalerSubString)
	autoScaleAnnotations[kautoscaling.ClassAnnotationKey] = getAutoScaleClassByMetric(scaleMetric)
	autoScaleAnnotations[kautoscaling.MetricAnnotationKey] = scaleMetric
	autoScaleAnnotations[kautoscaling.TargetAnnotationKey] = getTargetValue(scaleMetric, defaults)
	autoScaleAnnotations[kautoscaling.ActivationScaleKey] = fmt.Sprintf("%d", activationScale)

	if capp.Spec.ScaleSpec.MinReplicas != 0 {
		autoScaleAnnotations[kautoscaling.MinScaleAnnotationKey] = fmt.Sprintf("%d", capp.Spec.ScaleSpec.MinReplicas)
	}

	if capp.Spec.ScaleSpec.ScaleDelaySeconds != 0 {
		autoScaleAnnotations[kautoscaling.ScaleDownDelayAnnotationKey] = (time.Duration(capp.Spec.ScaleSpec.ScaleDelaySeconds) * time.Second).String()
	}

	autoScaleAnnotations = utils.MergeMaps(autoScaleAnnotations, givenAutoScaleAnnotation)

	return autoScaleAnnotations
}

// getTargetValue returns the target value for autoscaling based on the provided scale metric.
// It uses the AutoscaleConfig struct to determine the appropriate target value.
func getTargetValue(scaleMetric string, autoscale cappv1alpha1.AutoscaleConfig) string {
	var targetValue string
	switch scaleMetric {
	case rpsScaleKey:
		targetValue = fmt.Sprintf("%d", autoscale.RPS)
	case cpuScaleKey:
		targetValue = fmt.Sprintf("%d", autoscale.CPU)
	case memoryScaleKey:
		targetValue = fmt.Sprintf("%d", autoscale.Memory)
	case concurrencyScaleKey:
		targetValue = fmt.Sprintf("%d", autoscale.Concurrency)
	default:
		targetValue = "" // handle unknown scale metrics
	}
	return targetValue
}

// Determines the autoscaling class based on the metric provided. Returns "kpa.autoscaling.knative.dev" if the metric is in KPAMetrics, "hpa.autoscaling.knative.dev" otherwise.
func getAutoScaleClassByMetric(metric string) string {
	if slices.Contains(KPAMetrics, metric) {
		return "kpa.autoscaling.knative.dev"
	}
	return "hpa.autoscaling.knative.dev"
}
