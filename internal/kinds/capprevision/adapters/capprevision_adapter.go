package adapters

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	CappNameLabelKey   = "rcs.dana.io/cappName"
	ClientListLimit    = 100
	charSet            = "abcdefghijklmnopqrstuvwxyz0123456789"
	RandomStringLength = 5
	IndexCut           = 50
)

var seededRand = rand.New(rand.NewSource(time.Now().UnixNano()))

// generateRandomString returns a random string of the specified length using characters from the charset.
func generateRandomString(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charSet[seededRand.Intn(len(charSet))]
	}
	return string(b)
}

// getSubstringUntilIndex returns a substring from the start of the given string up to (but not including) the character at the specified index.
// If the index is out of bounds, it returns the entire string.
func getSubstringUntilIndex(s string, index int) string {
	if index < 0 || index > len(s) {
		return s
	}
	return s[:index]
}

// GetCappRevisions retrieves a list of CappRevision resources filtered by labels matching a specific Capp, returning the list and any error encountered.
func GetCappRevisions(ctx context.Context, r client.Client, capp cappv1alpha1.Capp) ([]cappv1alpha1.CappRevision, error) {
	cappRevisions := cappv1alpha1.CappRevisionList{}

	requirement, err := labels.NewRequirement(CappNameLabelKey, selection.Equals, []string{capp.Name})
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
			Name: fmt.Sprintf("%s-%s-v%s", getSubstringUntilIndex(capp.Name, IndexCut),
				generateRandomString(RandomStringLength), strconv.Itoa(revisionNumber)),
			Namespace: capp.Namespace,
			Labels:    map[string]string{CappNameLabelKey: capp.Name},
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
