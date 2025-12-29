# Build-from-Source for `Capp` using `Shipwright` — Phase 4 (Low-Level Plan)

## Scope
Phase 4 extends the existing `CappConfig` CRD to carry **build-from-source**
defaults/policy (platform-owned).

## Goals
- Keep build-from-source policy in a **typed/validated** API, consistent with
  existing operator configuration patterns.
- Ensure future `CappBuild` reconciliation can read policy from
  `CappConfig.spec.cappBuild`.

## Deliverables

### 1) API: extend `CappConfigSpec`
Add `spec.cappBuild` (optional) with minimal policy/defaults:

- `output.defaultImageRepo` (string; optional):
  - Default image repository used by the platform when deriving output image
    names for builds that are attached to a `Capp`.
  - Expected format: OCI image repository (no tag, no digest), e.g.
    `registry.example.com/team/apps` or `ghcr.io/dana-team/capps`.
- `clusterBuildStrategy` (object; **required when `cappBuild` is set**):
  - Platform-owned defaults for selecting a build strategy.
  - Rationale: the strategy is **not exposed on the per-build CR**, so it must be
    provided via platform configuration (users should not need to care about it).
- `clusterBuildStrategy.buildFile.present` (string; **required**):
  - Name of the strategy to use when the source indicates a file-based build
    (e.g. a `Dockerfile`) (platform default).
- `clusterBuildStrategy.buildFile.absent` (string; **required**):
  - Name of the strategy to use when the source does not indicate a file-based
    build (platform default).

Keep the whole `cappBuild` block optional for backwards compatibility.
When `cappBuild` is provided (or the feature is enabled via Helm), the chart
must render a valid `clusterBuildStrategy` configuration.

### 2) Helm: render config into the `CappConfig` instance
Extend chart values under `values.yaml` `config.*` and render them into
`templates/cappconfig.yaml` under `spec.cappBuild`.

Enablement of the feature (controller running) remains controlled by the chart
value `cappBuild.enabled`.


