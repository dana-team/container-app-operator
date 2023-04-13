package resourceprepares

import (
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
)

type ResourceManager interface {
	CreateOrUpdateObject(capp rcsv1alpha1.Capp) error
	CleanUp(capp rcsv1alpha1.Capp) error
}
