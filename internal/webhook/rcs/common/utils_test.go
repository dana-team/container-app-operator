package common

import (
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
)

const (
	allowedHostnamePattern      = `.*\.example\.com`
	nonMatchingHostname         = "myapp.other.com"
	errMustMatchAllowedPatterns = "must match one of the allowed patterns"
)

func TestValidateDomainName(t *testing.T) {
	tests := []struct {
		name            string
		domainName      string
		allowedPatterns []cappv1alpha1.HostnamePattern
		wantErr         bool
		errContains     string
	}{
		{
			name:            "Valid domain matching specific pattern",
			domainName:      "myapp.example.com",
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: allowedHostnamePattern}},
			wantErr:         false,
		},
		{
			name:            "Valid domain matching wild card",
			domainName:      "myapp.any.com",
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: `.*`}},
			wantErr:         false,
		},
		{
			name:            "Invalid domain not matching pattern",
			domainName:      nonMatchingHostname,
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: allowedHostnamePattern}},
			wantErr:         true,
			errContains:     errMustMatchAllowedPatterns,
		},
		{
			name:            "Empty allowed patterns (deny all)",
			domainName:      "myapp.example.com",
			allowedPatterns: []cappv1alpha1.HostnamePattern{},
			wantErr:         true,
			errContains:     errMustMatchAllowedPatterns,
		},
		{
			name:            "Multiple patterns, one match",
			domainName:      "myapp.org",
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: `.*\.com`}, {Match: `.*\.org`}},
			wantErr:         false,
		},
		{
			name:            "Multiple patterns, no match",
			domainName:      "myapp.net",
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: `.*\.com`}, {Match: `.*\.org`}},
			wantErr:         true,
			errContains:     errMustMatchAllowedPatterns,
		},
		{
			name:            "Invalid FQDN syntax",
			domainName:      "-invalid-start",
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: `.*`}},
			wantErr:         true,
		},
		{
			name:            "Invalid hostname with leading dots rejected as FQDN",
			domainName:      "...aaa.a....",
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: `.*`}},
			wantErr:         true,
		},
		{
			name:            "Invalid hostname with underscore rejected as FQDN under wildcard patterns",
			domainName:      "invalid_domain!",
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: `.*`}},
			wantErr:         true,
		},
		{
			name:            "Explanation appears in error message",
			domainName:      nonMatchingHostname,
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: allowedHostnamePattern, Explanation: "subdomains of example.com only"}},
			wantErr:         true,
			errContains:     "subdomains of example.com only",
		},
		{
			name:            "Raw pattern shown when explanation absent",
			domainName:      nonMatchingHostname,
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: allowedHostnamePattern}},
			wantErr:         true,
			errContains:     allowedHostnamePattern,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateDomainName(tt.domainName, tt.allowedPatterns)
			if tt.wantErr {
				assert.NotNil(t, errs)
				if tt.errContains != "" {
					assert.Contains(t, errs.Error(), tt.errContains)
				}
			} else {
				assert.Nil(t, errs)
			}
		})
	}
}
