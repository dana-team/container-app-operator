// Package status_utils This is a Go package that contains functions for synchronizing the status
// of a custom resource definition (CRD)
// called Capp with the status of the Knative service and revisions associated with it.
// The SyncStatus function is the main function that orchestrates the synchronization process.
package status_utils

import (
	"context"
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateEnabledStatus responsible to change The enabled status by identfying changes in the spec
func CreateEnabledStatus(enabledStatus *rcsv1alpha1.EnabledStatus, enabledFromSpec bool) {
	if enabledFromSpec != enabledStatus.IsEnabled {
		enabledStatus.IsEnabled = enabledFromSpec
		enabledStatus.LastChange = metav1.Now()
	}
}

// This is the main function that synchronizes the status of the Capp CRD with the Knative service and revisions associated with it.
// It gets the Capp CRD, builds the ApplicationLinks and RevisionInfo statuses, and updates the status of the Capp CRD if it has changed.
func SyncStatus(ctx context.Context, capp rcsv1alpha1.Capp, log logr.Logger, r client.Client, onOpenshift bool) error {
	cappObject := rcsv1alpha1.Capp{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, &cappObject); err != nil {
		return err
	}

	applicationLinks, err := buildApplicationLinks(ctx, log, r, onOpenshift)
	if err != nil {
		return err
	}

	knativeObjectStatus, revisionInfo, err := buildKnativeStatus(ctx, r, log, capp)
	if err != nil {
		return err
	}

	cappObject.Status.KnativeObjectStatus = knativeObjectStatus
	cappObject.Status.RevisionInfo = revisionInfo
	if cappObject.Spec.LogSpec != (rcsv1alpha1.LogSpec{}) {
		loggingStatus, err := buildLoggingStatus(ctx, capp, log, r)
		if err != nil {
			return err
		}
		cappObject.Status.LoggingStatus = loggingStatus
	}

	CreateEnabledStatus(&cappObject.Status.EnabledStatus, capp.Spec.Enabled)
	cappObject.Status.KnativeObjectStatus = knativeObjectStatus
	cappObject.Status.RevisionInfo = revisionInfo
	cappObject.Status.ApplicationLinks = *applicationLinks
	if err := r.Status().Update(ctx, &cappObject); err != nil {
		log.Error(err, "can't update capp status")
		return err
	}

	return nil
}
