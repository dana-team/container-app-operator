package webhooks

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func newCappMutator(t *testing.T, scheme *runtime.Scheme) *CappMutator {
	t.Helper()

	cfg := newCappConfig()
	cfg.Spec.DefaultResources = newDefaultResources()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cfg).
		Build()

	return &CappMutator{
		Client:  fakeClient,
		Decoder: admission.NewDecoder(scheme),
	}
}

func TestCappMutatorHandle(t *testing.T) {
	scheme := newScheme(t)
	mutator := newCappMutator(t, scheme)

	t.Run("skips mutation for status subresource", func(t *testing.T) {
		capp := newCapp("")
		raw, err := json.Marshal(capp)
		require.NoError(t, err)

		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				SubResource: "status",
				Object:      runtime.RawExtension{Raw: raw},
				Name:        cappName,
				Namespace:   nsName,
			},
		}

		resp := mutator.Handle(context.Background(), req)
		require.True(t, resp.Allowed)
		require.Empty(t, resp.Patches)
	})

	t.Run("returns error when CappConfig is missing", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		m := &CappMutator{
			Client:  fakeClient,
			Decoder: admission.NewDecoder(scheme),
		}

		capp := newCapp("")
		raw, err := json.Marshal(capp)
		require.NoError(t, err)

		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object:    runtime.RawExtension{Raw: raw},
				Name:      cappName,
				Namespace: nsName,
			},
		}

		resp := m.Handle(context.Background(), req)
		require.Equal(t, int32(http.StatusInternalServerError), resp.Result.Code)
	})

	t.Run("returns error on invalid object", func(t *testing.T) {
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Object: runtime.RawExtension{Raw: []byte(`{invalid`)},
				Name:   cappName,
			},
		}

		resp := mutator.Handle(context.Background(), req)
		require.Equal(t, int32(http.StatusBadRequest), resp.Result.Code)
	})

	t.Run("patches annotation and default resources", func(t *testing.T) {
		capp := &cappv1alpha1.Capp{}
		capp.Name = cappName
		capp.Namespace = nsName
		capp.Spec.ConfigurationSpec.Template.Spec.Containers = []corev1.Container{
			{Name: testContainerName},
		}

		raw, err := json.Marshal(capp)
		require.NoError(t, err)

		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object:    runtime.RawExtension{Raw: raw},
				Name:      cappName,
				Namespace: nsName,
				UserInfo:  authenticationv1.UserInfo{Username: testUsername},
			},
		}

		resp := mutator.Handle(context.Background(), req)
		require.True(t, resp.Allowed)
		require.NotEmpty(t, resp.Patches)

		patched := patchedCapp(t, raw, resp.Patches)
		require.Equal(t, testUsername, patched.Annotations[lastUpdatedByAnnotationKey])

		container := patched.Spec.ConfigurationSpec.Template.Spec.Containers[0]
		require.Equal(t, resource.MustParse("100m"), container.Resources.Requests[corev1.ResourceCPU])
		require.Equal(t, resource.MustParse("128Mi"), container.Resources.Requests[corev1.ResourceMemory])
		require.Equal(t, resource.MustParse("500m"), container.Resources.Limits[corev1.ResourceCPU])
		require.Equal(t, resource.MustParse("512Mi"), container.Resources.Limits[corev1.ResourceMemory])
	})

	t.Run("preserves existing container resources", func(t *testing.T) {
		capp := &cappv1alpha1.Capp{}
		capp.Name = cappName
		capp.Namespace = nsName
		capp.Spec.ConfigurationSpec.Template.Spec.Containers = []corev1.Container{
			{
				Name: testContainerName,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("200m"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				},
			},
		}

		raw, err := json.Marshal(capp)
		require.NoError(t, err)

		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object:    runtime.RawExtension{Raw: raw},
				Name:      cappName,
				Namespace: nsName,
				UserInfo:  authenticationv1.UserInfo{Username: testUsername},
			},
		}

		resp := mutator.Handle(context.Background(), req)
		require.True(t, resp.Allowed)

		patched := patchedCapp(t, raw, resp.Patches)
		container := patched.Spec.ConfigurationSpec.Template.Spec.Containers[0]
		require.Equal(t, resource.MustParse("200m"), container.Resources.Requests[corev1.ResourceCPU])
		require.Equal(t, resource.MustParse("128Mi"), container.Resources.Requests[corev1.ResourceMemory])
		require.Equal(t, resource.MustParse("500m"), container.Resources.Limits[corev1.ResourceCPU])
		require.Equal(t, resource.MustParse("1Gi"), container.Resources.Limits[corev1.ResourceMemory])
	})
}

func TestMutateAnnotations(t *testing.T) {
	t.Run("sets annotation on nil annotations map", func(t *testing.T) {
		capp := &cappv1alpha1.Capp{}
		mutateAnnotations(capp, testUsername)

		require.Equal(t, testUsername, capp.Annotations[lastUpdatedByAnnotationKey])
	})

	t.Run("appends annotation without clobbering existing keys", func(t *testing.T) {
		capp := &cappv1alpha1.Capp{}
		capp.Annotations = map[string]string{"existing-key": "existing-value"}
		mutateAnnotations(capp, testUsername)

		require.Equal(t, testUsername, capp.Annotations[lastUpdatedByAnnotationKey])
		require.Equal(t, "existing-value", capp.Annotations["existing-key"])
	})
}

func TestMutateResources(t *testing.T) {
	defaults := newDefaultResources()

	t.Run("skips mutation when default resources are zero-valued", func(t *testing.T) {
		capp := &cappv1alpha1.Capp{}
		capp.Spec.ConfigurationSpec.Template.Spec.Containers = []corev1.Container{
			{Name: testContainerName},
		}

		mutateResources(capp, corev1.ResourceRequirements{})

		container := capp.Spec.ConfigurationSpec.Template.Spec.Containers[0]
		require.Nil(t, container.Resources.Requests)
		require.Nil(t, container.Resources.Limits)
	})

	t.Run("injects defaults when container has nil resource lists", func(t *testing.T) {
		capp := &cappv1alpha1.Capp{}
		capp.Spec.ConfigurationSpec.Template.Spec.Containers = []corev1.Container{
			{Name: testContainerName},
		}

		mutateResources(capp, defaults)

		container := capp.Spec.ConfigurationSpec.Template.Spec.Containers[0]
		require.Equal(t, resource.MustParse("100m"), container.Resources.Requests[corev1.ResourceCPU])
		require.Equal(t, resource.MustParse("128Mi"), container.Resources.Requests[corev1.ResourceMemory])
		require.Equal(t, resource.MustParse("500m"), container.Resources.Limits[corev1.ResourceCPU])
		require.Equal(t, resource.MustParse("512Mi"), container.Resources.Limits[corev1.ResourceMemory])
	})

	t.Run("preserves existing values and fills missing ones", func(t *testing.T) {
		capp := &cappv1alpha1.Capp{}
		capp.Spec.ConfigurationSpec.Template.Spec.Containers = []corev1.Container{
			{
				Name: testContainerName,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("250m"),
					},
				},
			},
		}

		mutateResources(capp, defaults)

		container := capp.Spec.ConfigurationSpec.Template.Spec.Containers[0]
		require.Equal(t, resource.MustParse("250m"), container.Resources.Requests[corev1.ResourceCPU])
		require.Equal(t, resource.MustParse("128Mi"), container.Resources.Requests[corev1.ResourceMemory])
		require.Equal(t, resource.MustParse("500m"), container.Resources.Limits[corev1.ResourceCPU])
		require.Equal(t, resource.MustParse("512Mi"), container.Resources.Limits[corev1.ResourceMemory])
	})

	t.Run("handles multiple containers independently", func(t *testing.T) {
		capp := &cappv1alpha1.Capp{}
		capp.Spec.ConfigurationSpec.Template.Spec.Containers = []corev1.Container{
			{
				Name: "first",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
			},
			{Name: "second"},
		}

		mutateResources(capp, defaults)

		first := capp.Spec.ConfigurationSpec.Template.Spec.Containers[0]
		require.Equal(t, resource.MustParse("1"), first.Resources.Requests[corev1.ResourceCPU])
		require.Equal(t, resource.MustParse("2Gi"), first.Resources.Limits[corev1.ResourceMemory])

		second := capp.Spec.ConfigurationSpec.Template.Spec.Containers[1]
		require.Equal(t, resource.MustParse("100m"), second.Resources.Requests[corev1.ResourceCPU])
		require.Equal(t, resource.MustParse("512Mi"), second.Resources.Limits[corev1.ResourceMemory])
	})
}
