package mocks

import (
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/test/k8s_tests/utils"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
)

var (
	NsName         = "capp-e2e-tests"
	RPSScaleMetric = "rps"
	SecretKey      = "extra"
	SecretValue    = "YmFyCg=="
	passEnvName    = "PASSWORD"
)

func CreateBaseCapp() *rcsv1alpha1.Capp {
	return &rcsv1alpha1.Capp{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Capp",
			APIVersion: "rcs.dana.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.CappName,
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
									Image:     "ghcr.io/knative/autoscale-go:latest",
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

func CreateSecretObject(secretName string) *v1.Secret {
	return &v1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: NsName,
		},
		Type: "Opaque",
		Data: map[string][]byte{SecretKey: []byte(SecretValue)},
	}
}

func CreateDomainMappingObject(domainMappingName string) *knativev1beta1.DomainMapping {
	return &knativev1beta1.DomainMapping{
		ObjectMeta: metav1.ObjectMeta{
			Name:      domainMappingName,
			Namespace: NsName,
		},
	}
}

func CreateRevisionObject(revisionName string) *knativev1.Revision {
	return &knativev1.Revision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      revisionName,
			Namespace: NsName,
		},
	}
}

func CreateKnativeServiceObject(knativeServiceName string) *knativev1.Service {
	return &knativev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      knativeServiceName,
			Namespace: NsName,
		},
	}
}

func CreateEnvVarObject(refName string) *[]corev1.EnvVar {
	return &[]corev1.EnvVar{
		{
			Name: passEnvName,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: refName,
					},
					Key: SecretKey,
				},
			},
		},
	}
}
