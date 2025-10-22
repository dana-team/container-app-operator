package utils

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

var (
	CappAPIGroup      = cappv1alpha1.GroupVersion.Group
	CappNamespaceKey  = CappAPIGroup + "/parent-capp-ns"
	CappResourceKey   = CappAPIGroup + "/parent-capp"
	ManagedByLabelKey = CappAPIGroup + "/managed-by"
)

const (
	CappConfigName = "capp-config"
	CappNS         = "container-app-operator-system"
	CappKey        = "capp"
)

// IsOnOpenshift returns true if the cluster has the openshift config group
func IsOnOpenshift(config *rest.Config) (bool, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return false, err
	}
	apiGroups, err := dc.ServerGroups()
	if err != nil {
		return false, err
	}
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

// FilterMap returns a new map containing only the key-value pairs
// where the key contains the specified substring.
func FilterMap(originalMap map[string]string, substring string) map[string]string {
	filteredMap := make(map[string]string)
	for key, value := range originalMap {
		if strings.Contains(key, substring) {
			filteredMap[key] = value
		}
	}
	return filteredMap
}

// GenerateSecretName generates TLS secret name for certificate and domain mapping.
func GenerateSecretName(resourceName string) string {
	return fmt.Sprintf("%s-tls", resourceName)
}

// GetListOptions returns a list option object from a given Set.
func GetListOptions(set labels.Set) client.ListOptions {
	labelSelector := labels.SelectorFromSet(set)
	listOptions := client.ListOptions{
		LabelSelector: labelSelector,
	}

	return listOptions
}

// GetResource fetches an existing resource and returns an instance of it.
func GetResource(k8sClient client.Client, obj client.Object, name, namespace string) error {
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: name, Namespace: namespace}, obj); err != nil {
		return err
	}
	return nil
}

// GetCappConfig fetches and returns an existing instance of an existing cappConfig
func GetCappConfig(k8sClient client.Client) (*cappv1alpha1.CappConfig, error) {
	cappConfig := &cappv1alpha1.CappConfig{}
	if err := GetResource(k8sClient, cappConfig, CappConfigName, CappNS); err != nil {
		return nil, err
	}
	return cappConfig, nil
}
