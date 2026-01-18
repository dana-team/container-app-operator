# Implementation — Phase 10 (OnCommit webhooks → BuildRun triggers)

This document is the copy/paste implementation guide for `docs/build-from-source/design/lld/shipwright-phase10.md`.

## 0) Files touched (overview)
- API:
  - `/home/sbahar/projects/ps/dana-team/container-app-operator/api/v1alpha1/cappbuild_types.go`
  - `/home/sbahar/projects/ps/dana-team/container-app-operator/api/v1alpha1/cappconfig_types.go`
- Webhook handler:
  - Create: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/webhook/git/handler.go`
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
	// Ref is the git ref from the webhook payload (e.g., refs/heads/main).
	// +optional
	Ref string `json:"ref,omitempty"`

	// AfterSHA is the "after" commit SHA from the webhook payload (best-effort).
	// +optional
	AfterSHA string `json:"afterSHA,omitempty"`

	// ReceivedAt is when the operator received (and accepted) the webhook event.
	// +optional
	ReceivedAt metav1.Time `json:"receivedAt,omitempty"`
}

type CappBuildOnCommitLastTriggered struct {
	// +optional
	Name string `json:"name,omitempty"`

	// +optional
	TriggeredAt metav1.Time `json:"triggeredAt,omitempty"`
}

type CappBuildOnCommitStatus struct {
	// +optional
	LastReceived *CappBuildOnCommitEvent `json:"lastReceived,omitempty"`

	// +optional
	Pending *CappBuildOnCommitEvent `json:"pending,omitempty"`

	// +optional
	LastTriggeredBuildRun *CappBuildOnCommitLastTriggered `json:"lastTriggeredBuildRun,omitempty"`

	// +optional
	TriggerCounter int64 `json:"triggerCounter,omitempty"`
}

type CappBuildStatus struct {
	// ... keep existing fields ...

	// +optional
	// OnCommit stores webhook-trigger trigger intent and execution bookkeeping.
	OnCommit *CappBuildOnCommitStatus `json:"onCommit,omitempty"`
}
```

## 2) Webhook handler (GitHub + GitLab)

### 2.1) Add `internal/webhook/git/handler.go`

Create:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/webhook/git/handler.go`

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/webhook/git/handler.go`

```go
package git

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	capputils "github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/record"
)

const (
	pathWebhooksGit = "/webhooks/git"

	headerGithubEvent    = "X-GitHub-Event"
	headerGithubSig256   = "X-Hub-Signature-256"
	headerGitlabEvent    = "X-Gitlab-Event"
	headerGitlabToken    = "X-Gitlab-Token"
	githubPushEvent      = "push"
	gitlabPushEvent      = "Push Hook"
	githubSig256Prefix   = "sha256="
)

type Handler struct {
	Client        client.Client
	EventRecorder record.EventRecorder
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx).WithName("git-webhook")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	provider, err := detectProvider(r.Header)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cappConfig, err := capputils.GetCappConfig(h.Client)
	if err != nil || cappConfig.Spec.CappBuild == nil || cappConfig.Spec.CappBuild.OnCommit == nil || !cappConfig.Spec.CappBuild.OnCommit.Enabled {
		// Feature disabled: accept the request but do nothing (keeps providers from retry-storming).
		w.WriteHeader(http.StatusAccepted)
		return
	}

	ev, err := parsePushEvent(provider, body)
	if err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	candidates, err := h.findCandidateCappBuilds(ctx, ev)
	if err != nil {
		logger.Error(err, "failed to list cappbuilds")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(candidates) == 0 {
		if h.EventRecorder != nil {
			h.EventRecorder.Eventf(cappConfig, corev1.EventTypeNormal, "WebhookNoMatch", "git webhook accepted, no CappBuild matched")
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}

	now := metav1.Now()
	verifiedAny := false
	for _, cb := range candidates {
		cb := cb
		secret, err := loadCappBuildWebhookSecret(ctx, h.Client, &cb)
		if err != nil {
			logger.Error(err, "failed to load cappbuild webhook secret", "name", cb.Name, "namespace", cb.Namespace)
			continue
		}
		if err := verifyRequest(provider, r.Header, body, secret); err != nil {
			// Not a match for this CappBuild secret.
			continue
		}
		verifiedAny = true

		// Patch status to record trigger intent (controller applies debounce/rate-limit/active-build gating).
		err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			latest := &rcs.CappBuild{}
			if err := h.Client.Get(ctx, types.NamespacedName{Name: cb.Name, Namespace: cb.Namespace}, latest); err != nil {
				return err
			}

			if latest.Status.OnCommit == nil {
				latest.Status.OnCommit = &rcs.CappBuildOnCommitStatus{}
			}

			event := &rcs.CappBuildOnCommitEvent{
				Ref:       ev.Ref,
				AfterSHA:  ev.AfterSHA,
				ReceivedAt: now,
			}
			latest.Status.OnCommit.LastReceived = event
			latest.Status.OnCommit.Pending = event

			return h.Client.Status().Update(ctx, latest)
		})
		if err != nil {
			logger.Error(err, "failed to update cappbuild status", "name", cb.Name, "namespace", cb.Namespace)
			continue
		}
		if h.EventRecorder != nil {
			h.EventRecorder.Eventf(&cb, corev1.EventTypeNormal, "WebhookAccepted", "git webhook accepted for %s/%s", cb.Namespace, cb.Name)
		}
	}

	if !verifiedAny {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Respond quickly; actual work happens in reconciliation.
	w.WriteHeader(http.StatusAccepted)
}

func Path() string { return pathWebhooksGit }

type provider string

const (
	providerGitHub provider = "github"
	providerGitLab provider = "gitlab"
)

func detectProvider(h http.Header) (provider, error) {
	if strings.EqualFold(strings.TrimSpace(h.Get(headerGithubEvent)), githubPushEvent) {
		return providerGitHub, nil
	}
	if strings.EqualFold(strings.TrimSpace(h.Get(headerGitlabEvent)), gitlabPushEvent) {
		return providerGitLab, nil
	}
	return "", fmt.Errorf("unsupported or missing webhook event headers")
}

func loadCappBuildWebhookSecret(ctx context.Context, c client.Client, cb *rcs.CappBuild) ([]byte, error) {
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

func verifyRequest(p provider, h http.Header, body []byte, secret []byte) error {
	switch p {
	case providerGitHub:
		got := strings.TrimSpace(h.Get(headerGithubSig256))
		if !strings.HasPrefix(got, githubSig256Prefix) {
			return fmt.Errorf("missing %s", headerGithubSig256)
		}
		gotHex := strings.TrimPrefix(got, githubSig256Prefix)
		gotSig, err := hex.DecodeString(gotHex)
		if err != nil {
			return fmt.Errorf("invalid signature")
		}

		mac := hmac.New(sha256.New, secret)
		_, _ = mac.Write(body)
		expected := mac.Sum(nil)
		if !hmac.Equal(gotSig, expected) {
			return fmt.Errorf("signature mismatch")
		}
		return nil
	case providerGitLab:
		got := strings.TrimSpace(h.Get(headerGitlabToken))
		if got == "" {
			return fmt.Errorf("missing %s", headerGitlabToken)
		}
		// Exact match, constant-time compare.
		if !hmac.Equal([]byte(got), []byte(strings.TrimSpace(string(secret)))) {
			return fmt.Errorf("token mismatch")
		}
		return nil
	default:
		return fmt.Errorf("unknown provider")
	}
}

type pushEvent struct {
	RepoURL  string
	Ref      string
	AfterSHA string
}

// Minimal payload structs (only fields we need).
type githubPushPayload struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Repository struct {
		CloneURL string `json:"clone_url"`
		HTMLURL  string `json:"html_url"`
	} `json:"repository"`
}

type gitlabPushPayload struct {
	Ref     string `json:"ref"`
	After   string `json:"after"`
	Project struct {
		GitHTTPURL string `json:"git_http_url"`
		WebURL     string `json:"web_url"`
	} `json:"project"`
}

func parsePushEvent(p provider, body []byte) (*pushEvent, error) {
	switch p {
	case providerGitHub:
		var pl githubPushPayload
		if err := json.Unmarshal(body, &pl); err != nil {
			return nil, err
		}
		repo := strings.TrimSpace(pl.Repository.CloneURL)
		if repo == "" {
			repo = strings.TrimSpace(pl.Repository.HTMLURL)
		}
		if repo == "" || strings.TrimSpace(pl.Ref) == "" {
			return nil, fmt.Errorf("missing required fields")
		}
		return &pushEvent{RepoURL: repo, Ref: pl.Ref, AfterSHA: pl.After}, nil
	case providerGitLab:
		var pl gitlabPushPayload
		if err := json.Unmarshal(body, &pl); err != nil {
			return nil, err
		}
		repo := strings.TrimSpace(pl.Project.GitHTTPURL)
		if repo == "" {
			repo = strings.TrimSpace(pl.Project.WebURL)
		}
		if repo == "" || strings.TrimSpace(pl.Ref) == "" {
			return nil, fmt.Errorf("missing required fields")
		}
		return &pushEvent{RepoURL: repo, Ref: pl.Ref, AfterSHA: pl.After}, nil
	default:
		return nil, fmt.Errorf("unknown provider")
	}
}

func normalizeRepoURL(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimRight(s, "/")
	return s
}

func (h *Handler) findMatchingCappBuilds(ctx context.Context, ev *pushEvent) ([]rcs.CappBuild, error) {
	var list rcs.CappBuildList
	if err := h.Client.List(ctx, &list); err != nil {
		return nil, err
	}

	wantRepo := normalizeRepoURL(ev.RepoURL)

	var out []rcs.CappBuild
	for _, cb := range list.Items {
		cb := cb
		if cb.Spec.Rebuild == nil || cb.Spec.Rebuild.Mode != rcs.CappBuildRebuildModeOnCommit {
			continue
		}
		if cb.Spec.Source.Type != rcs.CappBuildSourceTypeGit {
			continue
		}
		if cb.Spec.Source.Git.URL == "" {
			continue
		}
		if cb.Spec.Source.Git.Revision == "" {
			// Phase 10: require explicit tracked branch for webhook matching.
			continue
		}

		if normalizeRepoURL(cb.Spec.Source.Git.URL) != wantRepo {
			continue
		}
		if ev.Ref != "refs/heads/"+cb.Spec.Source.Git.Revision {
			continue
		}

		out = append(out, cb)
	}
	return out, nil
}

func (h *Handler) findCandidateCappBuilds(ctx context.Context, ev *pushEvent) ([]rcs.CappBuild, error) {
	// Same as findMatchingCappBuilds but renamed for clarity: “candidate” means
	// matching repo/ref (untrusted payload), before secret verification.
	return h.findMatchingCappBuilds(ctx, ev)
}
```

Notes:
- The handler purposefully does **no BuildRun creation**; it only records `status.onCommit.pending`.
- Debounce / rate limit defaults are enforced in reconciliation (not in the handler).

### 2.2) Wire the handler in `cmd/main.go`

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/cmd/main.go`

Add import:

```go
gitwebhook "github.com/dana-team/container-app-operator/internal/webhook/git"
```

Then, inside the existing `ENABLE_WEBHOOKS` block, register the endpoint:

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/cmd/main.go`

```go
hookServer.Register(gitwebhook.Path(), &gitwebhook.Handler{
	Client:        mgr.GetClient(),
	EventRecorder: mgr.GetEventRecorderFor("git-webhook"),
})
```

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

// reconcileOnCommitBuildRun enforces debounce/rate-limit/one-active-build and creates a BuildRun
// when a pending trigger is ready.
//
// Returns:
// - selected BuildRun to use for status mapping (may be an existing active run)
// - optional requeueAfter for debounce/rate-limit timers
func (r *CappBuildReconciler) reconcileOnCommitBuildRun(
	ctx context.Context,
	cb *rcs.CappBuild,
) (*shipwright.BuildRun, *time.Duration, error) {
	if cb.Spec.Rebuild == nil || cb.Spec.Rebuild.Mode != rcs.CappBuildRebuildModeOnCommit {
		return nil, nil, nil
	}
	if cb.Status.OnCommit == nil || cb.Status.OnCommit.Pending == nil {
		return nil, nil, nil
	}

	cfg, err := capputils.GetCappConfig(r.Client)
	if err != nil || cfg.Spec.CappBuild == nil || cfg.Spec.CappBuild.OnCommit == nil || !cfg.Spec.CappBuild.OnCommit.Enabled {
		_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonOnCommitDisabled, "OnCommit rebuilds are disabled by policy")
		return nil, nil, nil
	}

	// Debounce: wait until pending is "old enough".
	receivedAt := cb.Status.OnCommit.Pending.ReceivedAt.Time
	if !receivedAt.IsZero() {
		if remaining := time.Until(receivedAt.Add(onCommitDebounce)); remaining > 0 {
			return nil, &remaining, nil
		}
	}

	// Rate limiting: enforce minimum interval between triggers.
	if cb.Status.OnCommit.LastTriggeredBuildRun != nil && !cb.Status.OnCommit.LastTriggeredBuildRun.TriggeredAt.IsZero() {
		last := cb.Status.OnCommit.LastTriggeredBuildRun.TriggeredAt.Time
		if remaining := time.Until(last.Add(onCommitMinInterval)); remaining > 0 {
			return nil, &remaining, nil
		}
	}

	// One-active-build: if the last observed BuildRun is still running/pending, do not start another.
	if cb.Status.LastBuildRunRef != "" {
		active, err := r.getOwnedBuildRun(ctx, cb, cb.Status.LastBuildRunRef)
		if err == nil && isBuildRunActive(active) {
			// Keep pending; BuildRun watch will re-trigger reconcile when it completes.
			return active, nil, nil
		}
	}

	// Accept trigger for execution: bump counter and create BuildRun name based on it.
	if cb.Status.OnCommit.TriggerCounter < 0 {
		cb.Status.OnCommit.TriggerCounter = 0
	}
	cb.Status.OnCommit.TriggerCounter++
	counter := cb.Status.OnCommit.TriggerCounter

	br, err := r.reconcileBuildRunOnCommit(ctx, cb, counter)
	if err != nil {
		return nil, nil, err
	}

	// Mark trigger consumed.
	orig := cb.DeepCopy()
	cb.Status.OnCommit.LastTriggeredBuildRun = &rcs.CappBuildOnCommitLastTriggered{
		Name:       br.Name,
		TriggeredAt: metav1.Now(),
	}
	cb.Status.OnCommit.Pending = nil
	if err := r.Status().Patch(ctx, cb, client.MergeFrom(orig)); err != nil {
		return nil, nil, err
	}

	return br, nil, nil
}

func (r *CappBuildReconciler) getOwnedBuildRun(ctx context.Context, cb *rcs.CappBuild, name string) (*shipwright.BuildRun, error) {
	br := &shipwright.BuildRun{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: cb.Namespace, Name: name}, br); err != nil {
		return nil, err
	}
	if !metav1.IsControlledBy(br, cb) {
		return nil, fmt.Errorf("buildrun %s/%s is not owned by cappbuild %s/%s", br.Namespace, br.Name, cb.Namespace, cb.Name)
	}
	return br, nil
}

func isBuildRunActive(br *shipwright.BuildRun) bool {
	cond := br.Status.GetCondition(shipwright.Succeeded)
	if cond == nil {
		return true
	}
	switch cond.GetStatus() {
	case "True", "False":
		return false
	default:
		return true
	}
}
```

### 3.3) Add BuildRun creation for OnCommit counter-based names

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/buildrun.go`

Add these helpers next to the existing ones:

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/buildrun.go`

```go
func buildRunNameForOnCommit(cb *rcs.CappBuild, counter int64) string {
	return fmt.Sprintf("%s-buildrun-oncommit-%d", cb.Name, counter)
}

func newBuildRunOnCommit(cb *rcs.CappBuild, counter int64) *shipwright.BuildRun {
	buildName := buildNameFor(cb)

	return &shipwright.BuildRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildRunNameForOnCommit(cb, counter),
			Namespace: cb.Namespace,
			Labels: map[string]string{
				rcs.GroupVersion.Group + "/parent-cappbuild": cb.Name,
				"rcs.dana.io/build-trigger":                 "oncommit",
			},
		},
		Spec: shipwright.BuildRunSpec{
			Build: shipwright.ReferencedBuild{
				Name: &buildName,
			},
		},
	}
}

func (r *CappBuildReconciler) reconcileBuildRunOnCommit(ctx context.Context, cb *rcs.CappBuild, counter int64) (*shipwright.BuildRun, error) {
	desired := newBuildRunOnCommit(cb, counter)

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
if br, requeueAfter, err := r.reconcileOnCommitBuildRun(ctx, cb); err != nil {
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

Create:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/webhook/git/handler_test.go`

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/webhook/git/handler_test.go`

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

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	capputils "github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
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
	return fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&rcs.CappBuild{}).
		WithObjects(objs...).
		Build()
}

func newEnabledCappConfig() *rcs.CappConfig {
	return &rcs.CappConfig{
		ObjectMeta: metav1.ObjectMeta{Name: capputils.CappConfigName, Namespace: capputils.CappNS},
		Spec: rcs.CappConfigSpec{
			CappBuild: &rcs.CappBuildConfig{
				ClusterBuildStrategy: rcs.CappBuildClusterStrategyConfig{
					BuildFile: rcs.CappBuildFileStrategyConfig{Present: "p", Absent: "a"},
				},
				OnCommit: &rcs.CappBuildOnCommit{Enabled: true},
			},
		},
	}
}

func TestGitHubWebhookRejectsMissingSignature(t *testing.T) {
	c := testClient(t,
		newEnabledCappConfig(),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "wh", Namespace: "ns"}, Data: map[string][]byte{"k": []byte("s")}},
	)
	h := &Handler{Client: c}

	req := httptest.NewRequest(http.MethodPost, Path(), bytes.NewBufferString(`{"ref":"refs/heads/main","after":"abc","repository":{"clone_url":"https://example/repo.git"}}`))
	req.Header.Set(headerGithubEvent, githubPushEvent)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req.WithContext(context.Background()))
	require.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestGitLabWebhookAcceptsValidTokenAndPatchesMatchingCappBuild(t *testing.T) {
	cb := &rcs.CappBuild{
		ObjectMeta: metav1.ObjectMeta{Name: "cb", Namespace: "ns"},
		Spec: rcs.CappBuildSpec{
			OnCommit: &rcs.CappBuildOnCommit{
				WebhookSecretRef: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "wh"},
					Key:                  "k",
				},
			},
			Source: rcs.CappBuildSource{Type: rcs.CappBuildSourceTypeGit, Git: rcs.CappBuildGitSource{URL: "https://gitlab.example/group/repo.git", Revision: "main"}},
			BuildFile: rcs.CappBuildFile{Mode: rcs.CappBuildFileModeAbsent},
			Output: rcs.CappBuildOutput{Image: "registry.example.com/team/app:v1"},
			Rebuild: &rcs.CappBuildRebuild{Mode: rcs.CappBuildRebuildModeOnCommit},
		},
	}

	c := testClient(t,
		newEnabledCappConfig(),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "wh", Namespace: cb.Namespace}, Data: map[string][]byte{"k": []byte("token")}},
		cb,
	)
	h := &Handler{Client: c}

	body := `{"ref":"refs/heads/main","after":"abc","project":{"git_http_url":"https://gitlab.example/group/repo.git"}}`
	req := httptest.NewRequest(http.MethodPost, Path(), bytes.NewBufferString(body))
	req.Header.Set(headerGitlabEvent, gitlabPushEvent)
	req.Header.Set(headerGitlabToken, "token")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req.WithContext(context.Background()))
	require.Equal(t, http.StatusAccepted, rr.Code)

	updated := &rcs.CappBuild{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(cb), updated))
	require.NotNil(t, updated.Status.OnCommit)
	require.NotNil(t, updated.Status.OnCommit.Pending)
	require.Equal(t, "refs/heads/main", updated.Status.OnCommit.Pending.Ref)
	require.Equal(t, "abc", updated.Status.OnCommit.Pending.AfterSHA)
}

func TestGitHubWebhookAcceptsValidSig256(t *testing.T) {
	secret := []byte("s3cr3t")

	cb := &rcs.CappBuild{
		ObjectMeta: metav1.ObjectMeta{Name: "cb", Namespace: "ns"},
		Spec: rcs.CappBuildSpec{
			OnCommit: &rcs.CappBuildOnCommit{
				WebhookSecretRef: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "wh"},
					Key:                  "k",
				},
			},
			Source: rcs.CappBuildSource{Type: rcs.CappBuildSourceTypeGit, Git: rcs.CappBuildGitSource{URL: "https://github.com/org/repo", Revision: "main"}},
			BuildFile: rcs.CappBuildFile{Mode: rcs.CappBuildFileModeAbsent},
			Output: rcs.CappBuildOutput{Image: "registry.example.com/team/app:v1"},
			Rebuild: &rcs.CappBuildRebuild{Mode: rcs.CappBuildRebuildModeOnCommit},
		},
	}

	c := testClient(t,
		newEnabledCappConfig(),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "wh", Namespace: cb.Namespace}, Data: map[string][]byte{"k": secret}},
		cb,
	)
	h := &Handler{Client: c}

	body := []byte(`{"ref":"refs/heads/main","after":"abc","repository":{"html_url":"https://github.com/org/repo"}}`)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	sig := githubSig256Prefix + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, Path(), bytes.NewReader(body))
	req.Header.Set(headerGithubEvent, githubPushEvent)
	req.Header.Set(headerGithubSig256, sig)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req.WithContext(context.Background()))
	require.Equal(t, http.StatusAccepted, rr.Code)
}
```

### 4.2) Controller tests for debounce / counter BuildRuns

Create:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/oncommit_test.go`

Implement unit tests around:
- debounce “requeueAfter”
- rate-limit “requeueAfter”
- counter increment + BuildRun name
- one-active-build blocks a new BuildRun (pending remains)

(Keep tests focused on stable contracts; don’t assert message strings.)

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

