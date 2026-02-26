# Build-from-Source for `Capp` using `Shipwright` — Phase 10 (Low-Level Plan)

## Scope
Phase 10 adds **automatic rebuilds on git commits** for `CappBuild` resources with:
- `spec.rebuild.mode=OnCommit`

This phase introduces a **git webhook ingestion endpoint** and the controller logic/policy to:
- accept/verify push events
- match events to `CappBuild` (repo + branch)
- apply triggering policy (debounce, rate limits, one active build)
- trigger a new Shipwright `BuildRun`
- on successful build, update `CappBuild.status.latestImage`

## Goals
- Provide a stable user-facing flow: **push code → build is triggered → `CappBuild.status.latestImage` updates on success**.
- Ensure **safety**:
  - only matching `CappBuild` objects are eligible
  - webhook authenticity is verified
  - coalesce bursts of commits (debounce)
  - enforce **max 1 active build** per `CappBuild`
- Keep operator contracts low-cardinality and observable via:
  - `CappBuild.status.conditions` (stable reasons)
  - Events for per-webhook actions (accepted/rejected/debounced/limited)

## Non-goals
- Supporting every git provider/event schema in Phase 10. (Baseline: **push** events; add providers incrementally.)
- Building every intermediate commit in a push burst (we intentionally coalesce to “latest”).
- Providing a platform UI/API to *create* repository webhooks. (Users configure webhooks in their repo UI.)
- Global, cross-`CappBuild` concurrency limits (Phase 10 focuses on per-`CappBuild` safety gates).

## Deliverables

### 1) User-facing contract: enabling OnCommit rebuilds
- A `CappBuild` is eligible for webhook-triggered rebuilds when all are true:
  - `spec.rebuild.mode == OnCommit`
  - `spec.source.type == Git`
  - `spec.source.git.url` is set (required by API)
  - `spec.source.git.revision` is treated as the **tracked branch** (see matching rules below)

**Clarification (repo reality)**
- The current API uses rebuild modes `{Initial, OnCommit}`. Phase 10 treats `Initial` as “no automatic triggers”.

### 2) Webhook endpoint: request/response contract
Expose a platform endpoint (example): `https://capp.example.com/webhooks/git`
- **Method**: `POST`
- **Event**: `push`
- **Content-Type**: `application/json`

**Provider baseline (Phase 10)**
- Support **GitHub** and **GitLab** push webhooks.
- Provider detection is header-based:
  - GitHub: `X-GitHub-Event: push`
  - GitLab: `X-Gitlab-Event: Push Hook`

**Verification (webhook authenticity)**
- Secrets are **per-`CappBuild`** and must be configured by users in their repo webhook settings.
- GitHub verification:
  - Verify `X-Hub-Signature-256` using **HMAC-SHA256** over the raw request body.
- GitLab verification:
  - Verify the configured secret token by exact match against `X-Gitlab-Token`.
- Failure behavior:
  - invalid/missing signature → respond `401 Unauthorized` (and do not mutate any resources)
  - malformed JSON / missing required fields → `400 Bad Request`

**Success behavior**
- If the webhook is valid but no `CappBuild` matches: respond `202 Accepted` (no-op).
- If one or more `CappBuild` objects match: respond `202 Accepted` after enqueueing trigger intent.

**Operational guidance (platform)**
- The endpoint is served from the operator’s existing webhook service; the platform must expose it (Ingress/Route/LB) so git providers can reach it.

### 3) Event → `CappBuild` matching rules
Only `CappBuild` objects with `spec.rebuild.mode=OnCommit` are considered.

**Repo match**
- Compare the webhook “repository URL” to `spec.source.git.url`.
- Matching must be deterministic; Phase 10 uses a simple normalization:
  - trim a trailing `.git`
  - trim trailing `/`
  - otherwise treat URLs as exact strings (no DNS canonicalization)

**Repository URL extraction (provider-specific)**
- GitHub push payload:
  - Use `repository.clone_url` when present; otherwise fall back to `repository.html_url`.
- GitLab push payload:
  - Use `project.git_http_url` when present; otherwise fall back to `project.web_url`.

**Branch match**
- Webhook payload provides a ref like `refs/heads/main`.
- A `CappBuild` matches when:
  - `spec.source.git.revision` is set to a branch name (e.g., `main`), and
  - `ref == "refs/heads/<revision>"`

**Commit extraction (optional, provider-specific)**
- GitHub: `after` (SHA)
- GitLab: `after` (SHA)

**Rationale**
- This avoids accidental builds from tag pushes or non-branch revisions.
- It keeps matching low-surprise and policy-friendly.

### 4) Trigger persistence + debounce model
The webhook handler must not directly create `BuildRun` objects. Instead, it records **trigger intent** on the matched `CappBuild` and lets reconciliation apply policy and create `BuildRun` objects.

**Clarification (why not rely on `generation` / labels?)**
- `metadata.generation` increments on **spec** changes, not on `metadata.labels` / `metadata.annotations` updates.
- Commit SHAs are **high-cardinality** and should not be placed into labels.
- Therefore, webhook-triggered rebuilds must use a **separate persisted trigger token** (e.g., in `status`) rather than `generation`.

**Status fields (Phase 10 adds)**
Add an `onCommit` sub-structure under `CappBuild.status` (names are illustrative; keep them stable once introduced):
- `status.onCommit.lastReceived`:
  - `ref` (branch ref from webhook)
  - `afterSHA` (optional; provider-provided)
  - `receivedAt` (timestamp)
- `status.onCommit.pending` (optional):
  - same fields as `lastReceived`
  - represents the **latest coalesced** trigger waiting to run
- `status.onCommit.lastTriggeredBuildRun` (optional):
  - `name` (BuildRun name)
  - `triggeredAt` (timestamp)
- `status.onCommit.triggerCounter` (optional, integer):
  - monotonically increasing counter used to derive deterministic BuildRun names for webhook triggers

**Debounce policy**
- Maintain a per-`CappBuild` debounce window (default: **10s**).
- When multiple webhooks arrive inside the window:
  - update `status.onCommit.pending` to the newest event
  - do **not** start additional builds
- After the window expires, reconciliation triggers **one** build for the latest pending event.

### 5) Rate limiting + “one active build” policy
Apply platform safety policy per `CappBuild`:

**One active build**
- If the current `CappBuild` has an active Shipwright `BuildRun` (i.e., build result is not terminal):
  - do **not** create a new `BuildRun`
  - keep only the latest `status.onCommit.pending` event
  - rely on `BuildRun` watch events to trigger a follow-up reconcile when it completes

**Rate limiting**
- Enforce a minimum interval between triggers (default: **30s**).
- If a valid webhook arrives inside the interval:
  - record it as `pending`
  - schedule reconciliation after the remaining interval (avoid hot-looping)

**Where policy comes from**
- Extend `CappConfig.spec.cappBuild` with an optional `onCommit` policy block:
  - `enabled` (bool; default false)

**Defaults (Phase 10)**
- Keep debounce and rate limit values as controller defaults to avoid over-configuring early:
  - debounce window default: **10s**
  - min trigger interval default: **30s**

**Per-`CappBuild` secret reference**
- Each `CappBuild` that uses `spec.rebuild.mode=OnCommit` must specify an on-commit secret reference (e.g., `spec.onCommit.webhookSecretRef`) pointing to a Secret in the same namespace as the `CappBuild`.
- Webhook verification is done **per matched `CappBuild`**:
  - parse payload (untrusted) to extract repo + ref
  - find candidate `CappBuild`s by repo/ref and `rebuild.mode=OnCommit`
  - verify signature/token against each candidate’s secret
  - only if verification succeeds for a specific `CappBuild`, record trigger intent on that `CappBuild`

### 6) BuildRun triggering model (Shipwright)
When a `CappBuild` has a pending trigger and is allowed by policy:
- Ensure the associated Shipwright `Build` exists (existing phases).
- Create a new Shipwright `BuildRun` for the trigger.

**Naming / idempotency**
- Phase 10 must support multiple `BuildRun`s without changing `CappBuild.spec` (generation stays constant on webhook triggers).
- Use a deterministic, controller-owned trigger token, such as:
  - increment `status.onCommit.triggerCounter` when a trigger is accepted for execution
  - BuildRun name: `<cappBuild.name>-buildrun-oncommit-<triggerCounter>`
- If the expected `BuildRun` already exists and is owned, reconcile is idempotent.
- If the name exists but is not owned, treat as a conflict and stop (no requeue).

**Labels (for audit/debug)**
- Keep the existing parent label and add a low-cardinality trigger label:
  - `rcs.dana.io/build-trigger=oncommit`

### 7) Conditions + Events (stable operator contracts)
Phase 10 keeps the existing condition types:
- `Ready`
- `BuildSucceeded`

**New `Ready` reasons (stable, low-cardinality)**
- `OnCommitDisabled` (platform policy disabled)
- `WebhookSecretMissing` (policy refers to a missing Secret/key)

**Events (per webhook / per trigger, not a stable API)**
Emit Kubernetes Events for operator observability:
- `WebhookAccepted`
- `WebhookRejected`
- `WebhookNoMatch`
- `OnCommitDebounced`
- `OnCommitRateLimited`
- `OnCommitBuildTriggered`

### 8) Tests to add (unit + minimal e2e)
**Unit tests (authoritative)**
- Webhook verification:
  - missing/invalid signature rejected (no mutations)
  - valid signature + minimal payload accepted
- Matching:
  - repo normalization + branch/ref matching
  - only `rebuild.mode=OnCommit` is eligible
- Policy:
  - debounce coalesces N webhooks into one trigger
  - active BuildRun blocks new BuildRun creation but retains `pending`
  - rate limiting delays triggering and avoids hot-looping

**E2E tests (minimal, stable)**
- Verify that a webhook-triggered rebuild path can create a second `BuildRun` without spec changes:
  - create a `CappBuild` with `rebuild.mode=OnCommit`
  - simulate an accepted trigger intent via the same mechanism used by the webhook handler (status/annotation)
  - assert a new `BuildRun` is created and owned

