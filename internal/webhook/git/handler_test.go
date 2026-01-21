package git

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	capputils "github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, rcs.AddToScheme(s))
	require.NoError(t, shipwright.AddToScheme(s))
	return fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&rcs.CappBuild{}).
		WithObjects(objs...).
		Build()
}

func newCappConfig() *rcs.CappConfig {
	return &rcs.CappConfig{
		ObjectMeta: metav1.ObjectMeta{Name: capputils.CappConfigName, Namespace: capputils.CappNS},
		Spec: rcs.CappConfigSpec{
			CappBuild: &rcs.CappBuildConfig{
				ClusterBuildStrategy: rcs.CappBuildClusterStrategyConfig{
					BuildFile: rcs.CappBuildFileStrategyConfig{
						Present: "present-strategy",
						Absent:  "absent-strategy",
					},
				},
				OnCommit: &rcs.CappBuildOnCommitConfig{Enabled: true},
			},
		},
	}
}

func newOnCommitCappBuild(url, revision string) *rcs.CappBuild {
	return &rcs.CappBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cb",
			Namespace: "ns",
			Labels:    map[string]string{"rcs.dana.io/oncommit-enabled": "true"},
		},
		Spec: rcs.CappBuildSpec{
			OnCommit: &rcs.CappBuildOnCommit{
				WebhookSecretRef: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "wh"},
					Key:                  "k",
				},
			},
			Source:    rcs.CappBuildSource{Type: rcs.CappBuildSourceTypeGit, Git: rcs.CappBuildGitSource{URL: url, Revision: revision}},
			BuildFile: rcs.CappBuildFile{Mode: rcs.CappBuildFileModeAbsent},
			Output:    rcs.CappBuildOutput{Image: "registry.example.com/team/app:v1"},
			Rebuild:   &rcs.CappBuildRebuild{Mode: rcs.CappBuildRebuildModeOnCommit},
		},
	}
}

func TestWebhookNoMatch(t *testing.T) {
	c := testClient(t, newCappConfig())
	h := &Handler{Client: c}

	body := `{"ref":"refs/heads/main","after":"abc","project":{"git_http_url":"https://example.com/none.git"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/git", bytes.NewBufferString(body))
	req.Header.Set(headerGitlabEvent, "Push Hook")
	req.Header.Set(headerGitlabToken, "any")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req.WithContext(context.Background()))
	require.Equal(t, http.StatusAccepted, rr.Code)
}