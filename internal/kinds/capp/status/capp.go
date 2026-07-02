package status

import (
	"context"

	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	utils "github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kapis "knative.dev/pkg/apis"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateStateStatus changes the state status by identifying changes in the manifest
func CreateStateStatus(stateStatus *cappv1alpha1.StateStatus, cappStateFromSpec string) {
	if cappStateFromSpec != stateStatus.State || stateStatus.State == "" {
		stateStatus.State = cappStateFromSpec
		stateStatus.LastChange = metav1.Now()
	}
}

// SyncStatus updates the Capp status subresource from the observed state of its managed resources.
func SyncStatus(ctx context.Context, capp cappv1alpha1.Capp, log logr.Logger, r client.Client, resourceManagers map[string]rmanagers.ResourceManager) error {
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
	routeStatus, err := buildRouteStatus(ctx, r, capp, routeRequired)
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

	buildCappConditions(&cappObject.Status, capp, resourceManagers)

	if equality.Semantic.DeepEqual(
		stripVolatileStatusFields(*oldStatus),
		stripVolatileStatusFields(cappObject.Status),
	) {
		return nil
	}

	log.Info("kubernetes API write status update", utils.ObjectIdentityKeyVals(&cappObject)...)
	if err := r.Status().Update(ctx, &cappObject); err != nil {
		log.Error(err, "failed to update Capp status")
		return err
	}

	return nil
}

// buildCappConditions derives top-level Capp conditions from the collected sub-statuses.
func buildCappConditions(status *cappv1alpha1.CappStatus, capp cappv1alpha1.Capp, resourceManagers map[string]rmanagers.ResourceManager) {
	condition := computeReadyCondition(status, capp, resourceManagers)
	meta.SetStatusCondition(&status.Conditions, condition)
}

// computeReadyCondition determines the Ready condition by cascading through
// each configured sub-resource, broadcasting its own Ready status.
func computeReadyCondition(status *cappv1alpha1.CappStatus, capp cappv1alpha1.Capp, resourceManagers map[string]rmanagers.ResourceManager) metav1.Condition {
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

func readyFalse(reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:    cappv1alpha1.CappConditionReady,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	}
}

func loggingNotReady(ls cappv1alpha1.LoggingStatus) (string, string, bool) {
	for _, c := range ls.Conditions {
		if c.Status == metav1.ConditionFalse {
			return cappv1alpha1.CappReadyReasonLoggingNotReady, c.Message, false
		}
	}
	return "", "", true
}

func domainMappingNotReady(rs cappv1alpha1.RouteStatus) (string, string, bool) {
	for _, c := range rs.DomainMappingObjectStatus.Conditions {
		if c.Type == kapis.ConditionReady && c.Status != corev1.ConditionTrue {
			return cappv1alpha1.CappReadyReasonDomainMappingNotReady, c.Message, false
		}
	}
	return "", "", true
}

func certificateNotReady(rs cappv1alpha1.RouteStatus) (string, string, bool) {
	for _, c := range rs.CertificateObjectStatus.Conditions {
		if string(c.Type) == "Ready" && c.Status != "True" {
			return cappv1alpha1.CappReadyReasonCertificateNotReady, c.Message, false
		}
	}
	return "", "", true
}

func volumesNotReady(vs cappv1alpha1.VolumesStatus) (string, string, bool) {
	for _, v := range vs.NFSVolumesStatus {
		if v.NFSPVCStatus.PvPhase != string(corev1.VolumeBound) ||
			v.NFSPVCStatus.PvcPhase != string(corev1.ClaimBound) {
			return cappv1alpha1.CappReadyReasonVolumesNotReady,
				"NFS volume " + v.VolumeName + " is not bound", false
		}
	}
	return "", "", true
}

func eventingNotReady(es cappv1alpha1.EventingStatus) (string, string, bool) {
	for _, src := range es.EventSources {
		if src.Condition.Status != corev1.ConditionTrue {
			return cappv1alpha1.CappReadyReasonEventingNotReady,
				"event source " + src.Name + " is not ready", false
		}
	}
	return "", "", true
}

func knativeNotReady(ks knativev1.ServiceStatus) (string, string, bool) {
	if len(ks.Conditions) == 0 {
		return cappv1alpha1.CappReadyReasonKnativeNotReady, "Knative Service has no status yet", false
	}

	if ks.LatestCreatedRevisionName != "" &&
		ks.LatestCreatedRevisionName != ks.LatestReadyRevisionName {
		return cappv1alpha1.CappReadyReasonKnativeNotReady,
			"latest revision " + ks.LatestCreatedRevisionName + " is not ready", false
	}

	for _, c := range ks.Conditions {
		if c.Type == kapis.ConditionReady {
			if c.Status == corev1.ConditionTrue {
				return "", "", true
			}
			return cappv1alpha1.CappReadyReasonKnativeNotReady, c.Message, false
		}
	}
	return cappv1alpha1.CappReadyReasonKnativeNotReady, "Knative Service Ready condition not found", false
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
