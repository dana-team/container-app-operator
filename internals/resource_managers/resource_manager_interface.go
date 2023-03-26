package resourcemanagers

import (
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ResourceManager interface {
	PrepareResource(capp rcsv1alpha1.Capp) client.Object
	CreateOrUpdateResource(capp rcsv1alpha1.Capp) error
	CreateResource(resource client.Object) error
	UpdateResource(resource client.Object, oldResource client.Object) error
	DeleteResource(capp rcsv1alpha1.Capp) error
}
