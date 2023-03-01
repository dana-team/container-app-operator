package utils

import (
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"k8s.io/utils/strings/slices"

	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
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

func SetAutoScaler(capp rcsv1alpha1.Capp, knativeService knativev1.Service) map[string]string {
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

	knativeService.Spec.Template.ObjectMeta.Annotations = autoScaleAnnotations
	return autoScaleAnnotations
}

func getAutoScaleClassByMetric(metric string) string {
	if slices.Contains(KPAMetrics, metric) {
		return "kpa.autoscaling.knative.dev"
	}
	return "hpa.autoscaling.knative.dev"
}
