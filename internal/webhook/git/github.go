package git

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/go-github/v69/github"
)

type githubProvider struct{}

func (p *githubProvider) Name() string { return "github" }

func (p *githubProvider) Detect(r *http.Request) bool {
	return github.WebHookType(r) == "push"
}

func (p *githubProvider) ReadPushEvent(body []byte) (*pushEvent, error) {
	webhookEvent, err := github.ParseWebHook("push", body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GitHub push event: %w", err)
	}
	payload, ok := webhookEvent.(*github.PushEvent)
	if !ok {
		return nil, fmt.Errorf("unexpected GitHub event type: %T", webhookEvent)
	}

	repo := strings.TrimSpace(payload.GetRepo().GetCloneURL())
	if repo == "" {
		repo = strings.TrimSpace(payload.GetRepo().GetHTMLURL())
	}
	if repo == "" || strings.TrimSpace(payload.GetRef()) == "" {
		return nil, fmt.Errorf("missing required fields: ref or repository URL")
	}
	return &pushEvent{RepoURL: repo, Ref: payload.GetRef(), CommitSHA: payload.GetAfter()}, nil
}

func (p *githubProvider) Authenticate(r *http.Request, body []byte, secret []byte) error {
	r.Body = io.NopCloser(bytes.NewReader(body))
	_, err := github.ValidatePayload(r, secret)
	return err
}
