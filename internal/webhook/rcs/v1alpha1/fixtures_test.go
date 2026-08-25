package webhooks

import (
	"encoding/json"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/stretchr/testify/require"
	gomodulesjsonpatch "gomodules.xyz/jsonpatch/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

const (
	cappName             = "test-capp"
	nsName               = "test-ns"
	testUsername         = "developer@example.com"
	testContainerName    = "app"
	cappCleanupFinalizer = "dana.io/capp-cleanup"

	mountedNFSVolumeName      = "mounted"
	unmountedNFSVolumeName    = "a-data"
	eventSourceName           = "ping-a"
	unchangedHostname         = "same.example.com"
	oldHostname               = "old.example.com"
	newHostname               = "new.example.com"
	elasticHost               = "https://elastic.example.com"
	elasticIndex              = "my-index"
	elasticSecretKey          = "elastic"
	elasticPasswordKey        = "password"
	missingSecretName         = "missing-secret"
	existingSecretName        = "existing-secret"
	missingRequiredKeyMessage = "missing required key"

	allowedHostnamePattern      = `.*\.example\.com`
	nonMatchingHostname         = "myapp.other.com"
	errMustMatchAllowedPatterns = "must match one of the allowed patterns"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(cappv1alpha1.AddToScheme(scheme))

	return scheme
}

func newCappConfig() *cappv1alpha1.CappConfig {
	return &cappv1alpha1.CappConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cappmeta.CappConfigName,
			Namespace: cappmeta.CappNS,
		},
		Spec: cappv1alpha1.CappConfigSpec{
			AllowedHostnamePatterns: []cappv1alpha1.HostnamePattern{{Match: ".*"}},
			MaxKafkaConsumers:       5,
			AutoscaleConfig: cappv1alpha1.AutoscaleConfig{
				MinReplicasLimit: 10,
				MaxScaleDelay:    100,
				MaxReplicasLimit: 10,
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

func newDefaultResources() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}
}

func patchedCapp(t *testing.T, raw []byte, patches []gomodulesjsonpatch.JsonPatchOperation) cappv1alpha1.Capp {
	t.Helper()

	patchBytes, err := json.Marshal(patches)
	require.NoError(t, err)

	patch, err := jsonpatch.DecodePatch(patchBytes)
	require.NoError(t, err)

	modified, err := patch.Apply(raw)
	require.NoError(t, err)

	var capp cappv1alpha1.Capp
	require.NoError(t, json.Unmarshal(modified, &capp))
	return capp
}
