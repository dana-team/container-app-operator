package autoscale

import (
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetAutoScaler(t *testing.T) {
	exampleCapp := cappv1alpha1.Capp{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: cappv1alpha1.CappSpec{
			ScaleMetric: "cpu",
		},
	}
	exampleCappCpuExpected := map[string]string{
		"autoscaling.knative.dev/class":  "hpa.autoscaling.knative.dev",
		"autoscaling.knative.dev/metric": "cpu",
		"autoscaling.knative.dev/target": "80",
	}
	annotationsCpu := SetAutoScaler(exampleCapp, map[string]string{})
	assert.Equal(t, exampleCappCpuExpected, annotationsCpu)

	exampleCapp.Spec.ScaleMetric = "rps"
	exampleCappRpsExpected := map[string]string{
		"autoscaling.knative.dev/class":  "kpa.autoscaling.knative.dev",
		"autoscaling.knative.dev/metric": "rps",
		"autoscaling.knative.dev/target": "200",
	}
	annotationsRps := SetAutoScaler(exampleCapp, map[string]string{})
	assert.Equal(t, exampleCappRpsExpected, annotationsRps)
}
