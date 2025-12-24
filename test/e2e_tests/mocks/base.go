package mocks

import (
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
)

// CreateBaseCapp is responsible for making the most lean version of Capp, so we can manipulate it in the tests.
func CreateBaseCapp() *cappv1alpha1.Capp {
	return &cappv1alpha1.Capp{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Capp",
			APIVersion: "rcs.dana.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      testconsts.CappName,
			Namespace: testconsts.NSName,
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
											Value: testconsts.CappName,
										},
									},
									Image:     testconsts.CappBaseImage,
									Name:      testconsts.CappName,
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

// CreateSecretObject creates a basic secret.
func CreateSecretObject(name string) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testconsts.NSName,
		},
		Type: "Opaque",
		Data: map[string][]byte{testconsts.SecretKey: []byte(testconsts.SecretValue)},
	}
}

// CreateRole creates a role with the specified name and rules.
func CreateRole(name string, rules []rbacv1.PolicyRule) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testconsts.NSName,
		},
		Rules: rules,
	}
}

// CreateRoleBinding creates a role binding with the specified name, role reference, and subjects.
func CreateRoleBinding(name string, roleRef rbacv1.RoleRef, subjects []rbacv1.Subject) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testconsts.NSName,
		},
		RoleRef:  roleRef,
		Subjects: subjects,
	}
}

// CreateServiceAccount creates a service account with the specified name in the specified namespace.
func CreateServiceAccount(name, namespace string) *corev1.ServiceAccount {
	if namespace == "" {
		namespace = testconsts.NSName
	}

	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// CreateBaseCappConfig is responsible for making the most lean version of CappConfig, so we can manipulate it in the tests.
func CreateBaseCappConfig() *cappv1alpha1.CappConfig {
	return &cappv1alpha1.CappConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CappConfig",
			APIVersion: "rcs.dana.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      testconsts.CappConfigName,
			Namespace: testconsts.NSName,
		},
		Spec: cappv1alpha1.CappConfigSpec{
			DNSConfig: cappv1alpha1.DNSConfig{
				Zone:     "example.com",
				CNAME:    "cname.example.com",
				Provider: "mock-dns",
				Issuer:   "letsencrypt",
			},
			AutoscaleConfig: cappv1alpha1.AutoscaleConfig{
				RPS:             100,
				CPU:             50,
				Memory:          50,
				Concurrency:     10,
				ActivationScale: 1,
			},
			DefaultResources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
			},
			AllowedHostnamePatterns: []string{".*"},
		},
	}
}
