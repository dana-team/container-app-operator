package utils

import (
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

const (
	cappHaltAnnotation = "capp.dana.io/state"
)

// IsOnOpenshift returns true if the cluster has the openshift config group
func IsOnOpenshift(config *rest.Config) (bool, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return false, err
	}
	apiGroups, err := dc.ServerGroups()
	kind := schema.GroupVersionKind{Group: "config.openshift.io", Version: "v1", Kind: "ClusterVersion"}
	for _, apiGroup := range apiGroups.Groups {
		for _, supportedVersion := range apiGroup.Versions {
			if supportedVersion.GroupVersion == kind.GroupVersion().String() {
				return true, nil
			}
		}
	}
	return false, nil
}

// DoesHaltAnnotationExist checks if capp halt annotation exists in annotations
func DoesHaltAnnotationExist(annotations map[string]string) bool {
	for annotation := range annotations {
		if annotation == cappHaltAnnotation {
			return true
		}
	}
	return false
}

// FilterKeysWithoutPrefix removes keys from a map if they don't start with a given prefix
func FilterKeysWithoutPrefix(object map[string]string, prefix string) map[string]string {
	result := make(map[string]string)

	for key, value := range object {
		if strings.HasPrefix(key, prefix) {
			result[key] = value
		}
	}

	return result
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
