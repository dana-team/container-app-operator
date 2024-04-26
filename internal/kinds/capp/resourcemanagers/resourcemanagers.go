package resourcemanagers

import cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"

// ResourceManager is an interface for every resource managed by Capp.
type ResourceManager interface {
	Manage(capp cappv1alpha1.Capp) error
	CleanUp(capp cappv1alpha1.Capp) error
	IsRequired(capp cappv1alpha1.Capp) bool
}
