# Build-from-Source for `Capp` using `Shipwright` — Phase 1 (Low-Level Plan)

## Scope
Phase 1 only adds **platform prerequisites** for the Shipwright-based build-from-source feature:
- **Tekton Pipelines**
- **Shipwright Build**

## Goals
- Make Tekton + Shipwright installation and verification a supported, repeatable operator prerequisite.
- Provide clear operator feedback when prerequisites are missing.

## Deliverables
- Add Tekton + Shipwright prereq installation to `charts/capp-prereq-helmfile.yaml` as **Helm chart releases**:
  - Tekton Pipelines: install via a Helm chart (packaging the upstream install as chart-managed resources).
  - Shipwright Build: install via an internal Helm chart that **vendors the official `release.yaml` manifests** (no functional changes), and is installed after Tekton.

## Reference (installation)
- Shipwright installation: [Getting Started - Installation](https://shipwright.io/docs/getting-started/installation/)


