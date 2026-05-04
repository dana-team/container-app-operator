package common

import (
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: `.*\.example\.com`}},
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
			domainName:      "myapp.other.com",
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: `.*\.example\.com`}},
			wantErr:         true,
			errContains:     "must match one of the allowed patterns",
		},
		{
			name:            "Empty allowed patterns (deny all)",
			domainName:      "myapp.example.com",
			allowedPatterns: []cappv1alpha1.HostnamePattern{},
			wantErr:         true,
			errContains:     "must match one of the allowed patterns",
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
			errContains:     "must match one of the allowed patterns",
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
			domainName:      "myapp.other.com",
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: `.*\.example\.com`, Explanation: "subdomains of example.com only"}},
			wantErr:         true,
			errContains:     "subdomains of example.com only",
		},
		{
			name:            "Raw pattern shown when explanation absent",
			domainName:      "myapp.other.com",
			allowedPatterns: []cappv1alpha1.HostnamePattern{{Match: `.*\.example\.com`}},
			wantErr:         true,
			errContains:     `.*\.example\.com`,
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
		name    string
		logSpec cappv1alpha1.LogSpec
		wantErr bool
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
			} else {
				require.Nil(t, err)
			}
		})
	}
}
