package resourcemanagers

import (
	"context"
	"errors"
	"fmt"

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

// ManageError associates a resource manager with the failure it returned.
type ManageError struct {
	Name string
	Err  error
}

func (e ManageError) Error() string { return fmt.Sprintf("%s: %v", e.Name, e.Err) }
func (e ManageError) Unwrap() error { return e.Err }

// JoinManageErrors aggregates manager failures into a single error.
func JoinManageErrors(manageErrors []ManageError) error {
	if len(manageErrors) == 0 {
		return nil
	}
	errs := make([]error, 0, len(manageErrors))
	for _, e := range manageErrors {
		errs = append(errs, e)
	}
	return errors.Join(errs...)
}
