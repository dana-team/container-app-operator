package sources

import (
	"context"
	"sync"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	PingSourceKindName = "PingSource"
)

// EventSourceKind defines the interface for managing a specific Knative Eventing source type.
// Each source type (e.g., PingSource, ApiServerSource) should implement this interface.
type EventSourceKind interface {
	Generate(capp cappv1alpha1.Capp, source cappv1alpha1.SourceConfiguration) client.Object
	CreateOrUpdate(ctx context.Context, rm rclient.ResourceManagerClient, log logr.Logger, capp cappv1alpha1.Capp, source cappv1alpha1.SourceConfiguration) error
	List(ctx context.Context, rm rclient.ResourceManagerClient, log logr.Logger, capp cappv1alpha1.Capp) ([]client.Object, error)
	Delete(ctx context.Context, rm rclient.ResourceManagerClient, log logr.Logger, capp cappv1alpha1.Capp, source cappv1alpha1.SourceConfiguration) error
	GetStatus(ctx context.Context, rm rclient.ResourceManagerClient, log logr.Logger, capp cappv1alpha1.Capp) ([]cappv1alpha1.EventSourceStatus, error)
	Validate(capp cappv1alpha1.Capp, source cappv1alpha1.SourceConfiguration) error
}

var (
	registryMu              sync.RWMutex
	eventSourceKindRegistry = make(map[string]EventSourceKind)
)

// Register adds an EventSourceKind implementation under the given source type key.
// Pass nil to remove a previously registered kind.
func Register(sourceType string, kind EventSourceKind) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if kind == nil {
		delete(eventSourceKindRegistry, sourceType)
		return
	}
	eventSourceKindRegistry[sourceType] = kind
}

// AllKinds returns all registered EventSourceKind implementations.
func AllKinds() []EventSourceKind {
	registryMu.RLock()
	defer registryMu.RUnlock()
	result := make([]EventSourceKind, 0, len(eventSourceKindRegistry))
	for _, k := range eventSourceKindRegistry {
		result = append(result, k)
	}
	return result
}

// GetEventSourceKind returns the registered kind for the source type encoded in source.
func GetEventSourceKind(source cappv1alpha1.SourceConfiguration) (EventSourceKind, bool) {
	sourceType := ""
	switch {
	case source.PingSourceConfiguration != nil:
		sourceType = PingSourceKindName
	default:
		sourceType = ""
	}
	if sourceType == "" {
		return nil, false
	}
	registryMu.RLock()
	defer registryMu.RUnlock()
	kind, exists := eventSourceKindRegistry[sourceType]
	return kind, exists
}
