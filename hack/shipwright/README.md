## Shipwright (vendored manifests)

Used by `make install-shipwright`.

- `certs.yaml`: cert-manager webhook certs for Shipwright Build.
- `strategies.yaml`: only:
  - `buildah-strategy-managed-push` -> `spec.cappBuild.clusterBuildStrategy.buildFile.present`
  - `buildpacks-v3` -> `spec.cappBuild.clusterBuildStrategy.buildFile.absent`

