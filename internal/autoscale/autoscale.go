package autoscale

import (
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/utils"
	"k8s.io/utils/strings/slices"
)

const (
	KnativeMetricKey          = "autoscaling.knative.dev/metric"
	KnativeAutoscaleClassKey  = "autoscaling.knative.dev/class"
	KnativeAutoscaleTargetKey = "autoscaling.knative.dev/target"
	AutoScalerSubString       = "autoscaling"
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
func SetAutoScaler(capp cappv1alpha1.Capp, defaults map[string]string) map[string]string {
	scaleMetric := capp.Spec.ScaleMetric
	autoScaleAnnotations := make(map[string]string)
	autoScaleDefaults := defaults
	if scaleMetric == "" {
		return autoScaleAnnotations
	}
	if len(defaults) == 0 {
		autoScaleDefaults = TargetDefaultValues
	}
	givenAutoScaleAnnotation := utils.FilterMap(capp.Spec.ConfigurationSpec.Template.Annotations, AutoScalerSubString)
	autoScaleAnnotations[KnativeAutoscaleClassKey] = getAutoScaleClassByMetric(scaleMetric)
	autoScaleAnnotations[KnativeMetricKey] = scaleMetric
	autoScaleAnnotations[KnativeAutoscaleTargetKey] = autoScaleDefaults[scaleMetric]
	autoScaleAnnotations = utils.MergeMaps(autoScaleAnnotations, givenAutoScaleAnnotation)

	return autoScaleAnnotations
}

// Determines the autoscaling class based on the metric provided. Returns "kpa.autoscaling.knative.dev" if the metric is in KPAMetrics, "hpa.autoscaling.knative.dev" otherwise.
func getAutoScaleClassByMetric(metric string) string {
	if slices.Contains(KPAMetrics, metric) {
		return "kpa.autoscaling.knative.dev"
	}
	return "hpa.autoscaling.knative.dev"
}
