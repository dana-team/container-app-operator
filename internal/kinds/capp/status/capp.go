package status

import (
	"context"
	"slices"

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

// Messages of the Ready condition when it is True.
const (
	readyMessage    = "Capp is ready"
	disabledMessage = "Capp is disabled"
)

// capability is a Capp feature reported as its own condition.
type capability struct {
	conditionType string
	// readyReason is the Ready reason when this capability is the root cause of the problem.
	readyReason string
	// dependsOn lists the capabilities this capability needs, if they are not ready this capability failure is only a symptom.
	dependsOn []string
	// managers are the resource managers of the capability's child resources.
	managers []string
	// check returns why the capability's child resources are not ready, or nil when they are.
	check func(ctx context.Context, k8sClient client.Client, capp cappv1alpha1.Capp, cappConfig *cappv1alpha1.CappConfig) (*metav1.Condition, error)
}

// capabilities are listed in dependency order; this also helps choose the main root cause.
var capabilities = []capability{
	{
		conditionType: cappv1alpha1.CappConditionVolumesReady,
		readyReason:   cappv1alpha1.CappReadyReasonVolumesNotReady,
		managers:      []string{rmanagers.NfsPvc},
		check:         checkVolumes,
	},
	{
		conditionType: cappv1alpha1.CappConditionServingReady,
		readyReason:   cappv1alpha1.CappReadyReasonServingNotReady,
		dependsOn:     []string{cappv1alpha1.CappConditionVolumesReady},
		managers:      []string{rmanagers.KnativeService},
		check:         checkKnative,
	},
	{
		conditionType: cappv1alpha1.CappConditionRouteReady,
		readyReason:   cappv1alpha1.CappReadyReasonRouteNotReady,
		dependsOn:     []string{cappv1alpha1.CappConditionServingReady},
		managers:      []string{rmanagers.DNSRecord, rmanagers.Certificate, rmanagers.DomainMapping},
		check:         checkRoute,
	},
	{
		conditionType: cappv1alpha1.CappConditionEventingReady,
		readyReason:   cappv1alpha1.CappReadyReasonEventingNotReady,
		dependsOn:     []string{cappv1alpha1.CappConditionServingReady},
		managers:      []string{rmanagers.PingSource, rmanagers.KafkaSource},
		check:         checkEventing,
	},
	{
		conditionType: cappv1alpha1.CappConditionLoggingReady,
		readyReason:   cappv1alpha1.CappReadyReasonLoggingNotReady,
		managers:      []string{rmanagers.SyslogNGOutput, rmanagers.SyslogNGFlow},
		check:         checkLogging,
	},
}

// CreateStateStatus changes the state status by identifying changes in the manifest
func CreateStateStatus(stateStatus *cappv1alpha1.StateStatus, cappStateFromSpec string) {
	if cappStateFromSpec != stateStatus.State || stateStatus.State == "" {
		stateStatus.State = cappStateFromSpec
		stateStatus.LastChange = metav1.Now()
	}
}

// SyncStatus updates the Capp status subresource from the observed state of its managed resources.
// syncErrors holds the errors of resource managers that failed to apply, keyed by manager name.
func SyncStatus(ctx context.Context, capp cappv1alpha1.Capp, log logr.Logger, r client.Client, resourceManagers map[string]rmanagers.ResourceManager, cappConfig *cappv1alpha1.CappConfig, syncErrors map[string]error) error {
	cappObject := cappv1alpha1.Capp{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, &cappObject); err != nil {
		return err
	}

	oldStatus := cappObject.Status.DeepCopy()

	if err := buildDeprecatedStatus(ctx, &cappObject.Status, capp, log, r, resourceManagers, cappConfig); err != nil {
		return err
	}

	eventingStatus, err := buildEventingStatus(ctx, r, capp)
	if err != nil {
		return err
	}
	cappObject.Status.EventingStatus = eventingStatus

	CreateStateStatus(&cappObject.Status.StateStatus, capp.Spec.State)

	if err := setStatusConditions(ctx, r, &cappObject.Status, capp, resourceManagers, cappConfig, syncErrors); err != nil {
		return err
	}
	cappObject.Status.ObservedGeneration = capp.Generation

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

// buildDeprecatedStatus fills the deprecated child status fields until they are removed.
//
//nolint:staticcheck // writes deprecated fields
func buildDeprecatedStatus(ctx context.Context, status *cappv1alpha1.CappStatus, capp cappv1alpha1.Capp, log logr.Logger, r client.Client, resourceManagers map[string]rmanagers.ResourceManager, cappConfig *cappv1alpha1.CappConfig) error {
	knativeObjectStatus, revisionInfo, err := buildKnativeStatus(ctx, r, capp, resourceManagers[rmanagers.KnativeService].IsRequired(capp))
	if err != nil {
		return err
	}
	status.KnativeObjectStatus = knativeObjectStatus
	status.RevisionInfo = revisionInfo

	loggingStatus, err := buildLoggingStatus(ctx, capp, log, r, status.LoggingStatus, resourceManagers[rmanagers.SyslogNGFlow].IsRequired(capp))
	if err != nil {
		return err
	}
	status.LoggingStatus = loggingStatus

	routeRequired := map[string]bool{
		rmanagers.DomainMapping: resourceManagers[rmanagers.DomainMapping].IsRequired(capp),
		rmanagers.DNSRecord:     resourceManagers[rmanagers.DNSRecord].IsRequired(capp),
		rmanagers.Certificate:   resourceManagers[rmanagers.Certificate].IsRequired(capp),
	}
	routeStatus, err := buildRouteStatus(ctx, r, capp, routeRequired, cappConfig)
	if err != nil {
		return err
	}
	status.RouteStatus = routeStatus

	volumesStatus, err := buildVolumesStatus(ctx, r, capp, resourceManagers[rmanagers.NfsPvc].IsRequired(capp))
	if err != nil {
		return err
	}
	status.VolumesStatus = volumesStatus

	return nil
}

// setStatusConditions sets a condition per configured capability.
func setStatusConditions(ctx context.Context, k8sClient client.Client, status *cappv1alpha1.CappStatus, capp cappv1alpha1.Capp, resourceManagers map[string]rmanagers.ResourceManager, cappConfig *cappv1alpha1.CappConfig, syncErrors map[string]error) error {
	for _, cp := range capabilities {
		syncErr := capabilitySyncError(cp, syncErrors)
		if syncErr == nil && !isCapabilityConfigured(cp, capp, resourceManagers) {
			meta.RemoveStatusCondition(&status.Conditions, cp.conditionType)
			continue
		}

		cond, err := computeCapabilityCondition(ctx, k8sClient, cp, capp, cappConfig, syncErr)
		if err != nil {
			return err
		}
		cond.ObservedGeneration = capp.Generation
		meta.SetStatusCondition(&status.Conditions, cond)
	}

	ready := computeReadyCondition(capp, status.Conditions)
	ready.ObservedGeneration = capp.Generation
	meta.SetStatusCondition(&status.Conditions, ready)
	return nil
}

// isCapabilityConfigured reports whether the Capp requires any of the capability's child resources.
func isCapabilityConfigured(cp capability, capp cappv1alpha1.Capp, resourceManagers map[string]rmanagers.ResourceManager) bool {
	if capp.Spec.State == cappv1alpha1.CappStateDisabled && slices.Contains(cp.dependsOn, cappv1alpha1.CappConditionServingReady) {
		return false
	}
	for _, name := range cp.managers {
		if resourceManagers[name].IsRequired(capp) {
			return true
		}
	}
	return false
}

// capabilitySyncError returns the first sync error from the capability's resource managers.
func capabilitySyncError(cp capability, syncErrors map[string]error) error {
	for _, name := range cp.managers {
		if err := syncErrors[name]; err != nil {
			return err
		}
	}
	return nil
}

// computeCapabilityCondition returns the condition of a configured capability.
func computeCapabilityCondition(ctx context.Context, k8sClient client.Client, cp capability, capp cappv1alpha1.Capp, cappConfig *cappv1alpha1.CappConfig, syncErr error) (metav1.Condition, error) {
	if syncErr != nil {
		return metav1.Condition{
			Type:    cp.conditionType,
			Status:  metav1.ConditionFalse,
			Reason:  cappv1alpha1.CapabilityReasonResourceSyncFailed,
			Message: truncateConditionMessage(syncErr.Error()),
		}, nil
	}

	cond, err := cp.check(ctx, k8sClient, capp, cappConfig)
	if err != nil {
		return metav1.Condition{}, err
	}
	if cond == nil {
		return metav1.Condition{Type: cp.conditionType, Status: metav1.ConditionTrue, Reason: cappv1alpha1.CapabilityReasonReady}, nil
	}
	cond.Type = cp.conditionType
	return *cond, nil
}

// computeReadyCondition computes Ready from the capability conditions:
// a sync error first, then the disabled state, then the first root cause.
func computeReadyCondition(capp cappv1alpha1.Capp, conditions []metav1.Condition) metav1.Condition {
	for _, cp := range capabilities {
		if cond := meta.FindStatusCondition(conditions, cp.conditionType); cond != nil && cond.Reason == cappv1alpha1.CapabilityReasonResourceSyncFailed {
			return rootCauseReadyCondition(cp, *cond)
		}
	}

	if capp.Spec.State == cappv1alpha1.CappStateDisabled {
		return metav1.Condition{
			Type:    cappv1alpha1.CappConditionReady,
			Status:  metav1.ConditionTrue,
			Reason:  cappv1alpha1.CappReadyReasonDisabled,
			Message: disabledMessage,
		}
	}

	// A False root cause is reported before an Unknown one.
	for _, status := range []metav1.ConditionStatus{metav1.ConditionFalse, metav1.ConditionUnknown} {
		for _, cp := range capabilities {
			cond := meta.FindStatusCondition(conditions, cp.conditionType)
			if cond != nil && cond.Status == status && areDependenciesReady(cp, conditions) {
				return rootCauseReadyCondition(cp, *cond)
			}
		}
	}

	return metav1.Condition{
		Type:    cappv1alpha1.CappConditionReady,
		Status:  metav1.ConditionTrue,
		Reason:  cappv1alpha1.CappReadyReasonReady,
		Message: readyMessage,
	}
}

// areDependenciesReady reports whether the configured dependencies of a capability are ready.
func areDependenciesReady(cp capability, conditions []metav1.Condition) bool {
	for _, dependency := range cp.dependsOn {
		if cond := meta.FindStatusCondition(conditions, dependency); cond != nil && cond.Status != metav1.ConditionTrue {
			return false
		}
	}
	return true
}

// rootCauseReadyCondition returns the Ready condition that reports a capability as the root cause.
func rootCauseReadyCondition(cp capability, cond metav1.Condition) metav1.Condition {
	return metav1.Condition{
		Type:    cappv1alpha1.CappConditionReady,
		Status:  cond.Status,
		Reason:  cp.readyReason,
		Message: truncateConditionMessage(cond.Reason + ": " + cond.Message),
	}
}

// stripVolatileStatusFields clears condition transition timestamps for status comparison.
//
//nolint:staticcheck // reads deprecated fields
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
	for i := range out.VolumesStatus.NFSVolumesStatus {
		for j := range out.VolumesStatus.NFSVolumesStatus[i].NFSPVCStatus.Conditions {
			out.VolumesStatus.NFSVolumesStatus[i].NFSPVCStatus.Conditions[j].LastTransitionTime = metav1.Time{}
		}
	}

	return out
}
