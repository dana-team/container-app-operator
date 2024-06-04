package utils

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	zoneCM          = "dns-zone"
	zoneKey         = "zone"
	placeholderZone = "capp.com."
	dot             = "."
)

// GetZoneFromConfig returns the zone to be used for the record from a ConfigMap.
func GetZoneFromConfig(ctx context.Context, k8sClient client.Client) (string, error) {
	var ok bool

	routeCM := corev1.ConfigMap{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: CappNS, Name: zoneCM}, &routeCM); err != nil {
		return "", fmt.Errorf("could not fetch configMap %q from namespace %q: %w", zoneCM, CappNS, err)
	}

	zone := placeholderZone
	if len(routeCM.Data) > 0 {
		zone, ok = routeCM.Data[zoneKey]
		if !ok {
			return zone, fmt.Errorf("%q key is not set in ConfigMap %q", zoneKey, zoneCM)
		} else if zone == "" {
			return zone, fmt.Errorf("%q is empty in ConfigMap %q", zoneKey, zoneCM)
		} else if !strings.HasSuffix(zone, dot) {
			return zone, fmt.Errorf("%q value must end with a %q in ConfigMap %q", zoneKey, dot, zoneCM)
		}
	}

	return zone, nil
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
