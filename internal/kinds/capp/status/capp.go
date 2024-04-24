package status

import (
	"context"

	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resource-managers"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateStateStatus changes the state status by identifying changes in the manifest
func CreateStateStatus(stateStatus *cappv1alpha1.StateStatus, cappStateFromSpec string) {
	if cappStateFromSpec != stateStatus.State || stateStatus.State == "" {
		stateStatus.State = cappStateFromSpec
		stateStatus.LastChange = metav1.Now()
	}
}

// SyncStatus is the main function that synchronizes the status of the Capp CRD with the Knative service and revisions associated with it.
// It gets the Capp CRD, builds the ApplicationLinks and RevisionInfo statuses, and updates the status of the Capp CRD if it has changed.
func SyncStatus(ctx context.Context, capp cappv1alpha1.Capp, log logr.Logger, r client.Client, onOpenshift bool, resourceManagers map[string]rmanagers.ResourceManager) error {
	cappObject := cappv1alpha1.Capp{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, &cappObject); err != nil {
		return err
	}

	applicationLinks, err := buildApplicationLinks(ctx, log, r, onOpenshift)
	if err != nil {
		return err
	}

	knativeServiceManger := resourceManagers[rmanagers.KnativeServing]
	knativeObjectStatus, revisionInfo, err := buildKnativeStatus(ctx, r, capp, knativeServiceManger.IsRequired(capp))
	if err != nil {
		return err
	}

	cappObject.Status.KnativeObjectStatus = knativeObjectStatus
	cappObject.Status.RevisionInfo = revisionInfo

	FlowManager := resourceManagers[rmanagers.Flow]
	loggingStatus, err := buildLoggingStatus(ctx, capp, log, r, FlowManager.IsRequired(capp))
	if err != nil {
		return err
	}
	cappObject.Status.LoggingStatus = loggingStatus

	DomainMappinManger := resourceManagers[rmanagers.DomainMapping]
	routeStatus, err := buildRouteStatus(ctx, r, capp, DomainMappinManger.IsRequired(capp))
	if err != nil {
		return err
	}
	cappObject.Status.RouteStatus = routeStatus

	NFSPVCManager := resourceManagers[rmanagers.NFSPVC]
	volumesStatus, err := buildVolumesStatus(ctx, r, capp, NFSPVCManager.IsRequired(capp))
	if err != nil {
		return err
	}
	cappObject.Status.VolumesStatus = volumesStatus

	CreateStateStatus(&cappObject.Status.StateStatus, capp.Spec.State)
	cappObject.Status.KnativeObjectStatus = knativeObjectStatus
	cappObject.Status.RevisionInfo = revisionInfo
	cappObject.Status.ApplicationLinks = *applicationLinks
	if err := r.Status().Update(ctx, &cappObject); err != nil {
		log.Error(err, "can't update capp status")
		return err
	}

	return nil
}
