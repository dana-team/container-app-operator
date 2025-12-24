# Shipwright Phase 2 — `CappBuild` API (implementation runbook)

Goal: implement the `CappBuild` CRD **schema only**, per `docs/build-from-source/design/lld/shipwright-phase2.md`.

## 1) Scaffold the API (kubebuilder)
From repo root:

```bash
kubebuilder create api --group rcs --version v1alpha1 --kind CappBuild --resource --controller=false
```

## 2) Implement the schema fields
Copy/paste the minimal Go schema below.

### Go schema (copy/paste)

```go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CappBuildSourceType string

const (
	CappBuildSourceTypeGit CappBuildSourceType = "Git"
)

type CappBuildRebuildMode string

const (
	CappBuildRebuildModeInitial  CappBuildRebuildMode = "Initial"
	CappBuildRebuildModeOnCommit CappBuildRebuildMode = "OnCommit"
)

type CappBuildSpec struct {
	Source CappBuildSource `json:"source"`

	// +optional
	CappRef *CappBuildCappRef `json:"cappRef,omitempty"`

	// +optional
	Rebuild *CappBuildRebuildSpec `json:"rebuild,omitempty"`

	// +optional
	Output *CappBuildOutputSpec `json:"output,omitempty"`
}

type CappBuildSource struct {
	// +kubebuilder:validation:Enum=Git
	Type CappBuildSourceType `json:"type"`

	Git CappBuildGitSource `json:"git"`

	// +optional
	SecretRef *CappBuildSecretRef `json:"secretRef,omitempty"`
}

type CappBuildGitSource struct {
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`

	// +optional
	Revision string `json:"revision,omitempty"`

	// +optional
	ContextDir string `json:"contextDir,omitempty"`
}

type CappBuildSecretRef struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

type CappBuildCappRef struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// +optional
	Namespace string `json:"namespace,omitempty"`
}

type CappBuildRebuildSpec struct {
	// +kubebuilder:validation:Enum=Initial;OnCommit
	Mode CappBuildRebuildMode `json:"mode"`
}

type CappBuildOutputSpec struct {
	// +optional
	Image string `json:"image,omitempty"`
}

type CappBuildStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional
	LatestImage string `json:"latestImage,omitempty"`

	// +optional
	LastBuildRunRef string `json:"lastBuildRunRef,omitempty"`

	// +optional
	LastAppliedCapp string `json:"lastAppliedCapp,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type CappBuild struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CappBuildSpec   `json:"spec,omitempty"`
	Status CappBuildStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type CappBuildList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CappBuild `json:"items"`
}
```

## 3) Generate CRDs + deepcopy

```bash
make generate manifests
```

Verify the CRD is generated under:
- `config/crd/bases/`

## 4) Add a sample
Add a `CappBuild` example YAML under:
- `config/samples/`

Create a new sample file:
- `config/samples/rcs_v1alpha1_cappbuild.yaml`

### Sample `CappBuild` (copy/paste)

```yaml
apiVersion: rcs.dana.io/v1alpha1
kind: CappBuild
metadata:
  name: demo-cappbuild
  namespace: default
spec:
  source:
    type: Git
    git:
      url: https://github.com/example/repo.git
      revision: main
      contextDir: .

  cappRef:
    name: demo-capp

  rebuild:
    mode: OnCommit

  output:
    image: ghcr.io/example/demo:latest
```

## 5) Wire the CRD into distribution artifacts
This phase is **schema-only**, but the CRD must be included wherever we ship CRDs:

- **Kustomize**: `make manifests` updates `config/crd/bases/` (ensure the generated `rcs.dana.io_cappbuilds.yaml` is present there).
- **Helm chart**: copy the generated CRD into the chart `crds/` directory so `helm install` applies it automatically:
  - `charts/container-app-operator/crds/rcs.dana.io_cappbuilds.yaml`



