package webhooks

import (
	"context"
	"encoding/json"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	knativeautoscaling "knative.dev/serving/pkg/apis/autoscaling"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
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
			name:      "denies capp with scaleDelaySeconds exceeding global limit",
			operation: admissionv1.Create,
			capp: &cappv1alpha1.Capp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cappName,
					Namespace: nsName,
				},
				Spec: cappv1alpha1.CappSpec{
					ScaleSpec: cappv1alpha1.ScaleSpec{
						ScaleDelaySeconds: ptr.To(int32(150)),
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
			name:      "allows update when capp is terminating with forbidden annotation",
			operation: admissionv1.Update,
			capp: func() *cappv1alpha1.Capp {
				now := metav1.Now()
				capp := newCapp("")
				capp.DeletionTimestamp = &now
				capp.Finalizers = []string{cappCleanupFinalizer}
				capp.Spec.ConfigurationSpec.Template.Annotations = map[string]string{
					knativeautoscaling.GroupName + "/minScale": "3",
				}
				return capp
			}(),
			oldCapp: func() *cappv1alpha1.Capp {
				now := metav1.Now()
				capp := newCapp("")
				capp.DeletionTimestamp = &now
				capp.Finalizers = []string{cappCleanupFinalizer}
				capp.Spec.ConfigurationSpec.Template.Annotations = map[string]string{
					knativeautoscaling.GroupName + "/minScale": "3",
				}
				return capp
			}(),
			expectAllow: true,
		},
		{
			name:      "denies capp when password secret does not exist",
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
						PasswordSecret: missingSecretName,
					},
				},
			},
			expectAllow: false,
			expectMsg:   "secret \"" + missingSecretName + "\" not found",
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

func TestValidateSecretHasKeys(t *testing.T) {
	const (
		secretName     = "existing-secret"
		secretPassword = "password"
	)

	ctx := context.Background()
	scheme := newScheme(t)

	tests := []struct {
		name            string
		secretName      string
		data            map[string]string
		stringData      map[string]string
		requiredKeys    []string
		wantErrContains []string
	}{
		{
			name:         "allows secret with required key in data",
			secretName:   secretName,
			data:         map[string]string{elasticSecretKey: secretPassword},
			requiredKeys: []string{elasticSecretKey},
		},
		{
			name:         "allows secret with required key in stringData",
			secretName:   secretName,
			stringData:   map[string]string{elasticSecretKey: secretPassword},
			requiredKeys: []string{elasticSecretKey},
		},
		{
			name:            "rejects when secret not found",
			secretName:      missingSecretName,
			requiredKeys:    []string{elasticSecretKey},
			wantErrContains: []string{"not found", missingSecretName},
		},
		{
			name:            "rejects when required key is missing",
			secretName:      secretName,
			data:            map[string]string{"wrong-key": "value"},
			requiredKeys:    []string{elasticSecretKey},
			wantErrContains: []string{missingRequiredKeyMessage, elasticSecretKey},
		},
		{
			name:            "rejects when required key is empty",
			secretName:      secretName,
			data:            map[string]string{elasticSecretKey: ""},
			requiredKeys:    []string{elasticSecretKey},
			wantErrContains: []string{missingRequiredKeyMessage, elasticSecretKey},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var objects []client.Object
			if tc.data != nil || tc.stringData != nil {
				secretData := make(map[string][]byte, len(tc.data))
				for key, value := range tc.data {
					secretData[key] = []byte(value)
				}
				objects = []client.Object{&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tc.secretName,
						Namespace: nsName,
					},
					Data:       secretData,
					StringData: tc.stringData,
				}}
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			err := validateSecretHasKeys(ctx, fakeClient, nsName, tc.secretName, tc.requiredKeys)
			if len(tc.wantErrContains) == 0 {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			for _, s := range tc.wantErrContains {
				require.Contains(t, err.Error(), s)
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

	fakeClient := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			capp := cappv1alpha1.Capp{
				Spec: cappv1alpha1.CappSpec{
					EventSourcesSpec: cappv1alpha1.EventSourcesSpec{
						Sources: tc.sources,
					},
				},
			}

			err := validateEventSources(ctx, fakeClient, capp, 5)
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

func TestValidateKafkaSourceConsumers(t *testing.T) {
	tests := []struct {
		name            string
		cfg             *cappv1alpha1.KafkaSourceConfiguration
		maxConsumers    int32
		wantErrContains []string
	}{
		{
			name:         "allows consumers within capacity",
			cfg:          &cappv1alpha1.KafkaSourceConfiguration{Consumers: ptr.To(int32(3))},
			maxConsumers: 5,
		},
		{
			name:         "rejects consumers above capacity",
			cfg:          &cappv1alpha1.KafkaSourceConfiguration{Consumers: ptr.To(int32(6))},
			maxConsumers: 5,
			wantErrContains: []string{
				"consumers",
				"max kafka consumers",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateKafkaSourceConsumers(tc.cfg, tc.maxConsumers)
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

func TestValidateTemplateAnnotations(t *testing.T) {
	forbiddenKey := knativeautoscaling.GroupName + "/minScale"

	tests := []struct {
		name            string
		annotations     map[string]string
		wantErrContains []string
	}{
		{
			name:        "allows capp with non-autoscaling annotations",
			annotations: map[string]string{"app.example.com/foo": "bar"},
		},
		{
			name:            "rejects capp with autoscaling annotation",
			annotations:     map[string]string{forbiddenKey: "3"},
			wantErrContains: []string{"forbidden annotation", forbiddenKey},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			capp := cappv1alpha1.Capp{}
			capp.Spec.ConfigurationSpec.Template.Annotations = tc.annotations

			err := validateTemplateAnnotations(capp)
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

func TestValidateScaleSpec(t *testing.T) {
	const maxReplicasErrMsg = "maxReplicas"

	tests := []struct {
		name              string
		minReplicas       *int32
		maxReplicas       *int32
		scaleDelaySeconds *int32
		autoscaleConfig   cappv1alpha1.AutoscaleConfig
		wantErrContains   []string
	}{
		{
			name:        "allows when minReplicas is at the limit",
			minReplicas: ptr.To(int32(10)),
			autoscaleConfig: cappv1alpha1.AutoscaleConfig{
				MinReplicasLimit: 10,
				MaxScaleDelay:    100,
			},
		},
		{
			name:              "allows when scaleDelaySeconds is at the limit",
			scaleDelaySeconds: ptr.To(int32(100)),
			autoscaleConfig: cappv1alpha1.AutoscaleConfig{
				MinReplicasLimit: 10,
				MaxScaleDelay:    100,
			},
		},
		{
			name:        "rejects when minReplicas exceeds the limit",
			minReplicas: ptr.To(int32(11)),
			autoscaleConfig: cappv1alpha1.AutoscaleConfig{
				MinReplicasLimit: 10,
				MaxScaleDelay:    100,
			},
			wantErrContains: []string{"minReplicas"},
		},
		{
			name:              "rejects when scaleDelaySeconds exceeds the limit",
			scaleDelaySeconds: ptr.To(int32(101)),
			autoscaleConfig: cappv1alpha1.AutoscaleConfig{
				MinReplicasLimit: 10,
				MaxScaleDelay:    100,
			},
			wantErrContains: []string{"scaleDelaySeconds"},
		},
		{
			name:        "allows when maxReplicas is at the limit",
			maxReplicas: ptr.To(int32(10)),
			autoscaleConfig: cappv1alpha1.AutoscaleConfig{
				MinReplicasLimit: 10,
				MaxScaleDelay:    100,
				MaxReplicasLimit: 10,
			},
		},
		{
			name:        "rejects when maxReplicas exceeds the limit",
			maxReplicas: ptr.To(int32(11)),
			autoscaleConfig: cappv1alpha1.AutoscaleConfig{
				MinReplicasLimit: 10,
				MaxScaleDelay:    100,
				MaxReplicasLimit: 10,
			},
			wantErrContains: []string{maxReplicasErrMsg},
		},
		{
			name:        "rejects when maxReplicas is less than minReplicas",
			minReplicas: ptr.To(int32(5)),
			maxReplicas: ptr.To(int32(3)),
			autoscaleConfig: cappv1alpha1.AutoscaleConfig{
				MinReplicasLimit: 10,
				MaxScaleDelay:    100,
				MaxReplicasLimit: 10,
			},
			wantErrContains: []string{maxReplicasErrMsg, "minReplicas"},
		},
		{
			name:        "allows when maxReplicas equals minReplicas",
			minReplicas: ptr.To(int32(3)),
			maxReplicas: ptr.To(int32(3)),
			autoscaleConfig: cappv1alpha1.AutoscaleConfig{
				MinReplicasLimit: 10,
				MaxScaleDelay:    100,
				MaxReplicasLimit: 10,
			},
		},
		{
			name:        "rejects when maxReplicas is less than activationScale and minReplicas is zero",
			maxReplicas: ptr.To(int32(1)),
			autoscaleConfig: cappv1alpha1.AutoscaleConfig{
				MinReplicasLimit: 10,
				MaxScaleDelay:    100,
				MaxReplicasLimit: 10,
				ActivationScale:  2,
			},
			wantErrContains: []string{maxReplicasErrMsg, "activationScale"},
		},
		{
			name:        "allows when maxReplicas equals activationScale and minReplicas is zero",
			maxReplicas: ptr.To(int32(2)),
			autoscaleConfig: cappv1alpha1.AutoscaleConfig{
				MinReplicasLimit: 10,
				MaxScaleDelay:    100,
				MaxReplicasLimit: 10,
				ActivationScale:  2,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			capp := cappv1alpha1.Capp{
				Spec: cappv1alpha1.CappSpec{
					ScaleSpec: cappv1alpha1.ScaleSpec{
						MinReplicas:       tc.minReplicas,
						MaxReplicas:       tc.maxReplicas,
						ScaleDelaySeconds: tc.scaleDelaySeconds,
					},
				},
			}

			err := validateScaleSpec(capp, tc.autoscaleConfig)
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

func TestValidateDomainName(t *testing.T) {
	tests := []struct {
		name            string
		domainName      string
		allowedPatterns []cappv1alpha1.HostnamePattern
		wantErr         bool
		errContains     string
	}{
		{
			name:            "Valid domain matching specific pattern",
			domainName:      "myapp.example.com",
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: allowedHostnamePattern}},
			wantErr:         false,
		},
		{
			name:            "Valid domain matching wild card",
			domainName:      "myapp.any.com",
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: `.*`}},
			wantErr:         false,
		},
		{
			name:            "Invalid domain not matching pattern",
			domainName:      nonMatchingHostname,
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: allowedHostnamePattern}},
			wantErr:         true,
			errContains:     errMustMatchAllowedPatterns,
		},
		{
			name:            "Empty allowed patterns (deny all)",
			domainName:      "myapp.example.com",
			allowedPatterns: []cappv1alpha1.HostnamePattern{},
			wantErr:         true,
			errContains:     errMustMatchAllowedPatterns,
		},
		{
			name:            "Multiple patterns, one match",
			domainName:      "myapp.org",
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: `.*\.com`}, {Match: `.*\.org`}},
			wantErr:         false,
		},
		{
			name:            "Multiple patterns, no match",
			domainName:      "myapp.net",
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: `.*\.com`}, {Match: `.*\.org`}},
			wantErr:         true,
			errContains:     errMustMatchAllowedPatterns,
		},
		{
			name:            "Invalid FQDN syntax",
			domainName:      "-invalid-start",
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: `.*`}},
			wantErr:         true,
		},
		{
			name:            "Invalid hostname with leading dots rejected as FQDN",
			domainName:      "...aaa.a....",
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: `.*`}},
			wantErr:         true,
		},
		{
			name:            "Invalid hostname with underscore rejected as FQDN under wildcard patterns",
			domainName:      "invalid_domain!",
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: `.*`}},
			wantErr:         true,
		},
		{
			name:            "Explanation appears in error message",
			domainName:      nonMatchingHostname,
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: allowedHostnamePattern, Explanation: "subdomains of example.com only"}},
			wantErr:         true,
			errContains:     "subdomains of example.com only",
		},
		{
			name:            "Raw pattern shown when explanation absent",
			domainName:      nonMatchingHostname,
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: allowedHostnamePattern}},
			wantErr:         true,
			errContains:     allowedHostnamePattern,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateDomainName(tt.domainName, tt.allowedPatterns)
			if tt.wantErr {
				assert.NotNil(t, errs)
				if tt.errContains != "" {
					assert.Contains(t, errs.Error(), tt.errContains)
				}
			} else {
				assert.Nil(t, errs)
			}
		})
	}
}

func newCappValidator(t *testing.T, scheme *runtime.Scheme, decoder admission.Decoder) *CappValidator {
	t.Helper()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(newCappConfig()).
		Build()

	return &CappValidator{
		Client:  fakeClient,
		Decoder: decoder,
	}
}
