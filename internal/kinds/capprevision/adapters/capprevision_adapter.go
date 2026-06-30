package adapters

import (
	"context"
	"fmt"
	"maps"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	domain           = cappv1alpha1.GroupVersion.Group
	cappNameLabelKey = domain + "/cappName"
)

const (
	ClientListLimit = 100
)

// GetCappRevisions retrieves a list of CappRevision resources filtered by labels matching a specific Capp, returning the list and any error encountered.
func GetCappRevisions(ctx context.Context, r client.Client, capp cappv1alpha1.Capp) ([]cappv1alpha1.CappRevision, error) {
	cappRevisions := cappv1alpha1.CappRevisionList{}

	requirement, err := labels.NewRequirement(cappNameLabelKey, selection.Equals, []string{capp.Name})
	if err != nil {
		return cappRevisions.Items, err
	}

	labelSelector := labels.NewSelector().Add(*requirement)
	listOptions := client.ListOptions{
		Namespace:     capp.Namespace,
		LabelSelector: labelSelector,
		Limit:         ClientListLimit,
	}

	err = r.List(ctx, &cappRevisions, &listOptions)
	return cappRevisions.Items, err
}

// CreateCappRevision initializes and creates a CappRevision.
func CreateCappRevision(ctx context.Context, k8sClient client.Client, logger logr.Logger, capp cappv1alpha1.Capp, revisionNumber int) error {
	cappRevision := cappv1alpha1.CappRevision{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(capp.Name, fmt.Sprintf("-%05d", revisionNumber)),
			Namespace: capp.Namespace,
			Labels:    map[string]string{cappNameLabelKey: capp.Name},
		},
		Spec: cappv1alpha1.CappRevisionSpec{
			CappTemplate: cappv1alpha1.CappTemplate{
				Labels:      maps.Clone(capp.Labels),
				Annotations: maps.Clone(capp.Annotations),
				Spec:        *capp.Spec.DeepCopy(),
			},
			RevisionNumber: revisionNumber,
		},
	}

	if err := controllerutil.SetOwnerReference(&capp, &cappRevision, k8sClient.Scheme()); err != nil {
		return err
	}

	return rclient.ResourceManagerClient{K8sClient: k8sClient, Log: logger}.CreateResource(ctx, &cappRevision)
}

// DeleteCappRevision deletes a specified CappRevision and returning an error on failure.
func DeleteCappRevision(ctx context.Context, k8sClient client.Client, logger logr.Logger, cappRevision *cappv1alpha1.CappRevision) error {
	return rclient.ResourceManagerClient{K8sClient: k8sClient, Log: logger}.DeleteResource(ctx, cappRevision)
}
