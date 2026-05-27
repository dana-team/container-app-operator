package common

import (
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestValidateLogSpec(t *testing.T) {
	const elasticLogHost = "https://elasticsearch.dana.com/_bulk"

	tests := []struct {
		name        string
		logSpec     cappv1alpha1.LogSpec
		wantErr     bool
		errContains []string
	}{
		{
			name: "denies when elastic log configuration omits required fields",
			logSpec: cappv1alpha1.LogSpec{
				Type: cappv1alpha1.LogTypeElastic,
				Host: elasticLogHost,
			},
			wantErr: true,
		},
		{
			name: "denies when log type is invalid",
			logSpec: cappv1alpha1.LogSpec{
				Type: cappv1alpha1.LogType("bogus"),
			},
			wantErr: true,
			errContains: []string{
				string(cappv1alpha1.LogTypeElastic),
				string(cappv1alpha1.LogTypeElasticDataStream),
			},
		},
		{
			name: "accepts when elastic log configuration includes all required fields",
			logSpec: cappv1alpha1.LogSpec{
				Type:           cappv1alpha1.LogTypeElastic,
				Host:           elasticLogHost,
				Index:          "main",
				User:           "user",
				PasswordSecret: "elastic-secret",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLogSpec(tt.logSpec)
			if tt.wantErr {
				require.NotNil(t, err)
				for _, sub := range tt.errContains {
					require.Contains(t, err.Error(), sub)
				}
			} else {
				require.Nil(t, err)
			}
		})
	}
}
