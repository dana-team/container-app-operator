package cappmeta

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
)

var (
	CappAPIGroup              = cappv1alpha1.GroupVersion.Group
	CappNamespaceKey          = CappAPIGroup + "/parent-capp-ns"
	CappResourceKey           = CappAPIGroup + "/parent-capp"
	ManagedByLabelKey         = CappAPIGroup + "/managed-by"
	SkipDNSCheckAnnotationKey = CappAPIGroup + "/skip-dns-check"
	RolloutAnnotationKey      = CappAPIGroup + "/rollout"
)

const (
	CappConfigName = "capp-config"
	CappNS         = "container-app-operator-system"
	CappKey        = "capp"

	// RolloutActionPromote is the value of RolloutAnnotationKey that promotes the candidate revision.
	RolloutActionPromote = "promote"

	// RolloutActionAbort is the value of RolloutAnnotationKey that aborts the rollout.
	RolloutActionAbort = "abort"

	// RolloutActionRetry is the value of RolloutAnnotationKey that restarts a failed, rolled back or aborted rollout,
	// or a rollout held by a strategy change.
	RolloutActionRetry = "retry"
)

// ManagedResourceLabels returns labels used for child resources reconciled from a Capp.
func ManagedResourceLabels(cappName string) map[string]string {
	return map[string]string{
		CappResourceKey:   cappName,
		ManagedByLabelKey: CappKey,
	}
}

// MergeMaps merges two string-string maps by combining their key-value pairs into a new map.
func MergeMaps(m1 map[string]string, m2 map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range m1 {
		merged[k] = v
	}
	for key, value := range m2 {
		merged[key] = value
	}
	return merged
}

// ObjectIdentityKeyVals returns key/value pairs identifying obj for structured logs.
func ObjectIdentityKeyVals(obj client.Object) []any {
	gvk := obj.GetObjectKind().GroupVersionKind()
	kind := gvk.Kind
	if kind == "" {
		kind = "Unknown"
	}
	return []any{
		"kind", kind,
		"group", gvk.Group,
		"version", gvk.Version,
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
		"resourceVersion", obj.GetResourceVersion(),
	}
}
