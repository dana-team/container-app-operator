package status

import (
	"context"

	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/equality"
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

	oldStatus := cappObject.Status.DeepCopy()

	applicationLinks, err := buildApplicationLinks(ctx, log, r, onOpenshift)
	if err != nil {
		return err
	}

	knativeServiceManager := resourceManagers[rmanagers.KnativeServing]
	knativeObjectStatus, revisionInfo, err := buildKnativeStatus(ctx, r, capp, knativeServiceManager.IsRequired(capp))
	if err != nil {
		return err
	}

	cappObject.Status.KnativeObjectStatus = knativeObjectStatus
	cappObject.Status.RevisionInfo = revisionInfo

	syslogNGFlowManager := resourceManagers[rmanagers.SyslogNGFlow]
	loggingStatus, err := buildLoggingStatus(ctx, capp, log, r, cappObject.Status.LoggingStatus, syslogNGFlowManager.IsRequired(capp))
	if err != nil {
		return err
	}
	cappObject.Status.LoggingStatus = loggingStatus

	routeRequired := map[string]bool{
		rmanagers.DomainMapping: resourceManagers[rmanagers.DomainMapping].IsRequired(capp),
		rmanagers.DNSRecord:     resourceManagers[rmanagers.DNSRecord].IsRequired(capp),
		rmanagers.Certificate:   resourceManagers[rmanagers.Certificate].IsRequired(capp),
	}
	routeStatus, err := buildRouteStatus(ctx, r, capp, routeRequired)
	if err != nil {
		return err
	}
	cappObject.Status.RouteStatus = routeStatus

	nfspvcManager := resourceManagers[rmanagers.NfsPVC]
	volumesStatus, err := buildVolumesStatus(ctx, r, capp, nfspvcManager.IsRequired(capp))
	if err != nil {
		return err
	}
	cappObject.Status.VolumesStatus = volumesStatus

	eventSourceManager := resourceManagers[rmanagers.EventSources]
	eventingStatus, err := buildEventingStatus(ctx, capp, r, eventSourceManager.IsRequired(capp))
	if err != nil {
		return err
	}
	cappObject.Status.EventingStatus = eventingStatus

	CreateStateStatus(&cappObject.Status.StateStatus, capp.Spec.State)
	cappObject.Status.ApplicationLinks = *applicationLinks

	if equality.Semantic.DeepEqual(
		stripVolatileStatusFields(*oldStatus),
		stripVolatileStatusFields(cappObject.Status),
	) {
		return nil
	}

	log.Info("kubernetes API write status update", rclient.ObjectIdentityKeyVals(&cappObject)...)
	if err := r.Status().Update(ctx, &cappObject); err != nil {
		log.Error(err, "failed to update Capp status")
		return err
	}

	return nil
}

// stripVolatileStatusFields clears condition transition timestamps for status comparison.
func stripVolatileStatusFields(s cappv1alpha1.CappStatus) cappv1alpha1.CappStatus {
	out := *s.DeepCopy()
	for i := range out.Conditions {
		out.Conditions[i].LastTransitionTime = metav1.Time{}
	}
	for i := range out.LoggingStatus.Conditions {
		out.LoggingStatus.Conditions[i].LastTransitionTime = metav1.Time{}
	}
	for i := range out.RouteStatus.CertificateObjectStatus.Conditions {
		out.RouteStatus.CertificateObjectStatus.Conditions[i].LastTransitionTime = nil
	}
	for i := range out.RouteStatus.DNSRecordObjectStatus.CNAMERecordObjectStatus.Conditions {
		out.RouteStatus.DNSRecordObjectStatus.CNAMERecordObjectStatus.Conditions[i].LastTransitionTime = metav1.Time{}
	}
	return out
}
