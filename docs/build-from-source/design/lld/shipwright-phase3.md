# Build-from-Source for `Capp` using `Shipwright` — Phase 3 (Low-Level Plan)

## Scope
Scaffold a **`CappBuild` controller** using **kubebuilder** and wire it into the operator behind a Helm feature flag.

## Goals
- Add the controller skeleton (no build execution yet).
- Make it safe to deploy: **feature-gated** + required RBAC.

## Deliverables
- **Kubebuilder scaffolding**:
  - `kubebuilder create api --group rcs --version v1alpha1 --kind CappBuild --resource=false --controller=true`
- **Controller package/layout**:
  - Controller code lives under `internal/kinds/cappbuild/controllers`
  - Add `+kubebuilder:rbac` markers for `rcs.dana.io` `cappbuilds` + `cappbuilds/status`
  - Run `make manifests` to regenerate `config/` RBAC artifacts
- **Manager wiring (feature-gated)**:
  - Add an env var gate (e.g. `ENABLE_CAPPBUILD_CONTROLLER`) in `cmd/main.go`
  - Add Helm value `cappBuild.enabled` (default `false`) and propagate it into the manager `Deployment`
- **Helm RBAC**:
  - Update `charts/container-app-operator/templates/manager-rbac.yaml` to include `cappbuilds` permissions
- **Test stub**:
  - Add a minimal unit test skeleton for `CappBuildReconciler` (fake client)
