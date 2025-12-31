# Implementation — `CappBuild` reconcile start + strategy selection

## 0) Add Shipwright Go types (match prereq version)

Run:

```bash
cd /home/sbahar/projects/ps/dana-team/container-app-operator
go get github.com/shipwright-io/build@v0.17.0
go mod tidy
```

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/cmd/main.go`

Add import:

```go
shipwrightv1beta1 "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
```

Add to `init()`:

```go
utilruntime.Must(shipwrightv1beta1.AddToScheme(scheme))
```

## 1) Add condition/reason constants

Create:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/conditions.go`

```go
package controllers

import "errors"

const (
	TypeReady = "Ready"

	ReasonBuildReconcileFailed = "BuildReconcileFailed"
	ReasonBuildConflict        = "BuildConflict"
	ReasonMissingPolicy        = "MissingPolicy"
	ReasonSourceAccessFailed   = "SourceAccessFailed"
)

var ErrBuildConflict = errors.New("build conflict")
```

## 2) Add `Build` helpers (typed Shipwright `Build`)

Create:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/build.go`

```go
package controllers

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	shipwrightv1beta1 "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
)

func deriveBuildName(cb *cappv1alpha1.CappBuild) string {
	return cb.Name + "-build"
}

func (r *CappBuildReconciler) patchReadyCondition(
	ctx context.Context,
	cb *cappv1alpha1.CappBuild,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	orig := cb.DeepCopy()

	meta.SetStatusCondition(&cb.Status.Conditions, metav1.Condition{
		Type:               TypeReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: cb.Generation,
	})

	return r.Status().Patch(ctx, cb, client.MergeFrom(orig))
}

func (r *CappBuildReconciler) getBuild(
	ctx context.Context,
	cb *cappv1alpha1.CappBuild,
) (*shipwrightv1beta1.Build, bool, error) {
	key := types.NamespacedName{Namespace: cb.Namespace, Name: deriveBuildName(cb)}
	build := &shipwrightv1beta1.Build{}

	if err := r.Get(ctx, key, build); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	if !metav1.IsControlledBy(build, cb) {
		return nil, false, fmt.Errorf("%w: %s exists but is not owned by CappBuild %s/%s",
			ErrBuildConflict, key.String(), cb.Namespace, cb.Name)
	}
	return build, true, nil
}

func (r *CappBuildReconciler) newBuild(
	cb *cappv1alpha1.CappBuild,
	selectedStrategyName string,
) (*shipwrightv1beta1.Build, error) {
	// CappBuild CRD enforces spec.output.image (required, MinLength=1).
	outputImage := cb.Spec.Output.Image

	kind := shipwrightv1beta1.ClusterBuildStrategyKind

	build := &shipwrightv1beta1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deriveBuildName(cb),
			Namespace: cb.Namespace,
			Labels: map[string]string{
				cappv1alpha1.GroupVersion.Group + "/parent-cappbuild": cb.Name,
			},
		},
		Spec: shipwrightv1beta1.BuildSpec{
			Strategy: shipwrightv1beta1.Strategy{
				Name: selectedStrategyName,
				Kind: &kind,
			},
			Source: &shipwrightv1beta1.Source{
				Type: shipwrightv1beta1.GitType,
				Git: &shipwrightv1beta1.Git{
					URL: cb.Spec.Source.Git.URL,
				},
			},
			Output: shipwrightv1beta1.Image{
				Image: outputImage,
			},
		},
	}

	if cb.Spec.Source.Git.Revision != "" {
		rev := cb.Spec.Source.Git.Revision
		build.Spec.Source.Git.Revision = &rev
	}
	if cb.Spec.Source.ContextDir != "" {
		cd := cb.Spec.Source.ContextDir
		build.Spec.Source.ContextDir = &cd
	}
	if cb.Spec.Source.Git.CloneSecret != nil && cb.Spec.Source.Git.CloneSecret.Name != "" {
		sec := cb.Spec.Source.Git.CloneSecret.Name
		build.Spec.Source.Git.CloneSecret = &sec
	}
	if cb.Spec.Output.PushSecret != nil && cb.Spec.Output.PushSecret.Name != "" {
		ps := cb.Spec.Output.PushSecret.Name
		build.Spec.Output.PushSecret = &ps
	}

	if err := controllerutil.SetControllerReference(cb, build, r.Scheme); err != nil {
		return nil, err
	}
	return build, nil
}

// ensureBuild creates or patches the associated Build. Only sets Ready=False on failures.
func (r *CappBuildReconciler) ensureBuild(
	ctx context.Context,
	cb *cappv1alpha1.CappBuild,
	selectedStrategyName string,
) error {
	logger := log.FromContext(ctx)

	existing, ok, err := r.getBuild(ctx, cb)
	if err != nil {
		if errors.Is(err, ErrBuildConflict) {
			_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonBuildConflict, err.Error())
			return nil
		}
		_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonBuildReconcileFailed, err.Error())
		return err
	}

	desired, err := r.newBuild(cb, selectedStrategyName)
	if err != nil {
		_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonBuildReconcileFailed, err.Error())
		return err
	}

	if !ok {
		if err := r.Create(ctx, desired); err != nil {
			_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonBuildReconcileFailed, err.Error())
			return err
		}
		logger.Info("Created Shipwright Build", "name", desired.Name)
		return nil
	}

	orig := existing.DeepCopy()
	existing.Labels = desired.Labels
	existing.Spec = desired.Spec

	if err := r.Patch(ctx, existing, client.MergeFrom(orig)); err != nil {
		_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonBuildReconcileFailed, err.Error())
		return err
	}
	logger.Info("Patched Shipwright Build", "name", existing.Name)
	return nil
}
```

## 3) Update `CappBuild` controller reconcile flow

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/controller.go`

### 3.1) RBAC for Shipwright `Build`

Add:

```go
// +kubebuilder:rbac:groups=shipwright.io,resources=builds,verbs=get;list;watch;create;update;patch;delete
```

### 3.2) Wire: policy → probe → select strategy → ensureBuild

Replace the body after `ObservedGeneration` handling with:

```go
cfg, err := capputils.GetCappConfig(r.Client)
if err != nil || cfg.Spec.CappBuild == nil {
	_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonMissingPolicy, "CappConfig build policy is missing")
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

present := cfg.Spec.CappBuild.ClusterBuildStrategy.BuildFile.Present
absent := cfg.Spec.CappBuild.ClusterBuildStrategy.BuildFile.Absent

buildFilePresent, err := probeBuildFilePresence(ctx, cb)
if err != nil {
	_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonSourceAccessFailed, err.Error())
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

selected := absent
if buildFilePresent {
	selected = present
}

if err := r.ensureBuild(ctx, cb, selected); err != nil {
	// ensureBuild already set Ready=False on failures
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

return ctrl.Result{}, nil
```

Also add imports (adjust existing list):

```go
import (
	"time"

	capputils "github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)
```

## 4) Add build-file probe stub

Create:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/sourceprobe.go`

```go
package controllers

import (
	"context"
	"fmt"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
)

func probeBuildFilePresence(ctx context.Context, cb *cappv1alpha1.CappBuild) (bool, error) {
	return false, fmt.Errorf("probeBuildFilePresence not implemented")
}
```

## 5) Generate manifests + format

```bash
make fmt
make manifests
```


