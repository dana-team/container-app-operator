package git

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGitLabSuccess(t *testing.T) {
	cb := newOnCommitCappBuild("https://gitlab.example/group/repo.git", "main")
	c := testClient(t,
		newCappConfig(),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "wh", Namespace: cb.Namespace}, Data: map[string][]byte{"k": []byte("token")}},
		cb,
	)
	h := &Handler{Client: c}

	body := `{"ref":"refs/heads/main","after":"abc","project":{"git_http_url":"https://gitlab.example/group/repo.git"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/git", bytes.NewBufferString(body))
	req.Header.Set(headerGitlabEvent, "Push Hook")
	req.Header.Set(headerGitlabToken, "token")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req.WithContext(context.Background()))
	require.Equal(t, http.StatusAccepted, rr.Code)

	updated := &rcs.CappBuild{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(cb), updated))
	require.NotNil(t, updated.Status.OnCommit.Pending)
	require.Equal(t, "abc", updated.Status.OnCommit.Pending.CommitSHA)
}