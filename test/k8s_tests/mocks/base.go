package mocks

import (
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
)

var (
	NsName         = "capp-e2e-tests"
	RPSScaleMetric = "rps"
)

func CreateBaseCapp() *rcsv1alpha1.Capp {
	return &rcsv1alpha1.Capp{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Capp",
			APIVersion: "rcs.dana.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "capp-default-test",
			Namespace: NsName,
		},
		Spec: rcsv1alpha1.CappSpec{
			ConfigurationSpec: knativev1.ConfigurationSpec{
				Template: knativev1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.Time{},
					},
					Spec: knativev1.RevisionSpec{
						PodSpec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Env: []corev1.EnvVar{
										{
											Name:  "APP_NAME",
											Value: "capp-default-test",
										},
									},
									Image:     "quay.io/danateamorg/example-python-app:v1-flask",
									Name:      "capp-default-test",
									Resources: corev1.ResourceRequirements{},
								},
							},
						},
					},
				},
			},
			RouteSpec: rcsv1alpha1.RouteSpec{},
			LogSpec:   rcsv1alpha1.LogSpec{},
		},
	}
}
