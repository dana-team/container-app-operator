# Shipwright Phase 1 — Capp prereq target (runbook)

Goal: update the Capp repo so `make prereq` installs **Tekton Pipelines** and **Shipwright Build** (with readiness waits), managed centrally in the prereq Helmfile.

## What you change
- `charts/capp-prereq-helmfile.yaml`: add 2 releases (Tekton + Shipwright) implemented via `kubectl` hooks.
- `Makefile`: wrap Helmfile apply/destroy to create an **ephemeral hook chart directory** (unique per run) and export it as `HELMFILE_HOOK_CHART_DIR`.

## 1) Update `charts/capp-prereq-helmfile.yaml`
Add these releases under `releases:` (Tekton first, Shipwright second with `needs`).

They require Helmfile to receive a chart path via env var:
- `chart: {{ requiredEnv "HELMFILE_HOOK_CHART_DIR" | quote }}`

They support pinning via env vars:
- `TEKTON_PIPELINES_VERSION`
- `SHIPWRIGHT_BUILD_VERSION`

Defaults are defined in `Makefile` and passed into Helmfile automatically. Override them by running:
- `make prereq TEKTON_PIPELINES_VERSION=... SHIPWRIGHT_BUILD_VERSION=...`

Paste (and adjust placement as needed):

```yaml
{{- $tektonURL := printf "https://storage.googleapis.com/tekton-releases/pipeline/%s/release.yaml" (requiredEnv "TEKTON_PIPELINES_VERSION") -}}
{{- $shipwrightURL := printf "https://github.com/shipwright-io/build/releases/download/%s/release.yaml" (requiredEnv "SHIPWRIGHT_BUILD_VERSION") -}}

  # Shipwright build-from-source prerequisites (manifest-based, centrally managed by helmfile)
  - name: tekton-pipelines
    namespace: tekton-pipelines
    createNamespace: true
    chart: {{ requiredEnv "HELMFILE_HOOK_CHART_DIR" | quote }}
    hooks:
      - events: ["presync"]
        command: kubectl
        args:
          - apply
          - -f
          - {{ $tektonURL | quote }}
      - events: ["postsync"]
        command: kubectl
        args:
          - -n
          - tekton-pipelines
          - wait
          - deploy
          - --all
          - --for=condition=Available
          - --timeout=10m
      - events: ["postuninstall"]
        command: kubectl
        args:
          - delete
          - -f
          - {{ $tektonURL | quote }}
          - --ignore-not-found=true

  - name: shipwright-build
    namespace: shipwright-build
    createNamespace: true
    chart: {{ requiredEnv "HELMFILE_HOOK_CHART_DIR" | quote }}
    needs:
      - tekton-pipelines/tekton-pipelines
    hooks:
      - events: ["presync"]
        command: kubectl
        args:
          - apply
          - --server-side=true
          - -f
          - {{ $shipwrightURL | quote }}
      - events: ["postsync"]
        command: kubectl
        args:
          - -n
          - shipwright-build
          - wait
          - deploy
          - --all
          - --for=condition=Available
          - --timeout=10m
      - events: ["postuninstall"]
        command: kubectl
        args:
          - delete
          - -f
          - {{ $shipwrightURL | quote }}
          - --ignore-not-found=true
```

## 2) Update `Makefile` to create a per-run hook chart dir (mktemp)
Helmfile requires a chart per release, but these releases are “manifest-only”. We create a noop chart directory at a fixed path and export it once so Helmfile templates can read it via `requiredEnv`.

In `Makefile`, update **both** targets:
- `install-prereq-helmfile`
- `uninstall-prereq-helmfile`

Also declare the version variables (defaults) in the **Capp prerequisites** section (near `PREREQ_HELMFILE ?= ...`):

```make
# Export defaults so helmfile templates can read them via `requiredEnv`
export TEKTON_PIPELINES_VERSION ?= v0.56.0
export SHIPWRIGHT_BUILD_VERSION ?= v0.13.0

# Hook chart path used by the helmfile releases (unique per `make` invocation)
export HELMFILE_HOOK_CHART_DIR ?= $(shell mktemp -d)
```

Wrap the hook-chart creation in a **private make target**, and make the public targets depend on it.

Add:

```make
.PHONY: _prereq-hook-chart
_prereq-hook-chart:
	@mkdir -p "$(HELMFILE_HOOK_CHART_DIR)/templates"; \
	printf '%s\n' \
	  'apiVersion: v2' \
	  'name: helmfile-hook-chart' \
	  'version: 0.1.0' \
	  > "$(HELMFILE_HOOK_CHART_DIR)/Chart.yaml"; \
	: > "$(HELMFILE_HOOK_CHART_DIR)/templates/_hook.yaml"
```

Then refactor:

```make
.PHONY: install-prereq-helmfile
install-prereq-helmfile: helmfile helm helm-plugins _prereq-hook-chart
	@${HELMFILE} apply -f $(PREREQ_HELMFILE) \
	  --state-values-set providerDNSRealmName=${PROVIDER_DNS_REALM} \
	  --state-values-set providerDNSKDCName=${PROVIDER_DNS_KDC} \
	  --state-values-set providerDNSPolicy=${PROVIDER_DNS_POLICY} \
	  --state-values-set providerDNSNameservers=${PROVIDER_DNS_NAMESERVER} \
	  --state-values-set providerDNSUsername=${PROVIDER_DNS_USERNAME} \
	  --state-values-set providerDNSPassword=${PROVIDER_DNS_PASSWORD}

.PHONY: uninstall-prereq-helmfile
uninstall-prereq-helmfile: helmfile helm helm-plugins _prereq-hook-chart
	@${HELMFILE} destroy -f $(PREREQ_HELMFILE)
```

## 3) Run

```bash
make prereq
```

## 4) Verify

```bash
kubectl get crd pipelineruns.tekton.dev
kubectl get crd builds.shipwright.io buildruns.shipwright.io clusterbuildstrategies.shipwright.io
kubectl -n tekton-pipelines get deploy
kubectl -n shipwright-build get deploy
```
