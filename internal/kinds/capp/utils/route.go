package utils

import (
	"context"
	"fmt"
	"strings"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"

	dnsrecordv1alpha1 "github.com/dana-team/provider-dns-v2/apis/namespaced/record/v1alpha1"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
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
		readyCondition := dnsRecord.Status.GetCondition(xpv1.TypeReady)
		available = readyCondition.Status == corev1.ConditionTrue && readyCondition.Reason == xpv1.ReasonAvailable
	}

	return available, nil
}

// GetDNSConfig returns the data of the DNS for the CappConfig CRD.
func GetDNSConfig(ctx context.Context, k8sClient client.Client) (cappv1alpha1.DNSConfig, error) {
	cappConfig := cappv1alpha1.CappConfig{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: CappNS, Name: CappConfigName}, &cappConfig); err != nil {
		return cappv1alpha1.DNSConfig{}, fmt.Errorf("could not fetch cappConfig %q from namespace %q: %w", CappConfigName, CappNS, err)
	}
	return cappConfig.Spec.DNSConfig, nil
}

// GetDNSRecordFromConfig returns the DNSRecord to be used for the record from a CappConfig CRD.
func GetDNSRecordFromConfig(dnsConfig cappv1alpha1.DNSConfig) (string, error) {
	dnsRecord := dnsConfig.CNAME

	if dnsRecord == "" {
		return "", fmt.Errorf("%q is empty in CappConfig DNSConfig data", "CNAME")
	}
	return dnsRecord, nil
}

// GetZoneFromConfig returns the zone to be used for the record from the CappConfig CRD.
func GetZoneFromConfig(dnsConfig cappv1alpha1.DNSConfig) (string, error) {
	zone := dnsConfig.Zone

	if zone == "" {
		return "", fmt.Errorf("%q is empty in CappConfig DNSConfig data", "Zone")
	} else if !strings.HasSuffix(zone, dot) {
		return "", fmt.Errorf("%q value must end with a %q in CappConfig DNSConfig data", "Zone", dot)
	}

	return zone, nil
}

// GetXPProviderFromConfig returns the Crossplane provider to be used for the record from the CappConfig CRD.
func GetXPProviderFromConfig(dnsConfig cappv1alpha1.DNSConfig) (string, error) {
	provider := dnsConfig.Provider

	if provider == "" {
		return "", fmt.Errorf("%q is empty in CappConfig DNSConfig data", "Provider")
	}

	return provider, nil
}

// GetIssuerNameFromConfig returns the name of the Certificate Issuer
// to be used for the Certificate from a CappConfig CRD.
func GetIssuerNameFromConfig(dnsConfig cappv1alpha1.DNSConfig) (string, error) {
	issuer := dnsConfig.Issuer

	if issuer == "" {
		return "", fmt.Errorf("%q is empty in CappConfig DNSConfig data", "Issuer")
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
