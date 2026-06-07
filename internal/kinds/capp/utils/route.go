package utils

import (
	"context"
	"fmt"
	"strings"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	dot                 = "."
	maxCommonNameLength = 64
)

// GetDNSConfig returns the data of the DNS for the CappConfig CRD.
func GetDNSConfig(ctx context.Context, k8sClient client.Client) (cappv1alpha1.DNSConfig, error) {
	cappConfig := cappv1alpha1.CappConfig{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: CappNS, Name: CappConfigName}, &cappConfig); err != nil {
		return cappv1alpha1.DNSConfig{}, fmt.Errorf("could not fetch cappConfig %q from namespace %q: %w", CappConfigName, CappNS, err)
	}
	return cappConfig.Spec.DNSConfig, nil
}

// GenerateResourceName generates the hostname based on the provided suffix and a dot(".") trailing character.
// If the hostname does not already end with the suffix (minus the trailing dot), it appends the suffix to the hostname.
func GenerateResourceName(hostname, suffix string) string {
	suffixWithoutTrailingChar := suffix[:len(suffix)-len(dot)]
	if !strings.HasSuffix(hostname, suffixWithoutTrailingChar) {
		return hostname + dot + suffixWithoutTrailingChar
	}

	return hostname
}

// GenerateRecordName generates the hostname based on the provided suffix and a dot(".") trailing character.
// It returns the original hostname with the suffix removed if it was present, otherwise the original hostname.
func GenerateRecordName(hostname, suffix string) string {
	suffixWithoutTrailingChar := suffix[:len(suffix)-len(dot)]
	if !strings.HasSuffix(hostname, suffixWithoutTrailingChar) {
		return hostname
	}

	return hostname[:len(hostname)-len(suffix)]
}

// IsCustomHostnameSet returns a boolean indicating whether a custom hostname is set.
func IsCustomHostnameSet(hostname string) bool {
	return hostname != ""
}

// TruncateCommonName truncates the CommonName string to be no longer than 64 characters.
func TruncateCommonName(s string) string {
	if len(s) > maxCommonNameLength {
		return s[:maxCommonNameLength]
	}
	return s
}
