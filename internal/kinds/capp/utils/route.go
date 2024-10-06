package utils

import (
	"context"
	"fmt"
	"strings"

	dnsrecordv1alpha1 "github.com/dana-team/provider-dns/apis/record/v1alpha1"

	xpcommonv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	dnsCM               = "dns-config"
	zoneKey             = "zone"
	cnameKey            = "cname"
	issuerKey           = "issuer"
	providerKey         = "provider"
	placeholderZone     = "capp.com."
	placeholderIssuer   = "cert-issuer"
	placeholderProvider = "dns-default"
	dot                 = "."
	maxCommonNameLength = 64
)

// IsDNSRecordAvailable returns a boolean indicating whether a CNAMERecord is currently available.
func IsDNSRecordAvailable(ctx context.Context, k8sClient client.Client, name, namespace string) (bool, error) {
	var available bool

	dnsRecord := dnsrecordv1alpha1.CNAMERecord{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &dnsRecord); err != nil {
		return false, fmt.Errorf("failed getting DNSRecord: %w", err)
	}

	if dnsRecord.Status.Conditions != nil {
		readyCondition := dnsRecord.Status.GetCondition(xpcommonv1.TypeReady)
		available = readyCondition.Equal(xpcommonv1.Available())
	}

	return available, nil
}

// GetDNSConfig returns the data of the DNS ConfigMap.
func GetDNSConfig(ctx context.Context, k8sClient client.Client) (map[string]string, error) {
	routeCM := corev1.ConfigMap{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: CappNS, Name: dnsCM}, &routeCM); err != nil {
		return nil, fmt.Errorf("could not fetch configMap %q from namespace %q: %w", dnsCM, CappNS, err)
	}

	return routeCM.Data, nil
}

// GetDNSRecordFromConfig returns the DNSRecord to be used for the record from a ConfigMap.
func GetDNSRecordFromConfig(dnsConfig map[string]string) (string, error) {
	dnsRecord := ""
	var ok bool

	if len(dnsConfig) > 0 {
		dnsRecord, ok = dnsConfig[cnameKey]
		if !ok {
			return dnsRecord, fmt.Errorf("%q key is not set in ConfigMap %q", cnameKey, dnsCM)
		} else if dnsRecord == "" {
			return dnsRecord, fmt.Errorf("%q is empty in ConfigMap %q", cnameKey, dnsCM)
		}
	}

	return dnsRecord, nil
}

// GetZoneFromConfig returns the zone to be used for the record from a ConfigMap.
func GetZoneFromConfig(dnsConfig map[string]string) (string, error) {
	var ok bool
	zone := placeholderZone
	if len(dnsConfig) > 0 {
		zone, ok = dnsConfig[zoneKey]
		if !ok {
			return zone, fmt.Errorf("%q key is not set in ConfigMap %q", zoneKey, dnsCM)
		} else if zone == "" {
			return zone, fmt.Errorf("%q is empty in ConfigMap %q", zoneKey, dnsCM)
		} else if !strings.HasSuffix(zone, dot) {
			return zone, fmt.Errorf("%q value must end with a %q in ConfigMap %q", zoneKey, dot, dnsCM)
		}
	}

	return zone, nil
}

// GetXPProviderFromConfig returns the Crossplane provider to be used for the record from a ConfigMap.
func GetXPProviderFromConfig(dnsConfig map[string]string) (string, error) {
	var ok bool
	provider := placeholderProvider
	if len(dnsConfig) > 0 {
		provider, ok = dnsConfig[providerKey]
		if !ok {
			return provider, fmt.Errorf("%q key is not set in ConfigMap %q", providerKey, dnsCM)
		} else if provider == "" {
			return provider, fmt.Errorf("%q is empty in ConfigMap %q", providerKey, dnsCM)
		}
	}

	return provider, nil
}

// GetIssuerNameFromConfig returns the name of the Certificate Issuer
// to be used for the Certificate from a ConfigMap.
func GetIssuerNameFromConfig(dnsConfig map[string]string) (string, error) {
	var ok bool
	issuer := placeholderIssuer
	if len(dnsConfig) > 0 {
		issuer, ok = dnsConfig[issuerKey]
		if !ok {
			return issuer, fmt.Errorf("%q key is not set in ConfigMap %q", issuerKey, dnsCM)
		} else if issuer == "" {
			return issuer, fmt.Errorf("%q is empty in ConfigMap %q", issuerKey, dnsCM)
		}
	}

	return issuer, nil
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
