# Phase 3 Implementation — `CappBuild` Controller Scaffold (manual)

This is a copy/paste playbook to implement Phase 3.

## Notes (why manual)

`CappBuild` API types/CRDs already exist in this repo, so re-running kubebuilder scaffolding fails with:
`API resource already exists`.

This repo keeps reconcilers under `internal/kinds/.../controllers/`; use the paths below as the source of truth.

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
		// Best practice: advance ObservedGeneration only after reconciling this Generation.
		orig := cb.DeepCopy()
		cb.Status.ObservedGeneration = cb.Generation
		if err := r.Status().Patch(ctx, cb, client.MergeFrom(orig)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch CappBuild status: %w", err)
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

### 2.3 Local run (from host)

When running locally via `make run`, the webhook server may fail due to missing serving certs.
For local development, disable webhooks and enable the CappBuild controller:

```bash
ENABLE_WEBHOOKS=false ENABLE_CAPPBUILD_CONTROLLER=true make run
```

## 3) Helm: feature flag value + env var wiring

### 3.1 Add Helm value

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/charts/container-app-operator/values.yaml`

Add:

```yaml
cappBuild:
  # -- Feature flag: when true, the manager process will start the CappBuild controller.
  enabled: false
```

### 3.2 Pass the env var to the manager Deployment

Edit:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/charts/container-app-operator/templates/deployment.yaml`

Under `env:` add:

```yaml
        - name: ENABLE_CAPPBUILD_CONTROLLER
          # Use a string ("true"/"false") because env vars are strings; main.go checks for "true".
          value: {{ ternary "true" "false" .Values.cappBuild.enabled | quote }}
```

## 4) Helm RBAC: allow the manager to watch/update CappBuild

Best practice: treat generated RBAC as the source of truth, then port it to Helm.

1) Generate RBAC from `+kubebuilder:rbac` markers:

```bash
make manifests
```

2) Inspect the generated manager ClusterRole:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/config/rbac/role.yaml`

3) Port the needed `cappbuilds` rules into the Helm template:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/charts/container-app-operator/templates/manager-rbac.yaml`

Reference (these are the CappBuild-related rules you should see reflected in the chart):

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

## 6) E2E: skeleton test for the CappBuild controller

This is a minimal E2E that validates the controller is running and can write `status.observedGeneration`.

Prereqs:
- Deploy the operator with the controller enabled (`ENABLE_CAPPBUILD_CONTROLLER=true` / Helm `cappBuild.enabled=true`).

Create file:
- `/home/sbahar/projects/ps/dana-team/container-app-operator/test/e2e_tests/cappbuild_e2e_test.go`

Paste:

```go
package e2e_tests

import (
	"context"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Validate CappBuild controller", func() {
	It("Should update status.observedGeneration", func() {
		name := utilst.RandomName("cappbuild")
		cb := &cappv1alpha1.CappBuild{
			TypeMeta: metav1.TypeMeta{
				Kind:       "CappBuild",
				APIVersion: "rcs.dana.io/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testconsts.NSName,
			},
			Spec: cappv1alpha1.CappBuildSpec{
				Source: cappv1alpha1.CappBuildSource{
					Type: cappv1alpha1.CappBuildSourceTypeGit,
					Git: cappv1alpha1.CappBuildGitSource{
						URL: "https://github.com/dana-team/container-app-operator",
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), cb)).To(Succeed())

		Eventually(func() bool {
			latest := &cappv1alpha1.CappBuild{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: name, Namespace: testconsts.NSName}, latest)).To(Succeed())
			return latest.Generation != 0 && latest.Status.ObservedGeneration == latest.Generation
		}, testconsts.Timeout, testconsts.Interval).Should(BeTrue())
	})
})
```

Run (example):

```bash
make test-e2e E2E_GINKGO_FOCUS="CappBuild controller"
```


