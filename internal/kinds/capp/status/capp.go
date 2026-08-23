package status

import (
	"context"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kapis "knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// maxSyncErrorMessageLen enforce sync-error message length written to Capp.status.conditions
// so it always stays within metav1.Condition.Message's validation limit.
const maxSyncErrorMessageLen = 32768

// CreateStateStatus changes the state status by identifying changes in the manifest
func CreateStateStatus(stateStatus *cappv1alpha1.StateStatus, cappStateFromSpec string) {
	if cappStateFromSpec != stateStatus.State || stateStatus.State == "" {
		stateStatus.State = cappStateFromSpec
		stateStatus.LastChange = metav1.Now()
	}
}

// SyncStatus updates the Capp status subresource from the observed state of its managed resources.
func SyncStatus(ctx context.Context, capp cappv1alpha1.Capp, log logr.Logger, r client.Client, resourceManagers map[string]rmanagers.ResourceManager, cappConfig *cappv1alpha1.CappConfig, syncErrors []error) error {
	cappObject := cappv1alpha1.Capp{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, &cappObject); err != nil {
		return err
	}

	oldStatus := cappObject.Status.DeepCopy()

	knativeServiceManager := resourceManagers[rmanagers.KnativeService]
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
	routeStatus, err := buildRouteStatus(ctx, r, capp, routeRequired, cappConfig)
	if err != nil {
		return err
	}
	cappObject.Status.RouteStatus = routeStatus

	nfspvcManager := resourceManagers[rmanagers.NfsPvc]
	volumesStatus, err := buildVolumesStatus(ctx, r, capp, nfspvcManager.IsRequired(capp))
	if err != nil {
		return err
	}
	cappObject.Status.VolumesStatus = volumesStatus

	eventingStatus, err := buildEventingStatus(ctx, r, capp)
	if err != nil {
		return err
	}
	cappObject.Status.EventingStatus = eventingStatus

	CreateStateStatus(&cappObject.Status.StateStatus, capp.Spec.State)

	condition := computeReadyCondition(&cappObject.Status, capp, resourceManagers, syncErrors)
	meta.SetStatusCondition(&cappObject.Status.Conditions, condition)

	if equality.Semantic.DeepEqual(
		stripVolatileStatusFields(*oldStatus),
		stripVolatileStatusFields(cappObject.Status),
	) {
		return nil
	}

	log.Info("kubernetes API write status update", cappmeta.ObjectIdentityKeyVals(&cappObject)...)
	if err := r.Status().Update(ctx, &cappObject); err != nil {
		log.Error(err, "failed to update Capp status")
		return err
	}

	return nil
}

// computeReadyCondition determines the Ready condition by checking sync errors first,
// then cascading through each configured sub-resource's Ready status.
func computeReadyCondition(status *cappv1alpha1.CappStatus, capp cappv1alpha1.Capp, resourceManagers map[string]rmanagers.ResourceManager, syncErrors []error) metav1.Condition {
	if len(syncErrors) > 0 {
		return readyFalse(cappv1alpha1.CappReadyReasonResourceSyncFailed, formatSyncError(syncErrors[0]))
	}

	if resourceManagers[rmanagers.SyslogNGFlow].IsRequired(capp) {
		if reason, msg, ok := loggingNotReady(status.LoggingStatus); !ok {
			return readyFalse(reason, msg)
		}
	}

	if resourceManagers[rmanagers.DomainMapping].IsRequired(capp) {
		if reason, msg, ok := domainMappingNotReady(status.RouteStatus); !ok {
			return readyFalse(reason, msg)
		}
	}

	if resourceManagers[rmanagers.Certificate].IsRequired(capp) {
		if reason, msg, ok := certificateNotReady(status.RouteStatus); !ok {
			return readyFalse(reason, msg)
		}
	}

	if resourceManagers[rmanagers.NfsPvc].IsRequired(capp) {
		if reason, msg, ok := volumesNotReady(status.VolumesStatus); !ok {
			return readyFalse(reason, msg)
		}
	}

	if resourceManagers[rmanagers.PingSource].IsRequired(capp) ||
		resourceManagers[rmanagers.KafkaSource].IsRequired(capp) {
		if reason, msg, ok := eventingNotReady(status.EventingStatus); !ok {
			return readyFalse(reason, msg)
		}
	}

	if reason, msg, ok := knativeNotReady(status.KnativeObjectStatus); !ok {
		return readyFalse(reason, msg)
	}

	return metav1.Condition{
		Type:    cappv1alpha1.CappConditionReady,
		Status:  metav1.ConditionTrue,
		Reason:  cappv1alpha1.CappReadyReasonReady,
		Message: "Capp is ready",
	}
}

// formatSyncError truncates a child-resource sync failure's message so it always stays
// within metav1.Condition.Message's validation limit, regardless of how verbose an underlying error is.
func formatSyncError(err error) string {
	message := err.Error()
	if len(message) > maxSyncErrorMessageLen {
		message = message[:maxSyncErrorMessageLen] + "...(truncated)"
	}
	return message
}

func readyFalse(reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:    cappv1alpha1.CappConditionReady,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	}
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
	for i := range out.EventingStatus.EventSources {
		out.EventingStatus.EventSources[i].Condition.LastTransitionTime = kapis.VolatileTime{Inner: metav1.Time{}}
	}

	return out
}
