package autoscale

import (
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kautoscaling "knative.dev/serving/pkg/apis/autoscaling"
)

func TestSetAutoScaler(t *testing.T) {
	tests := []struct {
		name     string
		metric   string
		expected map[string]string
	}{
		{
			name:   "uses HPA class and default cpu target for cpu metric",
			metric: kautoscaling.CPU,
			expected: map[string]string{
				kautoscaling.ClassAnnotationKey:  kautoscaling.HPA,
				kautoscaling.MetricAnnotationKey: kautoscaling.CPU,
				kautoscaling.TargetAnnotationKey: "80",
				kautoscaling.ActivationScaleKey:  "3",
			},
		},
		{
			name:   "uses KPA class and default rps target for rps metric",
			metric: kautoscaling.RPS,
			expected: map[string]string{
				kautoscaling.ClassAnnotationKey:  kautoscaling.KPA,
				kautoscaling.MetricAnnotationKey: kautoscaling.RPS,
				kautoscaling.TargetAnnotationKey: "200",
				kautoscaling.ActivationScaleKey:  "3",
			},
		},
		{
			name:   "uses HPA class and default memory target for memory metric",
			metric: kautoscaling.Memory,
			expected: map[string]string{
				kautoscaling.ClassAnnotationKey:  kautoscaling.HPA,
				kautoscaling.MetricAnnotationKey: kautoscaling.Memory,
				kautoscaling.TargetAnnotationKey: "70",
				kautoscaling.ActivationScaleKey:  "3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capp := cappv1alpha1.Capp{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: cappv1alpha1.CappSpec{
					ScaleSpec: cappv1alpha1.ScaleSpec{
						Metric: tt.metric,
					},
				},
			}
			annotations := SetAutoScaler(capp, cappv1alpha1.AutoscaleConfig{})
			assert.Equal(t, tt.expected, annotations)
		})
	}
}
