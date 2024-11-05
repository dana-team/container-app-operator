package autoscale

import (
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"k8s.io/utils/strings/slices"
)

const (
	KnativeMetricKey          = "autoscaling.knative.dev/metric"
	KnativeAutoscaleClassKey  = "autoscaling.knative.dev/class"
	KnativeAutoscaleTargetKey = "autoscaling.knative.dev/target"
	AutoScalerSubString       = "autoscaling"
	KnativeActivationScaleKey = "autoscaling.knative.dev/activation-scale"
	rpsScaleKey               = "rps"
	cpuScaleKey               = "cpu"
	memoryScaleKey            = "memory"
	concurrencyScaleKey       = "concurrency"
)

var KPAMetrics = []string{"rps", "concurrency"}

// SetAutoScaler takes a Capp and a Knative Service and sets the autoscaler annotations based on the Capp's ScaleMetric.
// Returns a map of the autoscaler annotations that were set.
func SetAutoScaler(capp cappv1alpha1.Capp, defaults cappv1alpha1.AutoscaleConfig) map[string]string {
	scaleMetric := capp.Spec.ScaleMetric
	autoScaleAnnotations := make(map[string]string)
	if scaleMetric == "" {
		return autoScaleAnnotations
	}

	activationScale := defaults.ActivationScale

	givenAutoScaleAnnotation := utils.FilterMap(capp.Spec.ConfigurationSpec.Template.Annotations, AutoScalerSubString)

	autoScaleAnnotations[KnativeAutoscaleClassKey] = getAutoScaleClassByMetric(scaleMetric)
	autoScaleAnnotations[KnativeMetricKey] = scaleMetric
	autoScaleAnnotations[KnativeAutoscaleTargetKey] = getTargetValue(scaleMetric, defaults)
	autoScaleAnnotations[KnativeActivationScaleKey] = activationScale
	autoScaleAnnotations = utils.MergeMaps(autoScaleAnnotations, givenAutoScaleAnnotation)

	return autoScaleAnnotations
}

// getTargetValue returns the target value for autoscaling based on the provided scale metric.
// It uses the AutoscaleConfig struct to determine the appropriate target value.
func getTargetValue(scaleMetric string, autoscale cappv1alpha1.AutoscaleConfig) string {
	var targetValue string
	switch scaleMetric {
	case rpsScaleKey:
		targetValue = autoscale.RPS
	case cpuScaleKey:
		targetValue = autoscale.CPU
	case memoryScaleKey:
		targetValue = autoscale.Memory
	case concurrencyScaleKey:
		targetValue = autoscale.Concurrency
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
