# Build-from-Source for `Capp` using `Shipwright` — Phase 2 (Low-Level Plan)

## Scope
Phase 2 introduces the **`CappBuild` API schema** (CRD contract) for build-from-source.

## Design goals (API)
- Keep build implementation details **platform-owned**.
- Provide a stable user contract for **source → image**.
- Support two rebuild modes: **manual/initial** and **auto rebuild on commit** (when enabled by policy).

## `CappBuild` API

### Group/Version/Kind
- **group**: `rcs.dana.io`
- **version**: `v1alpha1`
- **kind**: `CappBuild`

### `spec`

#### `spec.source` (required)
Defines the source location and revision to build.

- **`type`** (required): `Git` (initial scope)
- **`git`** (required when `type=Git`)
  - **`url`** (required): repository URL
  - **`revision`** (optional): branch/tag/sha; if omitted, build from the repo default branch/HEAD
  - **`contextDir`** (optional): subdirectory within the repo to use as build context
- **`authRef`** (optional): reference to credentials for accessing the repo
  - **`name`** (required if set): Secret name

Notes:

#### `spec.rebuild` (optional)
User intent for rebuild behavior. Defaults and constraints are enforced via platform policy (`CappConfig`).

- **`mode`** (optional): `Manual` | `OnCommit`

#### `spec.output` (optional)
Controls where the image is published. May be fully platform-owned.

- **`image`** (optional): output image repository reference (registry/repo). If omitted, the platform derives a target based on policy.

### `status`

#### `status.observedGeneration`
Standard reconciliation marker.

#### `status.conditions[]`
Kubernetes-style conditions. Minimum set:
- **`Ready`**: overall readiness of the build configuration
- **`BuildSucceeded`**: last build result

#### `status.latestImage`
The last produced image reference (string).

#### `status.lastBuildRunRef`
Reference to the last Shipwright `BuildRun` created for this `CappBuild`.

## Validation / semantics
- `spec.source.type` is required and must be supported.
- `spec.source.git.url` is required for `type=Git`.
- `spec.rebuild.mode` is limited to `{Manual, OnCommit}`.

## Example

```yaml
apiVersion: rcs.dana.io/v1alpha1
kind: CappBuild
metadata:
  name: my-image-build
spec:
  source:
    type: Git
    git:
      url: https://github.com/example/my-lib
      revision: v1.2.3
  output:
    image: registry.example.com/team/my-lib
  rebuild:
    mode: OnCommit
```


