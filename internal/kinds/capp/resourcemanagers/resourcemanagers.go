package resourcemanagers

import (
	"context"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
)

// ResourceManager is an interface for every resource managed by Capp.
type ResourceManager interface {
	Manage(ctx context.Context, capp cappv1alpha1.Capp) error
	CleanUp(ctx context.Context, capp cappv1alpha1.Capp) error
	IsRequired(capp cappv1alpha1.Capp) bool
}

// ResourceManagerEntry pairs a name with its ResourceManager so that
// a slice can enforce deterministic execution order.
type ResourceManagerEntry struct {
	Name    string
	Manager ResourceManager
}

// ManagerMap converts an ordered slice of entries into a map keyed by name,
// for call sites that need keyed lookup (e.g. status).
func ManagerMap(entries []ResourceManagerEntry) map[string]ResourceManager {
	m := make(map[string]ResourceManager, len(entries))
	for _, e := range entries {
		m[e.Name] = e.Manager
	}
	return m
}
