# Implementation — Phase 7 BuildRun creation + status updates

This phase implements Shipwright `BuildRun` creation using the already-reconciled Shipwright `Build`, and updates `CappBuild.status` accordingly.

## 1) Add condition/reason constants

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/conditions.go`

Add the following constants (keep existing ones):

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/conditions.go`

```go
const (
	TypeBuildSucceeded = "BuildSucceeded"

	ReasonBuildRunReconcileFailed = "BuildRunReconcileFailed"
	ReasonBuildRunConflict        = "BuildRunConflict"

	ReasonBuildRunPending   = "BuildRunPending"
	ReasonBuildRunRunning   = "BuildRunRunning"
	ReasonBuildRunSucceeded = "BuildRunSucceeded"
	ReasonBuildRunFailed    = "BuildRunFailed"
)
```

## 2) Add BuildRun helpers

Create:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/buildrun.go`

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/buildrun.go`

```go
package controllers

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	distref "github.com/distribution/reference"
	shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

func buildRunNameFor(cb *rcs.CappBuild) string {
	return fmt.Sprintf("%s-buildrun-%d", cb.Name, cb.Generation)
}

func newBuildRun(cb *rcs.CappBuild) *shipwright.BuildRun {
	buildName := buildNameFor(cb)

	return &shipwright.BuildRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildRunNameFor(cb),
			Namespace: cb.Namespace,
			Labels: map[string]string{
				rcs.GroupVersion.Group + "/parent-cappbuild": cb.Name,
			},
		},
		Spec: shipwright.BuildRunSpec{
			Build: shipwright.ReferencedBuild{
				Name: &buildName,
			},
		},
	}
}

// reconcileBuildRun ensures the expected BuildRun exists for this CappBuild generation.
// This phase creates the BuildRun once per generation and does not patch it afterwards.
// It returns a controllerutil.AlreadyOwnedError if a conflicting controller owner exists.
func (r *CappBuildReconciler) reconcileBuildRun(
	ctx context.Context,
	cb *rcs.CappBuild,
) (*shipwright.BuildRun, error) {
	desired := newBuildRun(cb)

	existing := &shipwright.BuildRun{}
	key := client.ObjectKeyFromObject(desired)
	if err := r.Get(ctx, key, existing); err == nil {
		if !metav1.IsControlledBy(existing, cb) {
			return nil, &controllerutil.AlreadyOwnedError{Object: existing}
		}
		return existing, nil
	} else if client.IgnoreNotFound(err) != nil {
		return nil, err
	}

	if err := controllerutil.SetControllerReference(cb, desired, r.Scheme); err != nil {
		return nil, err
	}
	if err := r.Create(ctx, desired); err != nil {
		return nil, err
	}
	return desired, nil
}

func deriveBuildSucceededStatus(br *shipwright.BuildRun) (metav1.ConditionStatus, string, string) {
	succeededCondition := br.Status.GetCondition(shipwright.Succeeded)
	if succeededCondition == nil {
		return metav1.ConditionUnknown, ReasonBuildRunPending, "BuildRun has not reported status yet"
	}

	switch succeededCondition.GetStatus() {
	case corev1.ConditionTrue:
		return metav1.ConditionTrue, ReasonBuildRunSucceeded, "BuildRun succeeded"
	case corev1.ConditionFalse:
		// Keep message low-cardinality; include upstream reason/message only as best-effort debugging.
		msg := "BuildRun failed"
		if succeededCondition.GetReason() != "" || succeededCondition.GetMessage() != "" {
			msg = fmt.Sprintf("BuildRun failed: %s %s", succeededCondition.GetReason(), succeededCondition.GetMessage())
		}
		return metav1.ConditionFalse, ReasonBuildRunFailed, strings.TrimSpace(msg)
	default:
		// Unknown or empty => running/starting.
		msg := "BuildRun is running"
		if succeededCondition.GetReason() != "" || succeededCondition.GetMessage() != "" {
			msg = fmt.Sprintf("BuildRun is running: %s %s", succeededCondition.GetReason(), succeededCondition.GetMessage())
		}
		return metav1.ConditionUnknown, ReasonBuildRunRunning, strings.TrimSpace(msg)
	}
}

func computeLatestImage(cb *rcs.CappBuild, br *shipwright.BuildRun) string {
	if br.Status.Output != nil && br.Status.Output.Digest != "" {
		return cb.Spec.Output.Image + "@" + br.Status.Output.Digest
	}
	if hasTagOrDigest(cb.Spec.Output.Image) {
		return cb.Spec.Output.Image
	}
	return ""
}

func hasTagOrDigest(image string) bool {
	parsed, err := distref.ParseNormalizedNamed(image)
	if err != nil {
		return false
	}
	if _, ok := parsed.(distref.Digested); ok {
		return true
	}
	// Name-only means no explicit tag and no digest.
	return !distref.IsNameOnly(parsed)
}

func (r *CappBuildReconciler) patchBuildSucceededCondition(
	ctx context.Context,
	cb *rcs.CappBuild,
	br *shipwright.BuildRun,
) error {
	orig := cb.DeepCopy()

	cb.Status.ObservedGeneration = cb.Generation
	cb.Status.LastBuildRunRef = cb.Namespace + "/" + br.Name

	status, reason, message := deriveBuildSucceededStatus(br)
	meta.SetStatusCondition(&cb.Status.Conditions, metav1.Condition{
		Type:               TypeBuildSucceeded,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: cb.Generation,
	})

	return r.Status().Patch(ctx, cb, client.MergeFrom(orig))
}

func (r *CappBuildReconciler) patchLatestImage(
	ctx context.Context,
	cb *rcs.CappBuild,
	latestImage string,
) error {
	if latestImage == "" {
		return nil
	}

	orig := cb.DeepCopy()
	cb.Status.ObservedGeneration = cb.Generation
	cb.Status.LatestImage = latestImage
	return r.Status().Patch(ctx, cb, client.MergeFrom(orig))
}
```

## 3) Update controller wiring (watch + RBAC + reconcile flow)

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/controller.go`

### 3.1) RBAC for Shipwright `BuildRun`

Add:

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/controller.go`

```go
// +kubebuilder:rbac:groups=shipwright.io,resources=buildruns,verbs=get;list;watch;create
```

### 3.2) Watch BuildRuns owned by CappBuild

Update `SetupWithManager`:

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/controller.go`

```go
return ctrl.NewControllerManagedBy(mgr).
	For(&rcs.CappBuild{}).
	Owns(&shipwright.Build{}).
	Owns(&shipwright.BuildRun{}).
	Named(cappBuildControllerName).
	Complete(r)
```

### 3.3) Reconcile BuildRun and update status from BuildRun

After `reconcileBuild(...)` succeeds (and after `status.buildRef` is ensured), add:

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/controller.go`

```go
buildRun, err := r.reconcileBuildRun(ctx, cb)
if err != nil {
	if errors.As(err, &alreadyOwned) {
		_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonBuildRunConflict, err.Error())
		return ctrl.Result{}, nil
	}
	_ = r.patchReadyCondition(ctx, cb, metav1.ConditionFalse, ReasonBuildRunReconcileFailed, err.Error())
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

if err := r.patchBuildSucceededCondition(ctx, cb, buildRun); err != nil {
	return ctrl.Result{}, err
}

if buildRun.IsSuccessful() {
	if err := r.patchLatestImage(ctx, cb, computeLatestImage(cb, buildRun)); err != nil {
		return ctrl.Result{}, err
	}
}

// If build is still running, requeue to refresh status even if watches are delayed.
cond := meta.FindStatusCondition(cb.Status.Conditions, TypeBuildSucceeded)
if cond != nil && cond.Status == metav1.ConditionUnknown {
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}
```

Note: ensure imports include `meta` and `metav1` (already used in current controller), and keep stdlib `errors` for `errors.As`.

## 4) Generate manifests + format

Copy to: `/home/sbahar/projects/ps/dana-team/container-app-operator` (run in a shell)

```bash
cd /home/sbahar/projects/ps/dana-team/container-app-operator
make fmt
make manifests
```


