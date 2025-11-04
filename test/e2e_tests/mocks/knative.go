package mocks

import (
	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
)

// CreateRevisionObject returns an empty KnativeRevision object.
func CreateRevisionObject(name string) *knativev1.Revision {
	return &knativev1.Revision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testconsts.NSName,
		},
	}
}

// CreateEnvVarObject returns an EnvVar object.
func CreateEnvVarObject(refName string) *[]corev1.EnvVar {
	return &[]corev1.EnvVar{
		{
			Name: testconsts.PassEnvName,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: refName,
					},
					Key: testconsts.SecretKey,
				},
			},
		},
	}
}

// CreateKnativeServiceObject returns an empty KSVC object.
func CreateKnativeServiceObject(name string) *knativev1.Service {
	return &knativev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testconsts.NSName,
		},
	}
}
