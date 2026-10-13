package status

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Limits of metav1.Condition fields.
	maxConditionMessageLen = 32768
	maxConditionReasonLen  = 1024

	truncatedSuffix = "...(truncated)"

	// kindRevision names Knative revisions in condition messages.
	kindRevision = "Revision"
)

var conditionReasonPattern = regexp.MustCompile(`^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$`)

// getChildResource gets a child resource into obj. It returns a Pending condition when the child does not exist.
func getChildResource(ctx context.Context, k8sClient client.Client, kind string, key types.NamespacedName, obj client.Object) (*metav1.Condition, error) {
	if err := k8sClient.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return newPendingCondition(kind, key.Name), nil
		}
		return nil, err
	}
	return nil, nil
}

// newPendingCondition returns the condition for a child that is missing or not yet observed by its controller.
func newPendingCondition(kind, name string) *metav1.Condition {
	return &metav1.Condition{
		Status:  metav1.ConditionUnknown,
		Reason:  cappv1alpha1.CapabilityReasonPending,
		Message: fmt.Sprintf("%s %s not observed yet", kind, name),
	}
}

// formatChildCondition formats a child condition as a capability condition, or returns nil when it is True.
func formatChildCondition(kind, name string, conditionStatus metav1.ConditionStatus, reason, message string) *metav1.Condition {
	if conditionStatus == metav1.ConditionTrue {
		return nil
	}

	fallbackReason := cappv1alpha1.CapabilityReasonNotReady
	if conditionStatus != metav1.ConditionFalse {
		conditionStatus = metav1.ConditionUnknown
		fallbackReason = cappv1alpha1.CapabilityReasonPending
	}
	if len(reason) > maxConditionReasonLen || !conditionReasonPattern.MatchString(reason) {
		reason = fallbackReason
	}

	text := kind + " " + name
	if message != "" {
		text += ": " + message
	}
	return &metav1.Condition{Status: conditionStatus, Reason: reason, Message: truncateConditionMessage(text)}
}

// isConditionStale reports whether a condition was set for an older generation of obj. Zero means not tracked.
func isConditionStale(observedGeneration int64, obj metav1.Object) bool {
	return observedGeneration != 0 && observedGeneration < obj.GetGeneration()
}

// truncateConditionMessage keeps message within the metav1.Condition message limit.
func truncateConditionMessage(message string) string {
	if len(message) <= maxConditionMessageLen {
		return message
	}
	return strings.ToValidUTF8(message[:maxConditionMessageLen-len(truncatedSuffix)], "") + truncatedSuffix
}
