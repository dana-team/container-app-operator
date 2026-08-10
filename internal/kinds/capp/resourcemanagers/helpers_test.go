package resourcemanagers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	hostnameBareFixture = "my-app"
	hostnameFQDNFixture = "my-app.example.com"
	zoneDot             = "example.com."
	zoneNoDot           = "example.com"
)

func TestGenerateResourceName(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		suffix   string
		want     string
	}{
		{
			name:     "appends suffix with trailing dot stripped",
			hostname: hostnameBareFixture,
			suffix:   zoneDot,
			want:     hostnameFQDNFixture,
		},
		{
			name:     "appends suffix without trailing dot",
			hostname: hostnameBareFixture,
			suffix:   zoneNoDot,
			want:     hostnameFQDNFixture,
		},
		{
			name:     "no-op when hostname already ends with suffix",
			hostname: hostnameFQDNFixture,
			suffix:   zoneDot,
			want:     hostnameFQDNFixture,
		},
		{
			name:     "no-op when hostname already ends with suffix without dot",
			hostname: hostnameFQDNFixture,
			suffix:   zoneNoDot,
			want:     hostnameFQDNFixture,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateResourceName(tt.hostname, tt.suffix)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestGenerateRecordName(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		suffix   string
		want     string
	}{
		{
			name:     "strips suffix with trailing dot",
			hostname: hostnameFQDNFixture,
			suffix:   zoneDot,
			want:     hostnameBareFixture,
		},
		{
			name:     "strips suffix without trailing dot",
			hostname: hostnameFQDNFixture,
			suffix:   zoneNoDot,
			want:     hostnameBareFixture + ".",
		},
		{
			name:     "returns hostname unchanged when suffix not present",
			hostname: hostnameBareFixture,
			suffix:   zoneDot,
			want:     hostnameBareFixture,
		},
		{
			name:     "returns hostname unchanged when suffix without dot not present",
			hostname: hostnameBareFixture,
			suffix:   zoneNoDot,
			want:     hostnameBareFixture,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateRecordName(tt.hostname, tt.suffix)
			require.Equal(t, tt.want, got)
		})
	}
}
