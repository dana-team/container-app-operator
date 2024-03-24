package mocks

import (
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
)

const (
	CappName       = "capp-default-test"
	NsName         = "capp-e2e-tests"
	RPSScaleMetric = "rps"
	SecretKey      = "extra"
	SecretValue    = "YmFyCg=="
	ControllerNS   = "capp-operator-system"
	AutoScaleCM    = "autoscale-defaults"
)

func CreateBaseCapp() *cappv1alpha1.Capp {
	return &cappv1alpha1.Capp{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Capp",
			APIVersion: "rcs.dana.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      CappName,
			Namespace: NsName,
		},
		Spec: cappv1alpha1.CappSpec{
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
									Image:     "ghcr.io/knative/autoscale-go:latest",
									Name:      "capp-default-test",
									Resources: corev1.ResourceRequirements{},
								},
							},
						},
					},
				},
			},
			RouteSpec: cappv1alpha1.RouteSpec{},
			LogSpec:   cappv1alpha1.LogSpec{},
		},
	}
}

func CreateSecretObject(secretName string) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: NsName,
		},
		Type: "Opaque",
		Data: map[string][]byte{SecretKey: []byte(SecretValue)},
	}
}

func CreateConfigMapObject(namespace string, name string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}
}
