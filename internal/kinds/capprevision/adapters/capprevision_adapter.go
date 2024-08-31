package adapters

import (
	"context"
	"fmt"
	"strings"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
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

// copyAnnotations returns a map of annotations from a Capp that contain the Domain string.
func copyAnnotations(capp cappv1alpha1.Capp) map[string]string {
	annotations := make(map[string]string)
	for key, value := range capp.ObjectMeta.Annotations {
		if strings.Contains(key, domain) {
			annotations[key] = value
		}
	}
	return annotations
}

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
			Name:        kmeta.ChildName(capp.Name, fmt.Sprintf("-%05d", revisionNumber)),
			Namespace:   capp.Namespace,
			Labels:      map[string]string{cappNameLabelKey: capp.Name},
			Annotations: copyAnnotations(capp),
		},
		Spec: cappv1alpha1.CappRevisionSpec{
			CappTemplate: cappv1alpha1.CappTemplate{
				Labels:      capp.Labels,
				Annotations: capp.Annotations,
				Spec:        capp.Spec,
			},
			RevisionNumber: revisionNumber,
		},
	}

	if err := controllerutil.SetOwnerReference(&capp, &cappRevision, k8sClient.Scheme()); err != nil {
		return err
	}

	logger.Info(fmt.Sprintf("Trying to create CappRevision: %q.", cappRevision.Name))
	if err := k8sClient.Create(ctx, &cappRevision); err != nil {
		logger.Error(err, fmt.Sprintf("Failed to create CappRevision %q.", cappRevision.Name))
		return err
	}

	logger.Info(fmt.Sprintf("Successfully created CappRevision %q", cappRevision.Name))
	return nil
}

// DeleteCappRevision deletes a specified CappRevision and returning an error on failure.
func DeleteCappRevision(ctx context.Context, k8sClient client.Client, logger logr.Logger, cappRevision *cappv1alpha1.CappRevision) error {
	logger.Info(fmt.Sprintf("Trying to delete CappRevision: %q", cappRevision.Name))
	if err := k8sClient.Delete(ctx, cappRevision); err != nil {
		logger.Error(err, fmt.Sprintf("Failed to delete CappRevision %q.", cappRevision.Name))
		return err
	}

	return nil
}
