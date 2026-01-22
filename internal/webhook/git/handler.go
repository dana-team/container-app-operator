package git

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Handler struct {
	Client        client.Client
	EventRecorder record.EventRecorder
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx).WithName("git-webhook")

	body, provider, err := h.detectProvider(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	event, err := provider.ReadPushEvent(body)
	if err != nil {
		logger.Error(err, "failed to read push event", "provider", provider.Name())
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	var list rcs.CappBuildList
	if err := h.Client.List(ctx, &list, client.MatchingLabels{"rcs.dana.io/oncommit-enabled": "true"}); err != nil {
		logger.Error(err, "failed to list cappbuilds")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	normalizedRepoURL := normalizeRepoURL(event.RepoURL)
	matches := make([]rcs.CappBuild, 0, len(list.Items))
	for _, cb := range list.Items {

		isMatch := cb.Spec.Rebuild != nil &&
			cb.Spec.Rebuild.Mode == rcs.CappBuildRebuildModeOnCommit &&
			cb.Spec.Source.Type == rcs.CappBuildSourceTypeGit &&
			cb.Spec.Source.Git.URL != "" &&
			cb.Spec.Source.Git.Revision != "" &&
			normalizeRepoURL(cb.Spec.Source.Git.URL) == normalizedRepoURL &&
			event.Ref == "refs/heads/"+cb.Spec.Source.Git.Revision

		if !isMatch {
			continue
		}
		matches = append(matches, cb)
	}

	if len(matches) == 0 {
		logger.Info("webhook ignored: no matching CappBuild found", "repo", event.RepoURL, "ref", event.Ref)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	now := metav1.Now()
	var authenticatedCount int
	for _, cb := range matches {

		secret, err := resolveWebhookSecret(ctx, h.Client, &cb)
		if err != nil {
			logger.Error(err, "failed to resolve webhook secret", "name", cb.Name, "namespace", cb.Namespace)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if err := provider.Authenticate(r, body, secret); err != nil {
			continue
		}

		authenticatedCount++
		if err := h.patchOnCommitStatus(ctx, &cb, event, now); err != nil {
			logger.Error(err, "failed to update cappbuild status", "name", cb.Name, "namespace", cb.Namespace)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if h.EventRecorder != nil {
			h.EventRecorder.Eventf(&cb, corev1.EventTypeNormal, "WebhookAccepted", "git webhook accepted for %s/%s", cb.Namespace, cb.Name)
		}
	}

	if authenticatedCount == 0 {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) detectProvider(r *http.Request) ([]byte, webhookProvider, error) {
	if r.Method != http.MethodPost {
		return nil, nil, fmt.Errorf("method %s not allowed", r.Method)
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read body: %w", err)
	}

	for _, p := range []webhookProvider{&githubProvider{}, &gitlabProvider{}} {
		if p.Detect(r) {
			return body, p, nil
		}
	}

	return nil, nil, fmt.Errorf("unsupported webhook event: only GitHub and GitLab push events are supported")
}

func (h *Handler) patchOnCommitStatus(ctx context.Context, cb *rcs.CappBuild, event *pushEvent, now metav1.Time) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest := &rcs.CappBuild{}
		if err := h.Client.Get(ctx, types.NamespacedName{Name: cb.Name, Namespace: cb.Namespace}, latest); err != nil {
			return err
		}
		if latest.Status.OnCommit == nil {
			latest.Status.OnCommit = &rcs.CappBuildOnCommitStatus{}
		}
		onCommitEvent := &rcs.CappBuildOnCommitEvent{
			Ref:        event.Ref,
			CommitSHA:  event.CommitSHA,
			ReceivedAt: now,
		}
		latest.Status.OnCommit.LastReceived = onCommitEvent
		latest.Status.OnCommit.Pending = onCommitEvent
		return h.Client.Status().Update(ctx, latest)
	})
}

func resolveWebhookSecret(ctx context.Context, c client.Client, cb *rcs.CappBuild) ([]byte, error) {
	if cb.Spec.OnCommit == nil {
		return nil, fmt.Errorf("missing spec.onCommit")
	}
	ref := cb.Spec.OnCommit.WebhookSecretRef
	sec := &corev1.Secret{}
	key := types.NamespacedName{
		Name:      ref.Name,
		Namespace: cb.Namespace,
	}
	if err := c.Get(ctx, key, sec); err != nil {
		return nil, err
	}
	val, ok := sec.Data[ref.Key]
	if !ok || len(val) == 0 {
		return nil, fmt.Errorf("missing key %q in secret %s/%s", ref.Key, key.Namespace, key.Name)
	}
	return val, nil
}

func normalizeRepoURL(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimRight(s, "/")
	return s
}
