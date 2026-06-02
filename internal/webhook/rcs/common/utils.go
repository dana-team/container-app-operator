package common

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"k8s.io/apimachinery/pkg/types"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/network"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ValidateDomainName checks if the hostname is valid domain name and not part of the cluster's domain.
// It returns aggregated error if any of the validations failed.
func ValidateDomainName(domainName string, allowedPatterns []cappv1alpha1.HostnamePattern) (errs *apis.FieldError) {
	if domainName == "" {
		return nil
	}
	err := validation.IsFullyQualifiedDomainName(field.NewPath("name"), domainName)
	if err != nil {
		errs = errs.Also(apis.ErrGeneric(fmt.Sprintf(
			"invalid name %q: %s", domainName, err.ToAggregate()), "name"))
	}
	matched := false
	descriptions := make([]string, 0, len(allowedPatterns))
	for i, hp := range allowedPatterns {
		re, err := regexp.Compile(hp.Match)
		if err != nil {
			errs = errs.Also(apis.ErrGeneric(fmt.Sprintf("invalid pattern %q: %s", hp.Match, err), fmt.Sprintf("allowedHostnamePatterns[%d].pattern", i)))
			continue
		}
		if hp.Explanation != "" {
			descriptions = append(descriptions, hp.Explanation)
		} else {
			descriptions = append(descriptions, hp.Match)
		}
		if !matched && re.MatchString(domainName) {
			matched = true
			break
		}
	}
	if !matched {
		msg := fmt.Sprintf("invalid name %q: must match one of the allowed patterns", domainName)
		if len(descriptions) > 0 {
			msg = fmt.Sprintf("%s (%s)", msg, strings.Join(descriptions, ", "))
		}
		errs = errs.Also(apis.ErrGeneric(msg, "name").ViaField("routeSpec").ViaField("hostname"))
	}

	clusterLocalDomain := network.GetClusterDomainName()
	if strings.HasSuffix(domainName, "."+clusterLocalDomain) {
		errs = errs.Also(apis.ErrGeneric(
			fmt.Sprintf("invalid name %q: must not be a subdomain of cluster local domain %q", domainName, clusterLocalDomain), "name"))
	}
	return errs
}

// IsDomainNameTaken checks if the given hostname is already in use.
func IsDomainNameTaken(ctx context.Context, domainName string) (bool, error) {
	_, err := net.DefaultResolver.LookupHost(ctx, domainName)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetCappConfig returns an instance of Capp Config.
func GetCappConfig(ctx context.Context, k8sClient client.Client) (*cappv1alpha1.CappConfig, error) {
	config := cappv1alpha1.CappConfig{}
	key := types.NamespacedName{Name: utils.CappConfigName, Namespace: utils.CappNS}
	if err := k8sClient.Get(ctx, key, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// ValidateURI checks if the provided URI is valid and is a relative path.
func ValidateURI(uri *apis.URL) error {

	_, err := apis.ParseURL(uri.String())
	if err != nil {
		return err
	}
	return nil
}
