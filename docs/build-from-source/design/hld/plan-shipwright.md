# Build-from-Source for `Capp` using `Shipwright` (High-Level Plan)

## Prerequisites
- This feature requires **Shipwright Build** as a `Capp` prerequisite.
- Shipwright Build requires **Tekton Pipelines**.
- The platform provides build defaults and policy centrally.

## What users get
`Capp` can be created with **a code source**. The platform will:
- Build a container image from source
- Deploy the app using the produced image
- Rebuild automatically when new commits land

Users choose whether their source should be treated as **Dockerfile/Containerfile-based**
or **non-buildfile-based** (e.g., Buildpacks), and are responsible for ensuring the
selected mode matches their repository content.

## Design principle
Keep a stable contract:
- **Input**: “here is my source”
- **Output**: “here is the built image”

Everything in between is platform-owned and can evolve.

## High-level architecture
- **`CappBuild` API (New CRD)**: build-time resource (source + creds).
  - **Optional** `spec.cappRef`: when set, the built image is handed over to that `Capp`; when omitted, `CappBuild` is standalone (build-only).
  - **Required** `spec.output`: explicit output image target (registry/repo) to push to.
  - **Required** `spec.buildFile.mode`: user-selected strategy mode:
    - `Present`: use a Dockerfile/Containerfile-based strategy
    - `Absent`: use a non-buildfile-based strategy (e.g., Buildpacks)
  - **Optional rebuild trigger**: user intent can be expressed per `CappBuild` (e.g., manual-only vs on-commit), with defaults and guardrails enforced by platform policy.

## Multi-container `Capp` support (sidecars)
`Capp` embeds Knative’s `ConfigurationSpec`, which can include **multiple containers** (e.g., sidecars).

The build contract remains **source → single image**:
- Each successful `CappBuild` produces **one** image reference.
- When `spec.cappRef` is set, the `CappBuild` controller updates **exactly one** container image on the target `Capp`.

How the target container is selected:
- If `spec.cappRef.containerName` is set: update the container with that name in the `Capp` pod template.
- If `spec.cappRef.containerName` is omitted: update the **first** container in the `Capp` pod template (the “primary” container by convention).
- Sidecar images (additional containers) are expected to be managed independently (pinned to external images, separate build pipelines, etc.).

## Policy: image publishing
- `CappBuild.spec.output` is **required** (regardless of whether `spec.cappRef` is set).
- **Dedicated `CappBuild` controller**: reconciles `CappBuild`, creates/monitors Shipwright `Build`/`BuildRun`, and updates the target `Capp` image on success.
- **`Capp` API**: runtime resource; deploys the current image and reports runtime status.
- **Helm-gated feature**: enable/disable the `CappBuild` controller (and RBAC) via Helm values (e.g. `cappBuild.enabled`).
- **Platform configuration**: define defaults and policy centrally via `CappConfig`:
  - Build strategy selection, pinned versions, and policy constraints
  - Validation/policy for allowed `spec.output` targets (e.g., allowlist of registries, naming constraints)
  - Credentials + authorization model for source + registry access
  - Defaults and constraints for rebuild triggers
- **How Shipwright executes builds**:
  - `CappBuild` controller creates Shipwright `Build` and `BuildRun` resources.
  - Shipwright creates Tekton resources under the hood, so Tekton Pipelines must be installed/available.
  - The chosen `BuildStrategy`/`ClusterBuildStrategy` determines the build engine (Buildpacks/Kaniko/Buildah/…).
- **Strategy selection** (user-owned, no source probing):
  - If `CappBuild.spec.buildFile.mode=Present`, use the platform-configured Dockerfile/Containerfile-based `ClusterBuildStrategy`.
  - If `CappBuild.spec.buildFile.mode=Absent`, use the platform-configured non-buildfile-based `ClusterBuildStrategy`.
  - If the user-selected mode does not match the repository content, the build is expected to fail and Shipwright status becomes the source of truth.
- **Build execution (Shipwright Build)**:
  - The `CappBuild` operator requests builds using Shipwright primitives and tracks status:
    - A `Build` CR captures the “build definition” (source + output image + strategy).
    - A `BuildRun` CR represents each execution of the build (initial/manual run, or auto-triggered on new commits when enabled).
  - The `CappBuild` operator watches `BuildRun.status` to determine success/failure and obtains the produced image reference.
  - **Handover to Runtime**: Once a build succeeds, the `CappBuild` controller updates the target `Capp` with the new image reference.
- **Rebuild on commit (optional / policy-controlled)**:
  - If enabled, new commits trigger a new `BuildRun`, followed by `Capp` update.

## Advantages
- **Better separation of concerns**: Clear split between runtime (`Capp`) and build-time (`CappBuild`) resources and dependencies.
- **Standalone utility**: `CappBuild` can be used as a generic image building tool even without a `Capp`.
- **Simple controller**: avoids implementing source probing, git auth edge cases, and repo inspection behavior in the operator.
- **Kubernetes-native abstraction**: Shipwright provides a higher-level build API (`Build`/`BuildRun`) instead of wiring Tekton directly in the operator.
- **Strategy flexibility**: swap build engines (Buildpacks/Kaniko/Buildah/…) without changing the `CappBuild` contract (platform-owned).
- **Operator-owned contract**: we can evolve strategy choice, pinning, and defaults without changing app teams’ `Capp`s.

## Disadvantages
- **More prerequisites**: depends on **Shipwright Build** and **Tekton Pipelines** as additional platform dependencies.
- **Strategy lifecycle/versioning**: BuildStrategy/ClusterBuildStrategy definitions must be version-pinned and maintained (including any embedded Tekton steps/images).
- **Rebuild triggers are extra**: on-commit rebuild typically requires an additional webhook/triggering component beyond core Shipwright APIs.
- **User responsibility**: users must select the correct `spec.buildFile.mode`; mismatches lead to build failures.

## References
- Shipwright Build docs (Build/BuildRun): [Shipwright “Build” documentation](https://shipwright.io/docs/build/build/)
- Shipwright Build strategies overview: [Shipwright “Build Strategies”](https://shipwright.io/docs/build/buildstrategies/)
- Shipwright Build project: [shipwright-io/build](https://github.com/shipwright-io/build)


