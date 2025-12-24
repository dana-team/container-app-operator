package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateDomainName(t *testing.T) {
	tests := []struct {
		name            string
		domainName      string
		allowedPatterns []string
		expectError     bool
		errorContains   string
	}{
		{
			name:            "Valid domain matching specific pattern",
			domainName:      "myapp.example.com",
			allowedPatterns: []string{`.*\.example\.com`},
			expectError:     false,
		},
		{
			name:            "Valid domain matching wild card",
			domainName:      "myapp.any.com",
			allowedPatterns: []string{`.*`},
			expectError:     false,
		},
		{
			name:            "Invalid domain not matching pattern",
			domainName:      "myapp.other.com",
			allowedPatterns: []string{`.*\.example\.com`},
			expectError:     true,
			errorContains:   "must match one of the allowed patterns",
		},
		{
			name:            "Empty allowed patterns (deny all)",
			domainName:      "myapp.example.com",
			allowedPatterns: []string{},
			expectError:     true,
			errorContains:   "must match one of the allowed patterns",
		},
		{
			name:            "Multiple patterns, one match",
			domainName:      "myapp.org",
			allowedPatterns: []string{`.*\.com`, `.*\.org`},
			expectError:     false,
		},
		{
			name:            "Multiple patterns, no match",
			domainName:      "myapp.net",
			allowedPatterns: []string{`.*\.com`, `.*\.org`},
			expectError:     true,
			errorContains:   "must match one of the allowed patterns",
		},
		{
			name:            "Invalid FQDN syntax",
			domainName:      "-invalid-start",
			allowedPatterns: []string{`.*`},
			expectError:     true,
			errorContains:   "invalid name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateDomainName(tt.domainName, tt.allowedPatterns)
			if tt.expectError {
				assert.NotNil(t, errs)
				if tt.errorContains != "" {
					assert.True(t, strings.Contains(errs.Error(), tt.errorContains), "Expected error to contain %q, got %q", tt.errorContains, errs.Error())
				}
			} else {
				assert.Nil(t, errs)
			}
		})
	}
}
