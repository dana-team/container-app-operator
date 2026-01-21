# Implementation — Phase 10 (OnCommit webhooks → BuildRun triggers)

This document is the copy/paste implementation guide for `docs/build-from-source/design/lld/shipwright-phase10.md`.

## 0) Files touched (overview)
- API:
  - `/home/sbahar/projects/ps/dana-team/container-app-operator/api/v1alpha1/cappbuild_types.go`
  - `/home/sbahar/projects/ps/dana-team/container-app-operator/api/v1alpha1/cappconfig_types.go`
- Webhook handler:
  - Directory: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/webhook/git/`
  - Create: `handler.go`, `github.go`, `gitlab.go`, `types.go`
  - Wire: `/home/sbahar/projects/ps/dana-team/container-app-operator/cmd/main.go`
- CappBuild controller:
  - Create: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/oncommit.go`
  - Edit: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/controller.go`
  - Edit: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/buildrun.go`
  - Edit: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/conditions.go`
- Tests:
  - Create: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/webhook/git/handler_test.go`
  - Edit: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/buildrun_test.go`
  - Create: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/oncommit_test.go`
  - Edit (minimal e2e): `/home/sbahar/projects/ps/dana-team/container-app-operator/test/e2e_tests/cappbuild_e2e_test.go`

## 1) API changes

### 1.1) `CappConfig`: add on-commit enablement (feature gate)

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/api/v1alpha1/cappconfig_types.go`

Add types + field under `CappBuildConfig`:

Implementation note:
- Do **not** duplicate `package` / `import` blocks; insert these types/fields into the existing file.
- Ensure `corev1` is imported (for `corev1.SecretKeySelector`).

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/api/v1alpha1/cappconfig_types.go`

```go
type CappBuildConfig struct {
	// +optional
	// Output holds defaults for deriving the build output image.
	Output *CappBuildOutputConfig `json:"output,omitempty"`

	// ClusterBuildStrategy holds platform defaults for selecting a build strategy.
	ClusterBuildStrategy CappBuildClusterStrategyConfig `json:"clusterBuildStrategy"`

	// +optional
	// OnCommit configures webhook-triggered rebuilds.
	OnCommit *CappBuildOnCommitConfig `json:"onCommit,omitempty"`
}

type CappBuildOnCommitConfig struct {
	// Enabled enables webhook-triggered rebuilds.
	// +kubebuilder:default:=false
	Enabled bool `json:"enabled"`
}
```

Notes:
- This Phase 10 impl keeps debounce/min-interval as controller defaults (10s / 30s).
- Webhook secrets are per-`CappBuild` (see section 1.2 / section 2).

### 1.2) `CappBuild`: add `spec.onCommit.webhookSecretRef` (per-CappBuild secret)

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/api/v1alpha1/cappbuild_types.go`

Add types + field:

Implementation note:
- Do **not** duplicate `package` / `import` blocks; insert these types/fields into the existing file.
- Ensure `corev1` is imported (for `corev1.SecretKeySelector`).

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/api/v1alpha1/cappbuild_types.go`

```go
type CappBuildOnCommit struct {
	// WebhookSecretRef references the Secret used to verify webhook requests.
	WebhookSecretRef corev1.SecretKeySelector `json:"webhookSecretRef"`
}

type CappBuildSpec struct {
	// ... keep existing fields ...

	// +optional
	// OnCommit configures webhook-triggered rebuilds.
	OnCommit *CappBuildOnCommit `json:"onCommit,omitempty"`
}
```

### 1.3) `CappBuildStatus`: add `status.onCommit.*` trigger state

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/api/v1alpha1/cappbuild_types.go`

Add types + field:

Implementation note:
- Do **not** duplicate `package` / `import` blocks; insert these types/fields into the existing file.
- Ensure `metav1` is imported (for `metav1.Time`).

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/api/v1alpha1/cappbuild_types.go`

```go
type CappBuildOnCommitEvent struct {
	// Ref is the git ref from the webhook payload.
	// +optional
	Ref string `json:"ref,omitempty"`

	// CommitSHA is the commit SHA from the webhook payload.
	// +optional
	CommitSHA string `json:"commitSHA,omitempty"`

	// ReceivedAt is when the webhook was received.
	// +optional
	ReceivedAt metav1.Time `json:"receivedAt,omitempty"`
}

type CappBuildOnCommitLastTriggered struct {
	// Name is the name of the last BuildRun created from an on-commit trigger.
	// +optional
	Name string `json:"name,omitempty"`

	// TriggeredAt is when the last BuildRun was created from an on-commit trigger.
	// +optional
	TriggeredAt metav1.Time `json:"triggeredAt,omitempty"`
}

type CappBuildOnCommitStatus struct {
	// LastReceived is the last received webhook event.
	// +optional
	LastReceived *CappBuildOnCommitEvent `json:"lastReceived,omitempty"`

	// Pending is the latest pending on-commit trigger.
	// +optional
	Pending *CappBuildOnCommitEvent `json:"pending,omitempty"`

	// LastTriggeredBuildRun references the last BuildRun created from an on-commit trigger.
	// +optional
	LastTriggeredBuildRun *CappBuildOnCommitLastTriggered `json:"lastTriggeredBuildRun,omitempty"`

	// TriggerCounter is used to derive deterministic BuildRun names for on-commit triggers.
	// +optional
	TriggerCounter int64 `json:"triggerCounter,omitempty"`
}

type CappBuildStatus struct {
	// ... keep existing fields ...

	// +optional
	// OnCommit stores on-commit trigger state.
	OnCommit *CappBuildOnCommitStatus `json:"onCommit,omitempty"`
}
```

## 2) Webhook handler (GitHub + GitLab)

### 2.1) Add `internal/webhook/git/` (Modular Structure)

Create the directory and the following files.

Add dependencies:

```bash
go get github.com/google/go-github/v69/github@latest
go get github.com/go-playground/webhooks/v6/gitlab@latest
```

#### 2.1.1) `internal/webhook/git/types.go` (Interface & Shared Types)

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/webhook/git/types.go`

```go
package git

import (
	"net/http"
)

type pushEvent struct {
	RepoURL   string
	Ref       string
	CommitSHA string
}

type webhookProvider interface {
	Name() string
	Detect(r *http.Request) bool
	ReadPushEvent(body []byte) (*pushEvent, error)
	Authenticate(r *http.Request, body []byte, secret []byte) error
}
```

#### 2.1.2) `internal/webhook/git/github.go` (GitHub Implementation)

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/webhook/git/github.go`

```go
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
	req := &http.Request{
		Header: r.Header.Clone(),
		Body:   io.NopCloser(bytes.NewReader(body)),
	}
	_, err := github.ValidatePayload(req, secret)
	return err
}
```

#### 2.1.3) `internal/webhook/git/gitlab.go` (GitLab Implementation)

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/webhook/git/gitlab.go`

```go
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
	req := &http.Request{
		Header: r.Header.Clone(),
		Body:   io.NopCloser(bytes.NewReader(body)),
	}
	_, err = hook.Parse(req, gitlab.PushEvents)
	return err
}
```

#### 2.1.4) `internal/webhook/git/handler.go` (Orchestrator)

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/webhook/git/handler.go`

```go
package git

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/record"
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
	var matches []rcs.CappBuild
	for _, cb := range list.Items {
		cb := cb
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
		cb := cb
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
			Ref:       event.Ref,
			CommitSHA: event.CommitSHA,
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
```

Notes:
- The handler purposefully does **no BuildRun creation**; it only records `status.onCommit.pending`.
- Debounce / rate limit defaults are enforced in reconciliation (not in the handler).

Required label selector:
- Add a stable label maintained by the controller (or a mutating webhook):
  - label key: `rcs.dana.io/oncommit-enabled`
  - value: `"true"` when `spec.rebuild.mode=OnCommit`

### 2.2) Wire the handler in `cmd/main.go`

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/cmd/main.go`

Add import:

```go
gitwebhook "github.com/dana-team/container-app-operator/internal/webhook/git"
```

Then, inside the existing `ENABLE_WEBHOOKS` block, register the endpoint **only when OnCommit is enabled in CappConfig**.

Add this helper:

```go
func isOnCommitWebhookEnabled(ctx context.Context, c client.Client) bool {
	cfg, err := capputils.GetCappConfig(c)
	if err != nil || cfg.Spec.CappBuild == nil || cfg.Spec.CappBuild.OnCommit == nil {
		return false
	}
	return cfg.Spec.CappBuild.OnCommit.Enabled
}
```

Import note:
- Ensure `context` and `capputils` are imported in `cmd/main.go`.

And register conditionally:

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/cmd/main.go`

```go
if isOnCommitWebhookEnabled(context.Background(), mgr.GetClient()) {
	hookServer.Register("/webhooks/git", &gitwebhook.Handler{
		Client:        mgr.GetClient(),
		EventRecorder: mgr.GetEventRecorderFor("git-webhook"),
	})
}
```

Note:
- This keeps the handler from running at all when OnCommit is disabled.
- It requires a controller restart to pick up CappConfig changes (enable/disable).

## 3) CappBuild controller: debounce + rate-limit + BuildRun creation (OnCommit)

### 3.1) Add condition reasons

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/conditions.go`

Add:

```go
const (
	ReasonOnCommitDisabled   = "OnCommitDisabled"
	ReasonWebhookSecretMissing = "WebhookSecretMissing"
	ReasonOnCommitBuildTriggered = "OnCommitBuildTriggered"
)
```

### 3.1.1) Maintain the on-commit label (required for webhook filtering)

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/controller.go`

Add a small helper and use it in reconcile (after `cb` is loaded):

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/controller.go`

```go
const onCommitLabelKey = "rcs.dana.io/oncommit-enabled"

func (r *CappBuildReconciler) ensureOnCommitLabel(ctx context.Context, cb *rcs.CappBuild) error {
	desired := "false"
	if cb.Spec.Rebuild != nil && cb.Spec.Rebuild.Mode == rcs.CappBuildRebuildModeOnCommit {
		desired = "true"
	}

	if cb.Labels == nil {
		cb.Labels = map[string]string{}
	}
	if cb.Labels[onCommitLabelKey] == desired {
		return nil
	}

	orig := cb.DeepCopy()
	cb.Labels[onCommitLabelKey] = desired
	return r.Patch(ctx, cb, client.MergeFrom(orig))
}
```

Then use it in reconcile (near the top, after fetching `cb`):

```go
if err := r.ensureOnCommitLabel(ctx, cb); err != nil {
	return ctrl.Result{RequeueAfter: 30 * time.Second}, err
}
```

### 3.2) Add OnCommit reconciliation helper

Create:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/oncommit.go`

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/oncommit.go`

```go
package controllers

import (
	"context"
	"fmt"
	"time"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	capputils "github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	onCommitDebounce    = 10 * time.Second
	onCommitMinInterval = 30 * time.Second
)

// triggerBuildRun enforces debounce/rate-limit/one-active-build and creates a BuildRun
// when a pending trigger is ready.
//
// Returns:
// - selected BuildRun to use for status mapping (may be an existing active run)
// - optional requeueAfter for debounce/rate-limit timers
func (r *CappBuildReconciler) triggerBuildRun(
	ctx context.Context,
	cb *rcs.CappBuild,
) (*shipwright.BuildRun, *time.Duration, error) {
	// 1. Only process if mode is OnCommit
	if cb.Spec.Rebuild == nil || cb.Spec.Rebuild.Mode != rcs.CappBuildRebuildModeOnCommit {
		return nil, nil, nil
	}

	// 2. Check global policy (provides immediate feedback even if no push yet)
	if !isPolicyEnabled(ctx, r.Client) {
		_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonOnCommitDisabled, "OnCommit rebuilds are disabled by policy")
		return nil, nil, nil
	}

	// 3. Check if there is an actual trigger to process
	if cb.Status.OnCommit == nil || cb.Status.OnCommit.Pending == nil {
		return nil, nil, nil
	}

	if requeueAfter := requeueAfter(cb); requeueAfter != nil {
		return nil, requeueAfter, nil
	}

	if cb.Status.LastBuildRunRef != "" {
		active := &shipwright.BuildRun{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: cb.Namespace, Name: cb.Status.LastBuildRunRef}, active); err == nil {
			if metav1.IsControlledBy(active, cb) {
				cond := active.Status.GetCondition(shipwright.Succeeded)
				// If the build is still running (not True and not False), return it as active.
				if cond == nil || (cond.GetStatus() != corev1.ConditionTrue && cond.GetStatus() != corev1.ConditionFalse) {
					return active, nil, nil
				}
			}
		} else if client.IgnoreNotFound(err) != nil {
			return nil, nil, err
		}
	}

	counter := nextTrigger(cb)
	br, err := r.ensureBuildRunOnCommit(ctx, cb, counter)
	if err != nil {
		return nil, nil, err
	}

	if err := r.markTriggered(ctx, cb, br); err != nil {
		return nil, nil, err
	}

	return br, nil, nil
}

func isPolicyEnabled(ctx context.Context, c client.Client) bool {
	cfg, err := capputils.GetCappConfig(c)
	if err != nil || cfg.Spec.CappBuild == nil || cfg.Spec.CappBuild.OnCommit == nil {
		return false
	}
	return cfg.Spec.CappBuild.OnCommit.Enabled
}

func requeueAfter(cb *rcs.CappBuild) *time.Duration {
	receivedAt := cb.Status.OnCommit.Pending.ReceivedAt.Time
	if !receivedAt.IsZero() {
		if remaining := time.Until(receivedAt.Add(onCommitDebounce)); remaining > 0 {
			return &remaining
		}
	}

	if cb.Status.OnCommit.LastTriggeredBuildRun != nil && !cb.Status.OnCommit.LastTriggeredBuildRun.TriggeredAt.IsZero() {
		last := cb.Status.OnCommit.LastTriggeredBuildRun.TriggeredAt.Time
		if remaining := time.Until(last.Add(onCommitMinInterval)); remaining > 0 {
			return &remaining
		}
	}

	return nil
}

func nextTrigger(cb *rcs.CappBuild) int64 {
	if cb.Status.OnCommit.TriggerCounter < 0 {
		cb.Status.OnCommit.TriggerCounter = 0
	}
	cb.Status.OnCommit.TriggerCounter++
	return cb.Status.OnCommit.TriggerCounter
}

func (r *CappBuildReconciler) markTriggered(ctx context.Context, cb *rcs.CappBuild, br *shipwright.BuildRun) error {
	orig := cb.DeepCopy()
	cb.Status.OnCommit.LastTriggeredBuildRun = &rcs.CappBuildOnCommitLastTriggered{
		Name:       br.Name,
		TriggeredAt: metav1.Now(),
	}
	cb.Status.OnCommit.Pending = nil
	return r.Status().Patch(ctx, cb, client.MergeFrom(orig))
}

```

### 3.3) Add BuildRun creation for OnCommit counter-based names

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/buildrun.go`

Add these helpers next to the existing ones:

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/buildrun.go`

```go
func (r *CappBuildReconciler) ensureBuildRunOnCommit(ctx context.Context, cb *rcs.CappBuild, counter int64) (*shipwright.BuildRun, error) {
	desired := newBuildRun(cb)
	desired.Name = fmt.Sprintf("%s-buildrun-oncommit-%d", cb.Name, counter)
	desired.Labels["rcs.dana.io/build-trigger"] = "oncommit"

	existing := &shipwright.BuildRun{}
	key := client.ObjectKeyFromObject(desired)
	if err := r.Get(ctx, key, existing); err == nil {
		if !metav1.IsControlledBy(existing, cb) {
			return nil, &controllerutil.AlreadyOwnedError{Object: existing}
		}
		return existing, nil
	} else if client.IgnoreNotFound(err) != nil {
		return nil, err
	}

	if err := controllerutil.SetControllerReference(cb, desired, r.Scheme); err != nil {
		return nil, err
	}
	if err := r.Create(ctx, desired); err != nil {
		return nil, err
	}
	return desired, nil
}
```

### 3.4) Integrate OnCommit BuildRun selection into `Reconcile`

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/controller.go`

Replace the single `reconcileBuildRun(...)` call with:

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/controller.go`

```go
var buildRun *shipwright.BuildRun

// First: apply OnCommit policy if a pending trigger exists.
if br, requeueAfter, err := r.triggerBuildRun(ctx, cb); err != nil {
	_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonBuildRunReconcileFailed, err.Error())
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
} else if requeueAfter != nil {
	return ctrl.Result{RequeueAfter: *requeueAfter}, nil
} else if br != nil {
	buildRun = br
}

// Fallback: ensure the generation-based BuildRun exists (initial/manual behavior).
if buildRun == nil {
	br, err := r.reconcileBuildRun(ctx, cb)
	if err != nil {
		if errors.As(err, &alreadyOwned) {
			_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonBuildRunConflict, err.Error())
			return ctrl.Result{}, nil
		}
		_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonBuildRunReconcileFailed, err.Error())
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	buildRun = br
}
```

Then keep the existing status mapping logic:
- `patchBuildSucceededCondition(ctx, cb, buildRun)`
- if successful, `patchLatestImage(...)`
- if `BuildSucceeded=Unknown`, requeue `10s`

## 4) Unit tests

### 4.1) Webhook handler tests

### 4.1) Git Webhook Tests

Create three files in `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/webhook/git/`:

#### 1. `handler_test.go` (Shared helpers + Generic tests)

```go
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
		ObjectMeta: metav1.ObjectMeta{Name: "cb", Namespace: "ns"},
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
```

#### 2. `github_test.go`

```go
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
	req.Header.Set(github.EventTypeHeader, "push")
	req.Header.Set(github.SHA256SignatureHeader, sig)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req.WithContext(context.Background()))
	require.Equal(t, http.StatusAccepted, rr.Code)
}
```

#### 3. `gitlab_test.go`

```go
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
```

### 4.2) Controller tests for debounce / counter BuildRuns

Create:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/oncommit_test.go`

```go
package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestTriggerNaming(t *testing.T) {
	cb := newCappBuild("cb", "ns")
	cb.Spec.Rebuild = &rcs.CappBuildRebuild{Mode: rcs.CappBuildRebuildModeOnCommit}
	cb.Status.OnCommit = &rcs.CappBuildOnCommitStatus{
		Pending: &rcs.CappBuildOnCommitEvent{Ref: "refs/heads/main", CommitSHA: "abc"},
	}

	cfg := newCappConfig()
	cfg.Spec.CappBuild.OnCommit = &rcs.CappBuildOnCommitConfig{Enabled: true}
	r, _ := newReconciler(t, cfg, cb)

	br, requeue, err := r.triggerBuildRun(context.Background(), cb)
	require.NoError(t, err)
	require.Nil(t, requeue)
	require.NotNil(t, br)
	require.Equal(t, fmt.Sprintf("%s-buildrun-oncommit-1", cb.Name), br.Name)
	require.Equal(t, "oncommit", br.Labels["rcs.dana.io/build-trigger"])
}

func TestTriggerActiveBuild(t *testing.T) {
	cb := newCappBuild("cb", "ns")
	cb.Spec.Rebuild = &rcs.CappBuildRebuild{Mode: rcs.CappBuildRebuildModeOnCommit}
	cb.Status.LastBuildRunRef = "active-br"
	cb.Status.OnCommit = &rcs.CappBuildOnCommitStatus{
		Pending: &rcs.CappBuildOnCommitEvent{Ref: "refs/heads/main", CommitSHA: "abc"},
	}

	// Create an active BuildRun (no succeeded condition yet)
	activeBR := &shipwright.BuildRun{
		ObjectMeta: metav1.ObjectMeta{Name: "active-br", Namespace: cb.Namespace},
	}
	require.NoError(t, metav1.SetControllerReference(cb, activeBR, testScheme(t)))

	cfg := newCappConfig()
	cfg.Spec.CappBuild.OnCommit = &rcs.CappBuildOnCommitConfig{Enabled: true}
	r, _ := newReconciler(t, cfg, cb, activeBR)

	br, requeue, err := r.triggerBuildRun(context.Background(), cb)
	require.NoError(t, err)
	require.Nil(t, requeue)
	require.Nil(t, br, "should not trigger new build while one is active")
}

func TestTriggerDebounce(t *testing.T) {
	now := time.Now()
	cb := newCappBuild("cb", "ns")
	cb.Spec.Rebuild = &rcs.CappBuildRebuild{Mode: rcs.CappBuildRebuildModeOnCommit}
	cb.Status.OnCommit = &rcs.CappBuildOnCommitStatus{
		Pending: &rcs.CappBuildOnCommitEvent{
			Ref: "refs/heads/main", 
			CommitSHA: "abc",
			ReceivedAt: metav1.NewTime(now),
		},
	}

	cfg := newCappConfig()
	cfg.Spec.CappBuild.OnCommit = &rcs.CappBuildOnCommitConfig{Enabled: true}
	r, _ := newReconciler(t, cfg, cb)

	br, requeue, err := r.triggerBuildRun(context.Background(), cb)
	require.NoError(t, err)
	require.NotNil(t, requeue, "should requeue for debounce")
	require.Nil(t, br)
	require.True(t, *requeue > 0)
}
```

## 5) Minimal e2e extension (optional but recommended)

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/test/e2e_tests/cappbuild_e2e_test.go`

Add one spec that:
- creates a `CappBuild` with `rebuild.mode=OnCommit`
- patches `status.onCommit.pending` (simulating webhook acceptance)
- asserts a new owned BuildRun with name prefix `<cappBuild.name>-buildrun-oncommit-`

## 6) Regenerate manifests + run tests

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator` (run in a shell)

```bash
cd /home/sbahar/projects/ps/dana-team/container-app-operator
make fmt
make manifests
go test ./internal/kinds/cappbuild/controllers ./internal/webhook/git
```
