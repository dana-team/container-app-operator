# Phase 3 Implementation — `CappBuild` Controller Scaffold (kubebuilder)

This is a copy/paste playbook to implement Phase 3.

## Commands to run

From repo root (`/home/sbahar/projects/ps/dana-team/container-app-operator`):

```bash
# scaffold controller (API already exists)
kubebuilder create api --group rcs --version v1alpha1 --kind CappBuild --resource=false --controller=true
```

Notes:
- The kubebuilder command may create a default controller under `internal/controller/` and may propose changes in `cmd/main.go`.
- This repo keeps reconcilers under `internal/kinds/.../controllers/`; use the paths below as the source of truth.

## 1) Add controller code

Create file:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/internal/kinds/cappbuild/controllers/controller.go`

Paste:

```go
package controllers

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
)

const cappBuildControllerName = "CappBuildController"

// CappBuildReconciler reconciles a CappBuild object.
type CappBuildReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	EventRecorder record.EventRecorder
}

// +kubebuilder:rbac:groups=rcs.dana.io,resources=cappbuilds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rcs.dana.io,resources=cappbuilds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rcs.dana.io,resources=cappbuilds/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;patch;update
// +kubebuilder:rbac:groups="events.k8s.io",resources=events,verbs=get;list;watch;create;patch;update

func (r *CappBuildReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cappv1alpha1.CappBuild{}).
		Named(cappBuildControllerName).
		Complete(r)
}

func (r *CappBuildReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("CappBuildName", req.Name, "CappBuildNamespace", req.Namespace)
	logger.Info("Starting Reconcile")

	cb := &cappv1alpha1.CappBuild{}
	if err := r.Get(ctx, req.NamespacedName, cb); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get CappBuild: %w", err)
	}

	// Scaffold-only: demonstrate status write path for later phases.
	if cb.Status.ObservedGeneration != cb.Generation {
		cb.Status.ObservedGeneration = cb.Generation
		if err := r.Status().Update(ctx, cb); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update CappBuild status: %w", err)
		}
	}

	return ctrl.Result{}, nil
}
```

## 2) Wire the controller into the manager (feature-gated)

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/cmd/main.go`

### 2.1 Add import

Add:

```go
cappbuildcontroller "github.com/dana-team/container-app-operator/internal/kinds/cappbuild/controllers"
```

### 2.2 Add SetupWithManager block

Below the existing controller registrations (near the `Capp` and `CappRevision` setups), add:

```go
if os.Getenv("ENABLE_CAPPBUILD_CONTROLLER") == "true" {
	if err = (&cappbuildcontroller.CappBuildReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("cappbuild-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CappBuild")
		os.Exit(1)
	}
} else {
	setupLog.Info("cappbuild controller disabled (set ENABLE_CAPPBUILD_CONTROLLER=true to enable)")
}
```

## 3) Helm: feature flag value + env var wiring

### 3.1 Add Helm value

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/charts/container-app-operator/values.yaml`

Add:

```yaml
cappBuild:
  enabled: false
```

### 3.2 Pass the env var to the manager Deployment

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/charts/container-app-operator/templates/deployment.yaml`

Under `env:` add:

```yaml
        - name: ENABLE_CAPPBUILD_CONTROLLER
          value: {{ ternary "true" "false" .Values.cappBuild.enabled | quote }}
```

## 4) Helm RBAC: allow the manager to watch/update CappBuild

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/charts/container-app-operator/templates/manager-rbac.yaml`

Add these rules (near the existing `rcs.dana.io` rules):

```yaml
- apiGroups:
  - rcs.dana.io
  resources:
  - cappbuilds
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - rcs.dana.io
  resources:
  - cappbuilds/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - rcs.dana.io
  resources:
  - cappbuilds/finalizers
  verbs:
  - update
```

## 5) Regenerate generated artifacts

Run:

```bash
make manifests
make generate
make fmt
```

This updates generated files under:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/config/` (not the Helm chart templates)


