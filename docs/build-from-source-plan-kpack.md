# Build-from-Source for `Capp` using `kpack` (High-Level Plan)

## Prerequisites
- This feature requires **kpack** as a `Capp` prerequisite.
- The build implementation uses **Cloud Native Buildpacks (CNB)** to turn source code into a runnable container image (via kpack).
- The platform provides the build configuration (builder/registry access) centrally.

## What users get
`Capp` can be created with **a code source**. The platform will:
- Build a container image using **Buildpacks** (kpack-managed build)
- Deploy the app using the produced image
- Rebuild automatically when new commits land (optional / policy-controlled)

Users do **not** need to know how the build is implemented.

## Design principle
Keep a stable contract:
- **Input**: “here is my source”
- **Output**: “here is the built image”

Everything in between is platform-owned and can evolve.

## High-level architecture
- **`Capp` API**: add a simple `source` section and expose build/deploy progress in `status`.
- **Platform configuration**: define defaults and policy centrally via `CappConfig`:
  - Default builder configuration
  - Image publishing target (registry/repo)
  - Rebuild policy (on-commit / on-builder-update / manual-only)
- **Build execution (kpack)**:
  - The operator requests a build from the provided source using kpack and tracks build status.
  - Once a build succeeds, the operator deploys using the produced image.
- **Rebuild on commit (optional / policy-controlled)**:
  - If enabled, new commits trigger a rebuild and re-deploy (policy-controlled).

## Advantages
- **Simple user experience**: users specify source only.
- **Kubernetes-native & unprivileged**: kpack runs builds using standard Kubernetes primitives and pushes to a registry.
- **Standardized, opinionated builds**: consistent Buildpacks behavior across teams; supports incremental rebuilds and platform-driven updates.
- **Operator-owned contract**: we can evolve builder configuration and policies without changing the `Capp` contract.

## Disadvantages
- **More prerequisites**: requires `Capp` operator to depend on **kpack** as an additional platform dependency.

## References
- kpack integration overview: https://buildpacks.io/docs/for-platform-operators/how-to/integrate-ci/kpack/

