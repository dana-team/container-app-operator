package webhooks

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"

	rcsv1alpha1 "github.com/dana-team/rcs-ocm-deployer/api/v1alpha1"
	"github.com/dana-team/rcs-ocm-deployer/internal/utils"
	"k8s.io/apimachinery/pkg/types"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/strings/slices"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/network"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// isSiteValid checks if the specified site cluster name is valid or not.
// It takes a cappv1alpha1.Capp object, a list of placements, a Kubernetes client.Client, and a context.Context.
// The function returns a boolean value based on the validity of the specified site cluster name.
func isSiteValid(capp cappv1alpha1.Capp, placements []string, r client.Client, ctx context.Context) bool {
	if capp.Spec.Site == "" {
		return true
	}
	clusters, _ := getManagedClusters(r, ctx)
	return slices.Contains(clusters, capp.Spec.Site) || slices.Contains(placements, capp.Spec.Site)
}

// getManagedClusters retrieves the list of managed clusters from the Kubernetes API server
// and returns the list of cluster names as a slice of strings.
// If there is an error while retrieving the list of managed clusters, the function returns an error.
func getManagedClusters(r client.Client, ctx context.Context) ([]string, error) {
	var clusterNames []string
	clusters := clusterv1.ManagedClusterList{}
	if err := r.List(ctx, &clusters); err != nil {
		return clusterNames, err
	}
	for _, cluster := range clusters.Items {
		clusterNames = append(clusterNames, cluster.Name)
	}
	return clusterNames, nil
}

// validateDomainName checks if the hostname is valid domain name and not part of the cluster's domain.
// it returns aggregated error if any of the validations falied.
func validateDomainName(domainName string, invalidPatterns []string) (errs *apis.FieldError) {
	if domainName == "" {
		return nil
	}
	err := validation.IsFullyQualifiedDomainName(field.NewPath("name"), domainName)
	if err != nil {
		errs = errs.Also(apis.ErrGeneric(fmt.Sprintf(
			"invalid name %q: %s", domainName, err.ToAggregate()), "name"))
	}
	for _, pattern := range invalidPatterns {
		if pattern != "" {
			re := regexp.MustCompile(fmt.Sprintf("%v", pattern))
			if re.MatchString(domainName) {
				errs = errs.Also(apis.ErrGeneric(
					fmt.Sprintf("invalid name %q: must not match pattern %q", domainName, pattern), "name"))
			}
		}
	}

	clusterLocalDomain := network.GetClusterDomainName()
	if strings.HasSuffix(domainName, "."+clusterLocalDomain) {
		errs = errs.Also(apis.ErrGeneric(
			fmt.Sprintf("invalid name %q: must not be a subdomain of cluster local domain %q", domainName, clusterLocalDomain), "name"))
	}
	domainNameTaken, dnsErr := isDomainNameTaken(domainName)
	if dnsErr != nil {
		errs = errs.Also(apis.ErrGeneric(
			fmt.Sprintf("hostname check error: %v", dnsErr.Error())))
	}
	if domainNameTaken {
		errs = errs.Also(apis.ErrGeneric(
			fmt.Sprintf("invalid name %q: hostname must be unique and not already taken", domainName), "name"))
	}
	return errs
}

// isDomainNameTaken checks if the given hostname is already in use.
func isDomainNameTaken(domainName string) (bool, error) {
	_, err := net.LookupHost(domainName)
	if err != nil {
		if err.(*net.DNSError).IsNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// validateLogSpec checks if the LogSpec is valid based on the Type field.
func validateLogSpec(logSpec cappv1alpha1.LogSpec) *apis.FieldError {
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
func findMissingFields(logSpec cappv1alpha1.LogSpec, required []string) []string {
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

// getRCSConfig returns an instance of RCS Config.
func getRCSConfig(ctx context.Context, k8sClient client.Client) (*rcsv1alpha1.RCSConfig, error) {
	config := rcsv1alpha1.RCSConfig{}
	key := types.NamespacedName{Name: utils.RCSConfigName, Namespace: utils.RCSConfigNamespace}
	if err := k8sClient.Get(ctx, key, &config); err != nil {
		return nil, err
	}

	return &config, nil
}