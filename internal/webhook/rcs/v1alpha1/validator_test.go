package webhooks

import (
	"context"
	"encoding/json"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	_ "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers/eventsources"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestCappValidator_Handle(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(cappv1alpha1.AddToScheme(scheme))

	decoder := admission.NewDecoder(scheme)

	tests := []struct {
		name        string
		capp        *cappv1alpha1.Capp
		expectAllow bool
		expectMsg   string
	}{
		{
			name: "Allow capp without sources",
			capp: &cappv1alpha1.Capp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-capp",
					Namespace: "test-ns",
				},
				Spec: cappv1alpha1.CappSpec{
					ScaleSpec: cappv1alpha1.ScaleSpec{
						Metric: "cpu",
					},
					RouteSpec: cappv1alpha1.RouteSpec{
						Hostname: "valid-hostname.com",
					},
					LogSpec: cappv1alpha1.LogSpec{},
				},
			},
			expectAllow: true,
		},
		{
			name: "Allow Capp with valid scaleDelaySeconds",
			capp: &cappv1alpha1.Capp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-capp",
					Namespace: "test-ns",
				},
				Spec: cappv1alpha1.CappSpec{
					ScaleSpec: cappv1alpha1.ScaleSpec{
						Metric:            "cpu",
						ScaleDelaySeconds: 50,
					},
				},
			},
			expectAllow: true,
		},
		{
			name: "Deny Capp with invalid scaleDelaySeconds",
			capp: &cappv1alpha1.Capp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-capp",
					Namespace: "test-ns",
				},
				Spec: cappv1alpha1.CappSpec{
					ScaleSpec: cappv1alpha1.ScaleSpec{
						Metric:            "cpu",
						ScaleDelaySeconds: 150,
					},
				},
			},
			expectAllow: false,
			expectMsg:   "must be less than or equal to global max scale delay",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Add default capp config
			cappConfig := &cappv1alpha1.CappConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      utils.CappConfigName,
					Namespace: utils.CappNS,
				},
				Spec: cappv1alpha1.CappConfigSpec{
					AllowedHostnamePatterns: []cappv1alpha1.HostnamePattern{{Match: ".*"}},
					AutoscaleConfig: cappv1alpha1.AutoscaleConfig{
						MinReplicasLimit: 10,
						MaxScaleDelay:    100,
					},
				},
			}

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cappConfig).Build()

			validator := &CappValidator{
				Client:  fakeClient,
				Decoder: decoder,
			}

			raw, err := json.Marshal(tc.capp)
			if err != nil {
				t.Fatal(err)
			}

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: raw,
					},
					Name:      "test-capp",
					Namespace: "test-ns",
				},
			}

			resp := validator.Handle(context.Background(), req)
			assert.Equal(t, tc.expectAllow, resp.Allowed, "Expected allowed: %v, got: %v. Result: %v", tc.expectAllow, resp.Allowed, resp.Result)
			if !tc.expectAllow && tc.expectMsg != "" {
				assert.Contains(t, resp.Result.Message, tc.expectMsg)
			}
		})
	}
}

func TestValidateNFSVolumeMounts(t *testing.T) {
	invalidNFSVolumesMsg := "invalid nfsVolumes"
	mustBeMountedMsg := "must be mounted by at least one container"
	nfsVolumeName := "shared-data"

	tests := []struct {
		name            string
		nfsVolumes      []cappv1alpha1.NFSVolume
		containers      []corev1.Container
		wantErrContains []string
	}{
		{
			name: "allows when no nfs volumes are defined",
		},
		{
			name: "allows when all nfs volumes are mounted",
			nfsVolumes: []cappv1alpha1.NFSVolume{
				{Name: nfsVolumeName},
			},
			containers: []corev1.Container{
				{
					Name: "main",
					VolumeMounts: []corev1.VolumeMount{
						{Name: nfsVolumeName, MountPath: "/mnt/shared-data"},
					},
				},
			},
		},
		{
			name: "reports missing mount when nfs volume is not mounted",
			nfsVolumes: []cappv1alpha1.NFSVolume{
				{Name: nfsVolumeName},
			},
			containers: []corev1.Container{
				{
					Name: "main",
					VolumeMounts: []corev1.VolumeMount{
						{Name: "other-volume", MountPath: "/mnt/other-volume"},
					},
				},
			},
			wantErrContains: []string{
				invalidNFSVolumesMsg,
				nfsVolumeName,
				mustBeMountedMsg,
			},
		},
		{
			name: "reports missing volumes",
			nfsVolumes: []cappv1alpha1.NFSVolume{
				{Name: "mounted"},
				{Name: "z-data"},
				{Name: "a-data"},
				{Name: "a-data"},
			},
			containers: []corev1.Container{
				{
					Name: "mounted",
					VolumeMounts: []corev1.VolumeMount{
						{Name: "mounted", MountPath: "/mnt/mounted"},
					},
				},
			},
			wantErrContains: []string{
				invalidNFSVolumesMsg,
				"a-data",
				"z-data",
				mustBeMountedMsg,
			},
		},
	}

	for _, tc := range tests {

		t.Run(tc.name, func(t *testing.T) {
			capp := cappv1alpha1.Capp{
				Spec: cappv1alpha1.CappSpec{
					VolumesSpec: cappv1alpha1.VolumesSpec{
						NFSVolumes: tc.nfsVolumes,
					},
				},
			}
			capp.Spec.ConfigurationSpec.Template.Spec.Containers = tc.containers

			err := validateNFSVolumeMounts(capp)
			if len(tc.wantErrContains) == 0 {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			for _, expectedSubstring := range tc.wantErrContains {
				require.Contains(t, err.Error(), expectedSubstring)
			}
		})
	}
}

func TestValidateEventSources(t *testing.T) {
	tests := []struct {
		name            string
		sources         []cappv1alpha1.SourceConfiguration
		wantErrContains []string
	}{
		{
			name: "allows empty sources list",
		},
		{
			name: "allows unique source names",
			sources: []cappv1alpha1.SourceConfiguration{
				{Name: "ping-a", PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{}},
				{Name: "ping-b", PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{}},
			},
		},
		{
			name: "rejects duplicate source names",
			sources: []cappv1alpha1.SourceConfiguration{
				{Name: "ping-a", PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{}},
				{Name: "ping-a", PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{}},
			},
			wantErrContains: []string{
				"spec.eventSourcesSpec.sources",
				"duplicate",
				"ping-a",
			},
		},
		{
			name: "rejects source with no configuration",
			sources: []cappv1alpha1.SourceConfiguration{
				{Name: "ping-a"},
			},
			wantErrContains: []string{
				"spec.eventSourcesSpec.sources[0]",
				"ping-a",
				"must specify at least one source configuration",
			},
		},
		{
			name: "allows source with ping configuration",
			sources: []cappv1alpha1.SourceConfiguration{
				{Name: "ping-a", PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{Schedule: "* * * * *"}},
			},
		},
		{
			name: "rejects source with invalid cron schedule",
			sources: []cappv1alpha1.SourceConfiguration{
				{Name: "ping-a", PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{Schedule: "not-a-cron"}},
			},
			wantErrContains: []string{"schedule"},
		},
		{
			name: "rejects source with invalid JSON data",
			sources: []cappv1alpha1.SourceConfiguration{
				{Name: "ping-a", PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{Schedule: "* * * * *", Data: "not-json{"}},
			},
			wantErrContains: []string{"data"},
		},
		{
			name: "allows source with valid schedule and valid JSON",
			sources: []cappv1alpha1.SourceConfiguration{
				{Name: "ping-a", PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{Schedule: "*/5 * * * *", Data: `{"key":"value"}`}},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			capp := cappv1alpha1.Capp{
				Spec: cappv1alpha1.CappSpec{
					EventSourcesSpec: cappv1alpha1.EventSourcesSpec{
						Sources: tc.sources,
					},
				},
			}

			err := validateEventSources(capp)
			if len(tc.wantErrContains) == 0 {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			for _, s := range tc.wantErrContains {
				assert.Contains(t, err.Error(), s)
			}
		})
	}
}
