package autoscale

import (
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"k8s.io/utils/strings/slices"
)

const (
	KnativeMetricKey          = "autoscaling.knative.dev/metric"
	KnativeAutoscaleClassKey  = "autoscaling.knative.dev/class"
	KnativeAutoscaleTargetKey = "autoscaling.knative.dev/target"
	KnativeAutoscaleMinKey    = "autoscaling.knative.dev/min-scale"
	KnativeAutoscaleMaxKey    = "autoscaling.knative.dev/max-scale"
	KnativeAutoscaleWindowKey = "autoscaling.knative.dev/window"
)

var DefaultValues = map[string]string{
	KnativeAutoscaleWindowKey: "60s",
	KnativeAutoscaleMinKey:    "2",
	KnativeAutoscaleMaxKey:    "10",
}

var TargetDefaultValues = map[string]string{
	"rps":         "200",
	"cpu":         "80",
	"memory":      "70",
	"concurrency": "10",
}

var KPAMetrics = []string{"rps", "concurrency"}

//  This function takes a Capp and a Knative Service and sets the autoscaler annotations based on the Capp's ScaleMetric.
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
	autoScaleAnnotations[KnativeAutoscaleMaxKey] = DefaultValues[KnativeAutoscaleMaxKey]
	autoScaleAnnotations[KnativeAutoscaleMinKey] = DefaultValues[KnativeAutoscaleMinKey]

	return autoScaleAnnotations
}

// Determines the autoscaling class based on the metric provided. Returns "kpa.autoscaling.knative.dev" if the metric is in KPAMetrics, "hpa.autoscaling.knative.dev" otherwise.
func getAutoScaleClassByMetric(metric string) string {
	if slices.Contains(KPAMetrics, metric) {
		return "kpa.autoscaling.knative.dev"
	}
	return "hpa.autoscaling.knative.dev"
}
