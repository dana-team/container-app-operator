# Implementation — extend `CappConfig.spec.cappBuild`

## 1) Update API types

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/api/v1alpha1/cappconfig_types.go`

Paste the field **inside** `type CappConfigSpec struct { ... }` (recommended:
right after `InvalidHostnamePatterns`):

```go
	// +optional
	// CappBuild holds platform defaults/policy for the CappBuild feature.
	CappBuild *CappBuildConfig `json:"cappBuild,omitempty"`
```

Paste the type definitions **below the other spec types** (recommended: right
after `type AutoscaleConfig struct { ... }` and before `type CappConfigStatus`):

```go
type CappBuildConfig struct {
	// ClusterBuildStrategy holds platform defaults for selecting a build strategy.
	// This is required when cappBuild is configured, since it is not user-facing.
	ClusterBuildStrategy CappBuildClusterStrategyConfig `json:"clusterBuildStrategy"`
}

type CappBuildClusterStrategyConfig struct {
	// BuildFile holds strategy defaults for selecting a strategy based on whether a
	// build file indicator is present (example: Dockerfile).
	BuildFile CappBuildBuildFileStrategyConfig `json:"buildFile"`
}

type CappBuildBuildFileStrategyConfig struct {
	// Present is the strategy name to use when the source indicates a file-based
	// build (example: "Dockerfile").
	// +kubebuilder:validation:MinLength=1
	Present string `json:"present"`

	// Absent is the strategy name to use when the source does not indicate a
	// file-based build.
	// +kubebuilder:validation:MinLength=1
	Absent string `json:"absent"`
}
```

Note: Go doc comments here become CRD OpenAPI `description` via `controller-gen`.

## 2) Regenerate generated code + CRDs

```bash
make manifests
make generate
make fmt
```

## 3) (Optional) Render into the Helm chart

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/charts/container-app-operator/values.yaml`
- `/home/sbahar/projects/ps/dana-team/container-app-operator/charts/container-app-operator/templates/cappconfig.yaml`

Add values under `config:` (example; keep commented by default if you want):

```yaml
  cappBuild:
    clusterBuildStrategy:
      buildFile:
        # present is required when cappBuild is configured.
        present: "strategy-for-buildfile"
        # absent is required when cappBuild is configured.
        absent: "strategy-for-non-buildfile"
```

Template snippet under `spec:`:

```yaml
  {{- with .Values.config.cappBuild }}
  cappBuild:
    clusterBuildStrategy:
      buildFile:
        present: {{ required "config.cappBuild.clusterBuildStrategy.buildFile.present is required" .clusterBuildStrategy.buildFile.present | quote }}
        absent: {{ required "config.cappBuild.clusterBuildStrategy.buildFile.absent is required" .clusterBuildStrategy.buildFile.absent | quote }}
  {{- end }}
```

## 4) Keep chart CRDs in sync

After `make manifests`, copy:
- `config/crd/bases/rcs.dana.io_cappconfigs.yaml`

Into:
- `charts/container-app-operator/crds/rcs.dana.io_cappconfigs.yaml`

## 5) Sanity checks

```bash
make lint
make test
```
