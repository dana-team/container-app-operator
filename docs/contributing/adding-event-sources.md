# Adding a New Event Source Type

This guide explains how to implement and register a new Knative Eventing source type in the Capp operator.

## Overview

Event sources are pluggable. Each source type implements `EventSourceKind` and registers itself via `sources.Register`. The `EventSourceManager` then dispatches to the correct implementation at runtime without knowing the concrete types.

## Steps

### 1. Add the spec type to the API

Add a new spec struct and a field for it in `EventSource` in [api/v1alpha1/capp_types.go](../api/v1alpha1/capp_types.go).

```go
// ApiServerSourceSpec defines the desired state for a Knative ApiServerSource.
type ApiServerSourceSpec struct {
    // ... fields specific to this source type
}

type EventSource struct {
    Name            string               `json:"name"`                      // required, minLength=1
    ApiServerSource *ApiServerSourceSpec `json:"apiServerSource,omitempty"` // new field
}
```

Then run `make generate` to regenerate deepcopy functions.

### 2. Implement `EventSourceKind`

Create a new file in [internal/kinds/capp/resourcemanagers/eventsources/](../internal/kinds/capp/resourcemanagers/eventsources/) (create the directory if needed) and implement the interface from [internal/kinds/capp/sources/sources.go](../internal/kinds/capp/sources/sources.go):

```go
package eventsources

import (
    "context"

    cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
    rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
    "github.com/dana-team/container-app-operator/internal/kinds/capp/sources"
    "github.com/go-logr/logr"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

const ApiServerSourceType = "apiserversource"

type ApiServerSourceManager struct{}

func init() {
    sources.Register(ApiServerSourceType, ApiServerSourceManager{})
}

func (m ApiServerSourceManager) CreateOrUpdate(ctx context.Context, rm rclient.ResourceManagerClient, log logr.Logger, capp cappv1alpha1.Capp, source cappv1alpha1.EventSource) error {
    // Build the desired Knative ApiServerSource from capp + source.
    // Use rm.CreateResource / rm.UpdateResource.
    // Always set the sink to the Capp's Knative Service.
    return nil
}

func (m ApiServerSourceManager) List(ctx context.Context, rm rclient.ResourceManagerClient, log logr.Logger, capp cappv1alpha1.Capp) ([]client.Object, error) {
    // List all ApiServerSources with label utils.CappResourceKey == capp.Name
    // in capp.Namespace.
    return nil, nil
}

func (m ApiServerSourceManager) Delete(ctx context.Context, rm rclient.ResourceManagerClient, log logr.Logger, capp cappv1alpha1.Capp, source cappv1alpha1.EventSource) error {
    // Delete the ApiServerSource resource for this source entry.
    return nil
}

func (m ApiServerSourceManager) GetStatus(ctx context.Context, rm rclient.ResourceManagerClient, log logr.Logger, capp cappv1alpha1.Capp) ([]cappv1alpha1.EventSourceStatus, error) {
    // List owned ApiServerSources, check readiness, return []EventSourceStatus.
    return nil, nil
}
```

### 3. Wire the dispatch in `GetEventSourceKindManager`

Add a case to the switch in [internal/kinds/capp/sources/sources.go](../internal/kinds/capp/sources/sources.go) so the manager is selected when the source has your new field set:

```go
func GetEventSourceKindManager(source cappv1alpha1.EventSource) (EventSourceKind, bool) {
    sourceType := ""
    switch {
    case source.ApiServerSource != nil:
        sourceType = eventsources.ApiServerSourceType
    // existing cases...
    }
    ...
}
```

> **Note:** Import the eventsources package here (or use a string constant) — the `init()` in your new file registers the manager, so a blank import of the package is sufficient if the constant is defined there.

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

Use `findCappFromEvent` (maps by `namespace/name`) for sources whose name equals `<capp-name>-<source-name>`. If your naming scheme differs, implement a dedicated mapper similar to `findCappFromHostname`.

### 7. Write unit tests

Create a `_test.go` file next to your implementation. Test each method against a fake k8s client (`sigs.k8s.io/controller-runtime/pkg/client/fake`). Follow the pattern in [internal/kinds/capp/resourcemanagers/eventsource_test.go](../internal/kinds/capp/resourcemanagers/eventsource_test.go) for using the mock to verify `EventSourceManager` dispatches correctly.

Key cases to cover:

| Method | Cases |
|--------|-------|
| `CreateOrUpdate` | resource absent → creates; resource present, spec drifted → updates; no drift → no-op |
| `List` | returns only resources with matching `CappResourceKey` label |
| `Delete` | removes resource; not-found is not an error |
| `GetStatus` | ready source → `Ready: true`; not-ready → `Ready: false` + `Message` populated |

### 8. Update the CRD

Run `make manifests` then copy the updated CRD to `charts/container-app-operator/crds/` as described in the [Helm CRD sync guide](../charts/container-app-operator/README.md).

## Interface contract

| Method | Responsibility |
|--------|---------------|
| `CreateOrUpdate` | Idempotent apply of a single source entry from the Capp spec. Sink must always point to the Capp's Knative Service. Must call `controllerutil.SetOwnerReference(&capp, obj, rm.K8sclient.Scheme())` on every object before creating or updating it so Kubernetes GC can clean up owned resources. |
| `List` | Return all source resources of this type owned by the Capp (use `CappResourceKey` label selector). Used for orphan detection. |
| `Delete` | Remove a single source resource. Not-found should be treated as success. |
| `GetStatus` | Return `[]EventSourceStatus` for all owned resources. Called by `EventSourceManager.GetStatus` to build `CappStatus.EventingStatus`. |

## Naming convention

Resource names follow the pattern `<capp-name>-<source-name>`. `EventSource.Name` is required. Use `utils.ManagedResourceLabels(capp.Name)` for the resource labels.
