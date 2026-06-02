package autoscale

import (
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kautoscaling "knative.dev/serving/pkg/apis/autoscaling"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
)

func TestSetAutoScaler(t *testing.T) {
	tests := []struct {
		name     string
		metric   string
		expected map[string]string
	}{
		{
			name:   "uses HPA class and cpu target from autoscale config",
			metric: kautoscaling.CPU,
			expected: map[string]string{
				kautoscaling.ClassAnnotationKey:  kautoscaling.HPA,
				kautoscaling.MetricAnnotationKey: kautoscaling.CPU,
				kautoscaling.TargetAnnotationKey: "80",
				kautoscaling.ActivationScaleKey:  "3",
			},
		},
		{
			name:   "uses KPA class and rps target from autoscale config",
			metric: kautoscaling.RPS,
			expected: map[string]string{
				kautoscaling.ClassAnnotationKey:  kautoscaling.KPA,
				kautoscaling.MetricAnnotationKey: kautoscaling.RPS,
				kautoscaling.TargetAnnotationKey: "200",
				kautoscaling.ActivationScaleKey:  "3",
			},
		},
		{
			name:   "uses HPA class and memory target from autoscale config",
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
			annotations := SetAutoScaler(capp, cappv1alpha1.AutoscaleConfig{
				RPS: 200, CPU: 80, Memory: 70, Concurrency: 10, ActivationScale: 3,
			})
			require.Equal(t, tt.expected, annotations)
		})
	}

	scaleTests := []struct {
		name                  string
		minReplicas           int
		templateAnnotations   map[string]string
		expectMinScale        string
		expectActivationScale string
	}{
		{
			name:        "applies min replicas from spec and strips conflicting template scale annotations",
			minReplicas: 5,
			templateAnnotations: map[string]string{
				kautoscaling.ActivationScaleKey:    "2",
				kautoscaling.MinScaleAnnotationKey: "2",
			},
			expectMinScale: "5",
		},
		{
			name:        "drops min-scale and uses activation scale from config when min replicas is zero",
			minReplicas: 0,
			templateAnnotations: map[string]string{
				kautoscaling.MinScaleAnnotationKey: "3",
				kautoscaling.ActivationScaleKey:    "1",
			},
			expectActivationScale: "3",
		},
	}

	for _, tt := range scaleTests {
		t.Run(tt.name, func(t *testing.T) {
			capp := cappv1alpha1.Capp{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: cappv1alpha1.CappSpec{
					ScaleSpec: cappv1alpha1.ScaleSpec{
						Metric:      kautoscaling.CPU,
						MinReplicas: tt.minReplicas,
					},
					ConfigurationSpec: knativev1.ConfigurationSpec{
						Template: knativev1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{Annotations: tt.templateAnnotations},
						},
					},
				},
			}

			annotations := SetAutoScaler(capp, cappv1alpha1.AutoscaleConfig{
				RPS: 200, CPU: 80, Memory: 70, Concurrency: 10, ActivationScale: 3,
			})

			if tt.expectMinScale == "" {
				_, hasMinScale := annotations[kautoscaling.MinScaleAnnotationKey]
				require.False(t, hasMinScale)
			} else {
				require.Equal(t, tt.expectMinScale, annotations[kautoscaling.MinScaleAnnotationKey])
			}

			if tt.expectActivationScale == "" {
				_, hasActivationScale := annotations[kautoscaling.ActivationScaleKey]
				require.False(t, hasActivationScale)
			} else {
				require.Equal(t, tt.expectActivationScale, annotations[kautoscaling.ActivationScaleKey])
			}
		})
	}
}
