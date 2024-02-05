package autoscale

import (
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"k8s.io/utils/strings/slices"
)

const (
	KnativeMetricKey            = "autoscaling.knative.dev/metric"
	KnativeAutoscaleClassKey    = "autoscaling.knative.dev/class"
	KnativeAutoscaleTargetKey   = "autoscaling.knative.dev/target"
	KnativeActivationScaleKey   = "autoscaling.knative.dev/activation-scale"
	KnativeActivationScaleValue = "3"
)

var TargetDefaultValues = map[string]string{
	"rps":         "200",
	"cpu":         "80",
	"memory":      "70",
	"concurrency": "10",
}

var KPAMetrics = []string{"rps", "concurrency"}

// SetAutoScaler takes a Capp and a Knative Service and sets the autoscaler annotations based on the Capp's ScaleMetric.
// Returns a map of the autoscaler annotations that were set.
func SetAutoScaler(capp rcsv1alpha1.Capp) map[string]string {
	scaleMetric := capp.Spec.ScaleMetric
	autoScaleAnnotations := make(map[string]string)
	if scaleMetric == "" {
		return autoScaleAnnotations
	}
	autoScaleAnnotations[KnativeAutoscaleClassKey] = getAutoScaleClassByMetric(scaleMetric)
	autoScaleAnnotations[KnativeMetricKey] = scaleMetric
	autoScaleAnnotations[KnativeAutoscaleTargetKey] = TargetDefaultValues[scaleMetric]
	autoScaleAnnotations[KnativeActivationScaleKey] = KnativeActivationScaleValue

	return autoScaleAnnotations
}

// Determines the autoscaling class based on the metric provided. Returns "kpa.autoscaling.knative.dev" if the metric is in KPAMetrics, "hpa.autoscaling.knative.dev" otherwise.
func getAutoScaleClassByMetric(metric string) string {
	if slices.Contains(KPAMetrics, metric) {
		return "kpa.autoscaling.knative.dev"
	}
	return "hpa.autoscaling.knative.dev"
}
