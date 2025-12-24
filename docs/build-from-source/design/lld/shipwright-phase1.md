# Build-from-Source for `Capp` using `Shipwright` — Phase 1 (Low-Level Plan)

## Scope
Phase 1 only adds **platform prerequisites** for the Shipwright-based build-from-source feature:
- **Tekton Pipelines**
- **Shipwright Build**

## Goals
- Make Tekton + Shipwright installation and verification a supported, repeatable operator prerequisite.
- Provide clear operator feedback when prerequisites are missing.

## Deliverables
- Add Tekton + Shipwright prereq installation as **Make targets**:
  - `install-tekton`: Installs Tekton Pipelines directly from upstream manifests with a readiness wait.
  - `install-shipwright`: Installs Shipwright Build (requires Tekton) from upstream manifests, configures webhook certificates via `cert-manager`, and enables CA injection for CRDs.
  - Integration into `make prereq` and `make prereq-openshift`.

## Reference (installation)
- Shipwright installation: [Getting Started - Installation](https://shipwright.io/docs/getting-started/installation/)


