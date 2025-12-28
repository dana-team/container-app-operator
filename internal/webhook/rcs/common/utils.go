package common

import (
	"context"
	"fmt"

	"net"
	"regexp"
	"strings"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"k8s.io/apimachinery/pkg/types"

	v1alpha2 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/network"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ValidateDomainName checks if the hostname is valid domain name and not part of the cluster's domain.
// it returns aggregated error if any of the validations falied.
func ValidateDomainName(domainName string, allowedPatterns []string) (errs *apis.FieldError) {
	if domainName == "" {
		return nil
	}
	err := validation.IsFullyQualifiedDomainName(field.NewPath("name"), domainName)
	if err != nil {
		errs = errs.Also(apis.ErrGeneric(fmt.Sprintf(
			"invalid name %q: %s", domainName, err.ToAggregate()), "name"))
	}
	matched := false
	for _, pattern := range allowedPatterns {
		if pattern != "" {
			re, err := regexp.Compile(fmt.Sprintf("%v", pattern))
			if err != nil {
				errs = errs.Also(apis.ErrGeneric(fmt.Sprintf("invalid pattern %q: %s", pattern, err), "allowedHostnamePatterns"))
				continue
			}
			if re.MatchString(domainName) {
				matched = true
				break
			}
		}
	}
	if !matched {
		errs = errs.Also(apis.ErrGeneric(
			fmt.Sprintf("invalid name %q: must match one of the allowed patterns", domainName), "name"))
	}

	clusterLocalDomain := network.GetClusterDomainName()
	if strings.HasSuffix(domainName, "."+clusterLocalDomain) {
		errs = errs.Also(apis.ErrGeneric(
			fmt.Sprintf("invalid name %q: must not be a subdomain of cluster local domain %q", domainName, clusterLocalDomain), "name"))
	}
	return errs
}

// IsDomainNameTaken checks if the given hostname is already in use.
func IsDomainNameTaken(domainName string) (bool, error) {
	_, err := net.LookupHost(domainName)
	if err != nil {
		if err.(*net.DNSError).IsNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ValidateLogSpec checks if the LogSpec is valid based on the Type field.
func ValidateLogSpec(logSpec v1alpha2.LogSpec) *apis.FieldError {
	requiredFields := map[string][]string{
		"elastic": {"Host", "Index", "User", "PasswordSecret"},
	}
	required, exists := requiredFields[logSpec.Type]
	if !exists {
		validTypes := make([]string, 0, len(requiredFields))
		for validType := range requiredFields {
			validTypes = append(validTypes, validType)
		}
		return apis.ErrGeneric(
			fmt.Sprintf("Invalid LogSpec Type: %q. Valid types are: %q", logSpec.Type, strings.Join(validTypes, ", ")),
			"logSpec.Type")
	}
	missingFields := findMissingFields(logSpec, required)
	if len(missingFields) > 0 {
		return apis.ErrGeneric(
			fmt.Sprintf("%s log configuration is missing required fields: %q", logSpec.Type, strings.Join(missingFields, ", ")),
			"logSpec")
	}
	return nil
}

// findMissingFields checks for missing fields in LogSpec.
func findMissingFields(logSpec v1alpha2.LogSpec, required []string) []string {
	var missingFields []string
	fieldValues := map[string]string{
		"Host":           logSpec.Host,
		"Index":          logSpec.Index,
		"User":           logSpec.User,
		"PasswordSecret": logSpec.PasswordSecret,
	}
	for _, reqField := range required {
		if value, ok := fieldValues[reqField]; !ok || value == "" {
			missingFields = append(missingFields, reqField)
		}
	}
	return missingFields
}

// GetCappConfig returns an instance of Capp Config.
func GetCappConfig(ctx context.Context, k8sClient client.Client) (*v1alpha2.CappConfig, error) {
	config := v1alpha2.CappConfig{}
	key := types.NamespacedName{Name: utils.CappConfigName, Namespace: utils.CappNS}
	if err := k8sClient.Get(ctx, key, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
