package webhooks

import (
	"context"
	"encoding/json"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	knativeautoscaling "knative.dev/serving/pkg/apis/autoscaling"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	cappName               = "test-capp"
	nsName                 = "test-ns"
	mountedNFSVolumeName   = "mounted"
	unmountedNFSVolumeName = "a-data"
	eventSourceName        = "ping-a"
	unchangedHostname      = "same.example.com"
	oldHostname            = "old.example.com"
	newHostname            = "new.example.com"
	elasticHost            = "https://elastic.example.com"
	elasticIndex           = "my-index"
)

func TestCappValidatorHandle(t *testing.T) {
	scheme := newScheme(t)
	decoder := admission.NewDecoder(scheme)

	tests := []struct {
		name        string
		operation   admissionv1.Operation
		capp        *cappv1alpha1.Capp
		oldCapp     *cappv1alpha1.Capp
		expectAllow bool
		expectMsg   string
	}{
		{
			name:      "Allow capp without sources",
			operation: admissionv1.Create,
			capp: &cappv1alpha1.Capp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cappName,
					Namespace: nsName,
				},
				Spec: cappv1alpha1.CappSpec{
					ScaleSpec: cappv1alpha1.ScaleSpec{
						Metric: knativeautoscaling.CPU,
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
			name:      "Allow Capp with valid scaleDelaySeconds",
			operation: admissionv1.Create,
			capp: &cappv1alpha1.Capp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cappName,
					Namespace: nsName,
				},
				Spec: cappv1alpha1.CappSpec{
					ScaleSpec: cappv1alpha1.ScaleSpec{
						Metric:            knativeautoscaling.CPU,
						ScaleDelaySeconds: 50,
					},
				},
			},
			expectAllow: true,
		},
		{
			name:      "Deny Capp with invalid scaleDelaySeconds",
			operation: admissionv1.Create,
			capp: &cappv1alpha1.Capp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cappName,
					Namespace: nsName,
				},
				Spec: cappv1alpha1.CappSpec{
					ScaleSpec: cappv1alpha1.ScaleSpec{
						Metric:            knativeautoscaling.CPU,
						ScaleDelaySeconds: 150,
					},
				},
			},
			expectAllow: false,
			expectMsg:   "must be less than or equal to global max scale delay",
		},
		{
			name:        "allows update when hostname remains unchanged",
			operation:   admissionv1.Update,
			capp:        newCapp(unchangedHostname),
			oldCapp:     newCapp(unchangedHostname),
			expectAllow: true,
		},
		{
			name:        "rejects update when hostname changes",
			operation:   admissionv1.Update,
			capp:        newCapp(newHostname),
			oldCapp:     newCapp(oldHostname),
			expectAllow: false,
			expectMsg:   "spec.routeSpec.hostname is immutable once set",
		},
		{
			name:      "Deny Capp when PasswordSecret does not exist",
			operation: admissionv1.Create,
			capp: &cappv1alpha1.Capp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cappName,
					Namespace: nsName,
				},
				Spec: cappv1alpha1.CappSpec{
					ScaleSpec: cappv1alpha1.ScaleSpec{
						Metric: knativeautoscaling.CPU,
					},
					LogSpec: cappv1alpha1.LogSpec{
						Type:           cappv1alpha1.LogTypeElastic,
						Host:           elasticHost,
						Index:          elasticIndex,
						User:           elasticSecretKey,
						PasswordSecret: "missing-secret",
					},
				},
			},
			expectAllow: false,
			expectMsg:   "secret \"missing-secret\" not found",
		},
		{
			name:      "Allow Capp when PasswordSecret exists",
			operation: admissionv1.Create,
			capp: &cappv1alpha1.Capp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cappName,
					Namespace: nsName,
				},
				Spec: cappv1alpha1.CappSpec{
					ScaleSpec: cappv1alpha1.ScaleSpec{
						Metric: knativeautoscaling.CPU,
					},
					LogSpec: cappv1alpha1.LogSpec{
						Type:           cappv1alpha1.LogTypeElastic,
						Host:           elasticHost,
						Index:          elasticIndex,
						User:           elasticSecretKey,
						PasswordSecret: "existing-secret",
					},
				},
			},
			expectAllow: true,
		},
		{
			name:      "Deny Capp when PasswordSecret exists but missing required key",
			operation: admissionv1.Create,
			capp: &cappv1alpha1.Capp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cappName,
					Namespace: nsName,
				},
				Spec: cappv1alpha1.CappSpec{
					ScaleSpec: cappv1alpha1.ScaleSpec{
						Metric: knativeautoscaling.CPU,
					},
					LogSpec: cappv1alpha1.LogSpec{
						Type:           cappv1alpha1.LogTypeElastic,
						Host:           elasticHost,
						Index:          elasticIndex,
						User:           elasticSecretKey,
						PasswordSecret: "bad-key-secret",
					},
				},
			},
			expectAllow: false,
			expectMsg:   "missing required key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			validator := newCappValidator(t, scheme, decoder)

			raw, err := json.Marshal(tc.capp)
			if err != nil {
				t.Fatal(err)
			}

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: tc.operation,
					Object: runtime.RawExtension{
						Raw: raw,
					},
					Name:      cappName,
					Namespace: nsName,
				},
			}
			if tc.operation == admissionv1.Update {
				oldRaw, marshalErr := json.Marshal(tc.oldCapp)
				require.NoError(t, marshalErr)
				req.OldObject = runtime.RawExtension{Raw: oldRaw}
			}

			resp := validator.Handle(context.Background(), req)
			assert.Equal(t, tc.expectAllow, resp.Allowed, "Expected allowed: %v, got: %v. Result: %v", tc.expectAllow, resp.Allowed, resp.Result)
			if !tc.expectAllow && tc.expectMsg != "" {
				assert.Contains(t, resp.Result.Message, tc.expectMsg)
			}
		})
	}

}

func TestValidateHostnameImmutability(t *testing.T) {
	tests := []struct {
		name        string
		operation   admissionv1.Operation
		oldHostname string
		newHostname string
		wantErr     bool
	}{
		{
			name:      "allows create",
			operation: admissionv1.Create,
		},
		{
			name:        "allows update when hostname remains unchanged",
			operation:   admissionv1.Update,
			oldHostname: unchangedHostname,
			newHostname: unchangedHostname,
		},
		{
			name:        "allows update when hostname is set from empty",
			operation:   admissionv1.Update,
			newHostname: newHostname,
		},
		{
			name:        "rejects update when hostname changes",
			operation:   admissionv1.Update,
			oldHostname: oldHostname,
			newHostname: newHostname,
			wantErr:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			capp := *newCapp(tc.newHostname)
			oldCapp := newCapp(tc.oldHostname)

			err := validateHostnameImmutability(tc.operation, capp, oldCapp)
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			require.Contains(t, err.Error(), "spec.routeSpec.hostname")
			require.Contains(t, err.Error(), "immutable")
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
				{Name: mountedNFSVolumeName},
				{Name: "z-data"},
				{Name: unmountedNFSVolumeName},
				{Name: unmountedNFSVolumeName},
			},
			containers: []corev1.Container{
				{
					Name: mountedNFSVolumeName,
					VolumeMounts: []corev1.VolumeMount{
						{Name: mountedNFSVolumeName, MountPath: "/mnt/mounted"},
					},
				},
			},
			wantErrContains: []string{
				invalidNFSVolumesMsg,
				unmountedNFSVolumeName,
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
	ctx := context.Background()
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
				{Name: eventSourceName, PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{}},
				{Name: "ping-b", PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{}},
			},
		},
		{
			name: "rejects duplicate source names",
			sources: []cappv1alpha1.SourceConfiguration{
				{Name: eventSourceName, PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{}},
				{Name: eventSourceName, PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{}},
			},
			wantErrContains: []string{
				"spec.eventSourcesSpec.sources",
				"duplicate",
				eventSourceName,
			},
		},
		{
			name: "rejects source with no configuration",
			sources: []cappv1alpha1.SourceConfiguration{
				{Name: eventSourceName},
			},
			wantErrContains: []string{
				"spec.eventSourcesSpec.sources[0]",
				eventSourceName,
				"must specify at least one source configuration",
			},
		},
		{
			name: "allows source with ping configuration",
			sources: []cappv1alpha1.SourceConfiguration{
				{Name: eventSourceName, PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{Schedule: "* * * * * *"}},
			},
		},
		{
			name: "rejects source with invalid cron schedule",
			sources: []cappv1alpha1.SourceConfiguration{
				{Name: eventSourceName, PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{Schedule: "not-a-cron"}},
			},
			wantErrContains: []string{"schedule"},
		},
		{
			name: "rejects source with invalid JSON data",
			sources: []cappv1alpha1.SourceConfiguration{
				{Name: eventSourceName, PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{Schedule: "* * * * *", Data: "not-json{"}},
			},
			wantErrContains: []string{"data"},
		},
		{
			name: "allows source with valid schedule and valid JSON",
			sources: []cappv1alpha1.SourceConfiguration{
				{Name: eventSourceName, PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{Schedule: "*/5 * * * *", Data: `{"key":"value"}`}},
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

			err := validateEventSources(ctx, capp)
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

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(cappv1alpha1.AddToScheme(scheme))

	return scheme
}

func newCappValidator(t *testing.T, scheme *runtime.Scheme, decoder admission.Decoder) *CappValidator {
	t.Helper()

	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "existing-secret",
			Namespace: nsName,
		},
		Data: map[string][]byte{elasticSecretKey: []byte("password")},
	}

	badKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bad-key-secret",
			Namespace: nsName,
		},
		Data: map[string][]byte{"wrong-key": []byte("value")},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(newCappConfig(), existingSecret, badKeySecret).
		Build()

	return &CappValidator{
		Client:  fakeClient,
		Decoder: decoder,
	}
}

func newCappConfig() *cappv1alpha1.CappConfig {
	return &cappv1alpha1.CappConfig{
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
}

func newCapp(hostname string) *cappv1alpha1.Capp {
	return &cappv1alpha1.Capp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cappName,
			Namespace: nsName,
		},
		Spec: cappv1alpha1.CappSpec{
			RouteSpec: cappv1alpha1.RouteSpec{
				Hostname: hostname,
			},
		},
	}
}
