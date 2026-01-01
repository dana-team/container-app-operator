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
shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
```

Add to `init()`:

```go
utilruntime.Must(shipwright.AddToScheme(scheme))
```

## 1) Add condition/reason constants

Create:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/conditions.go`

```go
package controllers

const (
	TypeReady = "Ready"

	ReasonBuildReconcileFailed = "BuildReconcileFailed"
	ReasonBuildConflict        = "BuildConflict"
	ReasonMissingPolicy        = "MissingPolicy"
	ReasonSourceAccessFailed   = "SourceAccessFailed"
)
```

## 2) Add `Build` helpers (typed Shipwright `Build`)

Create:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/build.go`

```go
package controllers

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
)

func buildNameFor(cb *rcs.CappBuild) string {
	return cb.Name + "-build"
}

func (r *CappBuildReconciler) patchReadyCondition(
	ctx context.Context,
	cb *rcs.CappBuild,
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

func (r *CappBuildReconciler) newBuild(
	cb *rcs.CappBuild,
	selectedStrategyName string,
) (*shipwright.Build, error) {
	kind := shipwright.ClusterBuildStrategyKind

	build := &shipwright.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildNameFor(cb),
			Namespace: cb.Namespace,
			Labels: map[string]string{
				rcs.GroupVersion.Group + "/parent-cappbuild": cb.Name,
			},
		},
		Spec: shipwright.BuildSpec{
			Strategy: shipwright.Strategy{
				Name: selectedStrategyName,
				Kind: &kind,
			},
			Source: &shipwright.Source{
				Type: shipwright.GitType,
				Git: &shipwright.Git{
					URL: cb.Spec.Source.Git.URL,
				},
			},
			Output: shipwright.Image{
				Image: cb.Spec.Output.Image,
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

	return build, nil
}

// reconcileBuild ensures the Shipwright Build exists and matches desired state.
// It returns errors; the main Reconcile flow is responsible for updating CappBuild status.
func (r *CappBuildReconciler) reconcileBuild(
	ctx context.Context,
	cb *rcs.CappBuild,
	selectedStrategyName string,
) error {
	logger := log.FromContext(ctx)

	desired, err := r.newBuild(cb, selectedStrategyName)
	if err != nil {
		return fmt.Errorf("failed to generate build definition: %w", err)
	}

	actual := &shipwright.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      desired.Name,
			Namespace: desired.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, actual, func() error {
		if err := controllerutil.SetControllerReference(cb, actual, r.Scheme); err != nil {
			return err
		}
		// Merge controller-owned labels; do not wipe labels from other actors.
		if actual.Labels == nil {
			actual.Labels = map[string]string{}
		}
		for k, v := range desired.Labels {
			actual.Labels[k] = v
		}
		actual.Spec = desired.Spec
		return nil
	})
	if err != nil {
		return err
	}
	if op != controllerutil.OperationResultNone {
		logger.Info("Reconciled Shipwright Build", "name", actual.Name, "operation", string(op))
	}
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

### 3.2) Wire: policy → probe → select strategy → reconcileBuild

Replace the body after `ObservedGeneration` handling with:

```go
var alreadyOwned *controllerutil.AlreadyOwnedError

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

if err := r.reconcileBuild(ctx, cb, selected); err != nil {
	if errors.As(err, &alreadyOwned) {
		_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonBuildConflict, err.Error())
		return ctrl.Result{}, nil
	}
	_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonBuildReconcileFailed, err.Error())
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

return ctrl.Result{}, nil
```

Also add imports (adjust existing list):

```go
import (
	"errors"
	"time"

	capputils "github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
)

func probeBuildFilePresence(ctx context.Context, cb *rcs.CappBuild) (bool, error) {
	return false, fmt.Errorf("probeBuildFilePresence not implemented")
}
```

## 5) Generate manifests + format

```bash
make fmt
make manifests
```



