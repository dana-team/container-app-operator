# Build-from-Source for `Capp` (High-Level Plan)

## Prerequisites
- This feature requires **Tekton** as a `Capp` prerequisite.
- The build implementation uses the **Buildpacks** project to turn source code into a runnable container image.

## What users get
`Capp` can be created with **a code source**. The platform will:
- Build a container image using **Buildpacks**
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
- **Platform configuration**: define defaults and policy centrally via `CappConfig`.
- **Build execution**:
  - The controller will **only create `PipelineRun` instances** (the “run” of a build).
  - We ship **Tekton Task/Pipeline CRs** as a **versioned OCI image bundle**.
  - The OCI bundle is built/published via **GitHub CI** as a `Capp` build-artifact image.
- **Rebuild on commit**: Tekton Triggers listens for SCM webhooks and starts a new build for the commit; the operator then deploys the resulting image.

## Advantages
- **Simple user experience**: users specify source only.
- **Implementation flexibility**: pipelines can change over time without forcing app teams to change their `Capp`.
- **Community tasks**: reuse **Tekton Catalog** tasks.

## Disadvantages
- **More prerequisites**: requires `Capp` operator to depend on the **Tekton operator** as an additional platform dependency.
- **Upstream lifecycle risk**: relying on shared tasks requires version pinning and ongoing maintenance.

## Community task references

- Git fetch/clone:
  - [Tekton Catalog - git-clone](https://github.com/tektoncd/catalog/tree/main/task/git-clone)
- Buildpacks (Buildpacks “phases” can be expressed inside our bundled pipeline even if upstream splits change):
  - [Tekton Catalog - buildpacks-phases](https://github.com/tektoncd/catalog/tree/main/task/buildpacks-phases)


