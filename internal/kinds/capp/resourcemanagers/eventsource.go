package resourcemanagers

import (
	"context"
	"fmt"
	"sort"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/sources"
	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	EventSources = "eventSources"
)

// EventSourceManager manages all Knative Eventing sources for a Capp.
// To add a new source type: implement EventSourceKind and register it via sources.Register.
type EventSourceManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder record.EventRecorder
}

// IsRequired returns true when at least one event source is declared in the spec.
func (e EventSourceManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return len(capp.Spec.EventSourcesSpec.Sources) > 0
}

// Manage creates/updates sources when required, or cleans all up when not.
func (e EventSourceManager) Manage(capp cappv1alpha1.Capp) error {
	if e.IsRequired(capp) {
		return e.createOrUpdateSources(capp)
	}
	return e.CleanUp(capp)
}

// CleanUp deletes all source resources owned by the Capp, across all registered source types.
// It lists owned resources per manager so it handles the case where all sources were removed from spec.
func (e EventSourceManager) CleanUp(capp cappv1alpha1.Capp) error {
	rm := rclient.ResourceManagerClient{Ctx: e.Ctx, K8sclient: e.K8sclient, Log: e.Log}
	for _, kind := range sources.AllKinds() {
		owned, err := kind.List(e.Ctx, rm, e.Log, capp)
		if err != nil {
			return fmt.Errorf("failed to list owned event sources for cleanup: %w", err)
		}
		for _, obj := range owned {
			if err := e.K8sclient.Delete(e.Ctx, obj); client.IgnoreNotFound(err) != nil {
				return fmt.Errorf("failed to delete event source %q: %w", obj.GetName(), err)
			}
		}
	}
	return nil
}

// GetStatus returns the aggregated status of all event source resources owned by the Capp.
// Results are sorted by Name for a stable output across reconcile loops.
func (e EventSourceManager) GetStatus(capp cappv1alpha1.Capp) (cappv1alpha1.EventingStatus, error) {
	rm := rclient.ResourceManagerClient{Ctx: e.Ctx, K8sclient: e.K8sclient, Log: e.Log}
	var statuses []cappv1alpha1.EventSourceStatus
	for _, kind := range sources.AllKinds() {
		s, err := kind.GetStatus(e.Ctx, rm, e.Log, capp)
		if err != nil {
			return cappv1alpha1.EventingStatus{}, fmt.Errorf("failed to get event source status: %w", err)
		}
		statuses = append(statuses, s...)
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Name < statuses[j].Name })
	return cappv1alpha1.EventingStatus{EventSources: statuses}, nil
}

// createOrUpdateSources applies CreateOrUpdate for each source declared in the spec.
func (e EventSourceManager) createOrUpdateSources(capp cappv1alpha1.Capp) error {
	rm := rclient.ResourceManagerClient{Ctx: e.Ctx, K8sclient: e.K8sclient, Log: e.Log}
	for _, source := range capp.Spec.EventSourcesSpec.Sources {
		kind, exists := sources.GetEventSourceKind(source)
		if !exists {
			e.Log.Info("No kind registered for source, skipping", "sourceName", source.Name)
			continue
		}
		err := kind.CreateOrUpdate(e.Ctx, rm, e.Log, capp, source)
		if err != nil {
			return fmt.Errorf("failed to create or update %q source: %w", source.Name, err)
		}
	}

	return nil
}
