# Adding a New Event Source Type

This guide explains how to implement and register a new Knative Eventing source type in the Capp operator.

## Overview

Event sources are pluggable. Each source type implements `EventSourceKind` and registers itself via `sources.Register`. The `EventSourceManager` then dispatches to the correct implementation at runtime without knowing the concrete types.

## Steps

### 1. Add the spec type to the API

Add a new spec struct as a pointer field in `SourceConfiguration` in [api/v1alpha1/capp_types.go](../api/v1alpha1/capp_types.go).

```go
// ApiServerSourceConfiguration defines the desired state for a Knative ApiServerSource.
type ApiServerSourceConfiguration struct {
    // ... fields specific to this source type
}

type SourceConfiguration struct {
    Name                        string                        `json:"name"`
    URI                         *kapis.URL                    `json:"uri,omitempty"`
    ApiServerSourceConfiguration *ApiServerSourceConfiguration `json:"apiServerSourceConfiguration,omitempty"` // new field
}
```

Then run `make generate` to regenerate deepcopy functions.

### 2. Implement `EventSourceKind`

Create a new file in [internal/kinds/capp/resourcemanagers/eventsources/](../internal/kinds/capp/resourcemanagers/eventsources/) and implement the interface from [internal/kinds/capp/sources/registry.go](../internal/kinds/capp/sources/registry.go):

```go
package eventsources

import (
    "context"

    cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
    rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
    "github.com/dana-team/container-app-operator/internal/kinds/capp/sources"
    "github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
    "github.com/go-logr/logr"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const apiServerSourceKindName = "ApiServerSource"

type ApiServerSourceKind struct {
    sources.EventSourceKind
}

func init() {
    sources.Register(apiServerSourceKindName, &ApiServerSourceKind{})
}

func (k *ApiServerSourceKind) Generate(capp cappv1alpha1.Capp, source cappv1alpha1.SourceConfiguration) client.Object {
    // Build and return the desired Knative ApiServerSource.
    // Always set sink to the Capp's Knative Service using sources.KnativeServiceKind.
    // Set labels via utils.ManagedResourceLabels(capp.Name).
    // Guard nil pointer: source.ApiServerSourceConfiguration may be nil.
    return nil
}

func (k *ApiServerSourceKind) CreateOrUpdate(ctx context.Context, rm rclient.ResourceManagerClient, log logr.Logger, capp cappv1alpha1.Capp, source cappv1alpha1.SourceConfiguration) error {
    desired := k.Generate(capp, source)
    controllerutil.SetOwnerReference(&capp, desired, rm.K8sclient.Scheme())
    // Get existing; if not found create. If found and spec differs, copy spec onto
    // existing (preserving ResourceVersion) then update.
    return nil
}

func (k *ApiServerSourceKind) List(ctx context.Context, rm rclient.ResourceManagerClient, log logr.Logger, capp cappv1alpha1.Capp) ([]client.Object, error) {
    // List all ApiServerSources matching utils.ManagedResourceLabels(capp.Name)
    // in capp.Namespace. Convert []ApiServerSource to []client.Object before returning.
    return nil, nil
}

func (k *ApiServerSourceKind) Delete(ctx context.Context, rm rclient.ResourceManagerClient, log logr.Logger, capp cappv1alpha1.Capp, source cappv1alpha1.SourceConfiguration) error {
    // Get by name; if not found return nil. Otherwise delete.
    return nil
}

func (k *ApiServerSourceKind) GetStatus(ctx context.Context, rm rclient.ResourceManagerClient, log logr.Logger, capp cappv1alpha1.Capp) ([]cappv1alpha1.EventSourceStatus, error) {
    // Call List, then for each owned resource inspect its readiness condition
    // and return []EventSourceStatus.
    return nil, nil
}
```

### 3. Wire the dispatch in `GetEventSourceKind`

Add a case to the switch in [internal/kinds/capp/sources/registry.go](../internal/kinds/capp/sources/registry.go) so the kind is selected when the source has your new field set:

```go
func GetEventSourceKind(source cappv1alpha1.SourceConfiguration) (EventSourceKind, bool) {
    sourceType := ""
    switch {
    case source.PingSourceConfiguration != nil:
        sourceType = sources.PingSourceKindName
    case source.ApiServerSourceConfiguration != nil:
        sourceType = apiServerSourceKindName
    }
    ...
}
```

> **Note:** `apiServerSourceKindName` is a private constant defined in your new file. The `init()` registers it, and the switch here selects it by the same string value. Keep them in sync. Use `sources.PingSourceKindName` (the shared public constant) for PingSource; define a similar constant in `sources` for each new kind you add.

### 4. Import the package in `cmd/main.go`

The `init()` registration runs only if the package is imported. Add a blank import:

```go
import (
    _ "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers/eventsources"
)
```

### 5. Add a kubebuilder RBAC annotation

Add a `+kubebuilder:rbac` marker in [internal/kinds/capp/controllers/controller.go](../internal/kinds/capp/controllers/controller.go) so `make manifests` generates the correct ClusterRole rules for your source type.

```go
// +kubebuilder:rbac:groups=sources.knative.dev,resources=apiserversources,verbs=get;list;watch;create;update;delete
```

Place it alongside the other RBAC annotations above `SetupWithManager`. The `groups` value matches the Knative Eventing API group (`sources.knative.dev`); `resources` is the lowercase plural of the CRD kind.

### 6. Add a Watch and predicate to `SetupWithManager`

The controller must watch your source type so that readiness transitions re-trigger Capp reconciliation. Add the following generic helper and a `Watches` block to [internal/kinds/capp/controllers/controller.go](../internal/kinds/capp/controllers/controller.go).

Add the helper alongside the other predicate functions:

```go
// eventSourceWatchPredicate triggers on spec changes (generation bump) or readiness transitions.
// isReady extracts the ready state from the concrete source type T.
func eventSourceWatchPredicate[T client.Object](isReady func(T) bool) predicate.Predicate {
    return predicate.Or(
        predicate.GenerationChangedPredicate{},
        predicate.TypedFuncs[client.Object]{
            UpdateFunc: func(e event.TypedUpdateEvent[client.Object]) bool {
                oldObj, okOld := e.ObjectOld.(T)
                newObj, okNew := e.ObjectNew.(T)
                if !okOld || !okNew {
                    return false
                }
                return isReady(oldObj) != isReady(newObj)
            },
        },
    )
}
```

Then add a `Watches` block inside `SetupWithManager`, chained before `Complete(r)`:

```go
Watches(
    &sourcesv1.ApiServerSource{},
    handler.EnqueueRequestsFromMapFunc(r.findCappFromEvent),
    builder.WithPredicates(eventSourceWatchPredicate[*sourcesv1.ApiServerSource](
        func(o *sourcesv1.ApiServerSource) bool { return o.Status.IsReady() },
    )),
).
```

The predicate fires on:
- **Spec changes** — `GenerationChangedPredicate` catches any spec update.
- **Readiness transitions** — the `UpdateFunc` fires when `IsReady()` flips, so Capp status reflects source readiness without reconciling on every status heartbeat.

Use `findCappFromEvent` (maps by `namespace/name`) for sources whose name equals `<capp-name>-<source-name>`. If your naming scheme differs, implement a dedicated mapper similar to `findCappFromLabels`.

### 7. Write unit tests

Create a `_test.go` file next to your implementation. Test each method against a fake k8s client (`sigs.k8s.io/controller-runtime/pkg/client/fake`). Follow the pattern in [internal/kinds/capp/resourcemanagers/eventsources/pingsource_test.go](../internal/kinds/capp/resourcemanagers/eventsources/pingsource_test.go).

Key cases to cover:

| Method | Cases |
|--------|-------|
| `Generate` | correct name (`<capp>-<source>`), labels, sink ref, spec fields populated from config |
| `CreateOrUpdate` | resource absent → creates; resource present with different spec → updates; same spec → no-op |
| `List` | returns only resources with `utils.ManagedResourceLabels(capp.Name)` label; excludes unrelated resources |
| `Delete` | removes resource; not-found is not an error |
| `GetStatus` | no readiness condition set → `ConditionUnknown`; ready source → `ConditionTrue` with message |

### 8. Update the CRD

Run `make manifests` then copy the updated CRD to `charts/container-app-operator/crds/` as described in the [Helm CRD sync guide](../charts/container-app-operator/README.md).

## Interface contract

| Method | Responsibility |
|--------|---------------|
| `Generate` | Build the desired source object from `capp` and `source`. Set `Labels: utils.ManagedResourceLabels(capp.Name)`. Guard nil: the source-specific config field (e.g. `source.PingSourceConfiguration`) is a pointer and may be nil. |
| `CreateOrUpdate` | Idempotent apply of a single source entry. Call `controllerutil.SetOwnerReference(&capp, obj, rm.K8sclient.Scheme())` before create/update. On update: copy the desired spec onto the fetched existing object (preserving `ResourceVersion`) before calling `Update`. |
| `List` | Return all source resources of this type with `utils.ManagedResourceLabels(capp.Name)` in `capp.Namespace`. Used for orphan detection and status aggregation. Convert the typed slice to `[]client.Object` before returning. |
| `Delete` | Remove a single source resource by name. Not-found must be treated as success. |
| `GetStatus` | Return `[]EventSourceStatus` for all owned resources. Called by `EventSourceManager.GetStatus` to build `CappStatus.EventingStatus`. |

## Naming convention

Resource names follow the pattern `<capp-name>-<source-name>`. `SourceConfiguration.Name` is required and must be unique within the Capp. Use `utils.ManagedResourceLabels(capp.Name)` for resource labels and `fmt.Sprintf("%s-%s", capp.Name, source.Name)` for resource names.
