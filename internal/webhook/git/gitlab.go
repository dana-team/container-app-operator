package git

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-playground/webhooks/v6/gitlab"
)

const (
	headerGitlabEvent = "X-Gitlab-Event"
	headerGitlabToken = "X-Gitlab-Token"
)

type gitlabProvider struct{}

func (p *gitlabProvider) Name() string { return "gitlab" }

func (p *gitlabProvider) Detect(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get(headerGitlabEvent)), string(gitlab.PushEvents))
}

func (p *gitlabProvider) ReadPushEvent(body []byte) (*pushEvent, error) {
	var payload gitlab.PushEventPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse GitLab push event: %w", err)
	}

	repo := strings.TrimSpace(payload.Project.GitHTTPURL)
	if repo == "" {
		repo = strings.TrimSpace(payload.Project.WebURL)
	}
	if repo == "" || strings.TrimSpace(payload.Ref) == "" {
		return nil, fmt.Errorf("missing required fields: ref or repository URL")
	}
	return &pushEvent{RepoURL: repo, Ref: payload.Ref, CommitSHA: payload.After}, nil
}

func (p *gitlabProvider) Authenticate(r *http.Request, body []byte, secret []byte) error {
	hook, err := gitlab.New(gitlab.Options.Secret(string(secret)))
	if err != nil {
		return err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	_, err = hook.Parse(r, gitlab.PushEvents)
	return err
}