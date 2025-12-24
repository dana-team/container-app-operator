# Build-from-Source for `Capp` using `Shipwright` (High-Level Plan)

## Prerequisites
- This feature requires **Shipwright Build** as a `Capp` prerequisite.
- Shipwright Build requires **Tekton Pipelines**.
- Builds run via **Shipwright strategies** (`BuildStrategy` / `ClusterBuildStrategy`).
- The platform provides build defaults and policy centrally.

## What users get
`Capp` can be created with **a code source**. The platform will:
- Build a container image from source (Shipwright-managed build)
- Deploy the app using the produced image
- Rebuild automatically when new commits land (optional / policy-controlled)

Users do **not** need to know (or choose) how the build is implemented.
They can optionally include a `Dockerfile` in the repo; the platform will auto-select the appropriate build strategy.

## Design principle
Keep a stable contract:
- **Input**: “here is my source”
- **Output**: “here is the built image”

Everything in between is platform-owned and can evolve.

## High-level architecture
- **`Capp` API**: add a simple `source` section and expose build/deploy progress in `status`.
- **Platform configuration**: define defaults and policy centrally via `CappConfig`:
  - Build strategy selection (e.g., Buildpacks vs Kaniko), pinned versions, and policy constraints
  - Image publishing target (registry/repo naming conventions)
  - Credentials + authorization model for source + registry access
  - Rebuild policy (on-commit / manual-only / platform-controlled)
- **How Shipwright executes builds**:
  - Shipwright creates Tekton resources under the hood, so Tekton Pipelines must be installed/available.
  - The chosen `BuildStrategy`/`ClusterBuildStrategy` determines the build engine (Buildpacks/Kaniko/Buildah/…).
- **Strategy auto-selection** (platform-owned):
  - If the source repo contains a `Dockerfile` (or other configured indicator), use a Dockerfile-based `ClusterBuildStrategy`.
  - Otherwise, default to the Buildpacks-based `ClusterBuildStrategy`.
  - This keeps the user contract stable (provide source), while still supporting teams that already maintain `Dockerfile`s.
- **Build execution (Shipwright Build)**:
  - The operator requests builds using Shipwright primitives and tracks status:
    - A `Build` CR captures the “build definition” (source + output image + strategy).
    - A `BuildRun` CR represents each execution of the build (per commit / per change / manual trigger).
  - The operator watches `BuildRun.status` to determine success/failure and obtains the produced image reference (digest/tag) for deployment.
  - Once a build succeeds, the operator deploys using the produced image.
- **Rebuild on commit (optional / policy-controlled)**:
  - If enabled, new commits trigger a new `BuildRun` (via an SCM webhook receiver / Tekton Triggers / other platform mechanism), followed by re-deploy.

## Advantages
- **Simple user experience**: users specify source only.
- **Kubernetes-native abstraction**: Shipwright provides a higher-level build API (`Build`/`BuildRun`) instead of wiring Tekton directly in the operator.
- **Strategy flexibility**: swap build engines (Buildpacks/Kaniko/Buildah/…) without changing the `Capp` contract (platform-owned).
- **Operator-owned contract**: we can evolve strategy choice, pinning, and defaults without changing app teams’ `Capp`s.

## Disadvantages
- **More prerequisites**: depends on **Shipwright Build** and **Tekton Pipelines** as additional platform dependencies.
- **Strategy lifecycle/versioning**: BuildStrategy/ClusterBuildStrategy definitions must be version-pinned and maintained (including any embedded Tekton steps/images).
- **Rebuild triggers are extra**: on-commit rebuild typically requires an additional webhook/triggering component beyond core Shipwright APIs.

## References
- Shipwright Build docs (Build/BuildRun): [Shipwright “Build” documentation](https://shipwright.io/docs/build/build/)
- Shipwright Build strategies overview: [Shipwright “Build Strategies”](https://shipwright.io/docs/build/buildstrategies/)
- Shipwright Build project: [shipwright-io/build](https://github.com/shipwright-io/build)


