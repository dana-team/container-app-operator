package git

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-github/v69/github"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGitHubSuccess(t *testing.T) {
	secret := []byte("s3cr3t")
	cb := newOnCommitCappBuild("https://github.com/org/repo", "main")
	c := testClient(t,
		newCappConfig(),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "wh", Namespace: cb.Namespace}, Data: map[string][]byte{"k": secret}},
		cb,
	)
	h := &Handler{Client: c}

	body := []byte(`{"ref":"refs/heads/main","after":"abc","repository":{"html_url":"https://github.com/org/repo"}}`)
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/git", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(github.EventTypeHeader, "push")
	req.Header.Set(github.SHA256SignatureHeader, sig)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req.WithContext(context.Background()))
	require.Equal(t, http.StatusAccepted, rr.Code)
}
