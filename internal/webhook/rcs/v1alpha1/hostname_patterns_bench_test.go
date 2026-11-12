package webhooks

import (
	"fmt"
	"strings"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
)

// hostnamePatternBenchCase is a CappConfig pattern list together with a hostname that only matches
// its last pattern, so validation has to walk the whole list (the worst case for an allowed hostname).
type hostnamePatternBenchCase struct {
	name     string
	patterns []cappv1alpha1.HostnamePattern
	hostname string
}

func hostnamePatternBenchCases() []hostnamePatternBenchCase {
	many := make([]cappv1alpha1.HostnamePattern, 0, 10)
	for i := range 9 {
		many = append(many, cappv1alpha1.HostnamePattern{Match: fmt.Sprintf(`.*\.team%d\.apps\.example\.com`, i)})
	}
	many = append(many, cappv1alpha1.HostnamePattern{Match: `.*\.internal\.example\.com`, Explanation: "internal apps"})

	return []hostnamePatternBenchCase{
		{
			name:     "1-pattern",
			patterns: []cappv1alpha1.HostnamePattern{{Match: ".*", Explanation: "any hostname"}},
			hostname: exampleHostname,
		},
		{
			// Same patterns as hack/manifests/cappconfig.yaml.
			name: "3-patterns",
			patterns: []cappv1alpha1.HostnamePattern{
				{Match: `.*\.allowed`, Explanation: "subdomains of .allowed domain"},
				{Match: comPattern, Explanation: "any .com domain"},
				{Match: `test.dev-.*`, Explanation: "test subdomains of dev"},
			},
			hostname: "test.dev-abc123",
		},
		{
			name:     "10-patterns",
			patterns: many,
			hostname: "myapp.internal.example.com",
		},
	}
}

// deepCopyPatterns returns a copy that shares no string memory with patterns, like the fresh
// CappConfig object each admission request reads from the informer cache.
func deepCopyPatterns(patterns []cappv1alpha1.HostnamePattern) []cappv1alpha1.HostnamePattern {
	out := make([]cappv1alpha1.HostnamePattern, len(patterns))
	for i, p := range patterns {
		out[i] = cappv1alpha1.HostnamePattern{Match: strings.Clone(p.Match), Explanation: strings.Clone(p.Explanation)}
	}
	return out
}

// BenchmarkHostnamePatternValidation compares the per-request cost of hostname pattern validation:
//   - compile-per-request: the previous behaviour, compiling every pattern on each admission request.
//   - cached: the current behaviour, reusing patterns compiled by hostnamePatternCache.
//
// Both paths use the same anchored compilation and validateDomainName, so the difference is the cache.
func BenchmarkHostnamePatternValidation(b *testing.B) {
	for _, bc := range hostnamePatternBenchCases() {
		b.Run(bc.name+"/compile-per-request", func(b *testing.B) {
			patterns := deepCopyPatterns(bc.patterns)
			b.ReportAllocs()
			for b.Loop() {
				if errs := validateDomainName(bc.hostname, compileHostnamePatterns(patterns)); errs != nil {
					b.Fatal(errs)
				}
			}
		})

		b.Run(bc.name+"/cached", func(b *testing.B) {
			var cache hostnamePatternCache
			cache.get(deepCopyPatterns(bc.patterns))
			patterns := deepCopyPatterns(bc.patterns)
			b.ReportAllocs()
			for b.Loop() {
				if errs := validateDomainName(bc.hostname, cache.get(patterns)); errs != nil {
					b.Fatal(errs)
				}
			}
		})
	}
}

// BenchmarkHostnamePatternValidationParallel runs the same comparison with concurrent requests,
// as the webhook server handles admission requests in parallel goroutines.
func BenchmarkHostnamePatternValidationParallel(b *testing.B) {
	bc := hostnamePatternBenchCases()[1]

	b.Run(bc.name+"/compile-per-request", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			patterns := deepCopyPatterns(bc.patterns)
			for pb.Next() {
				if errs := validateDomainName(bc.hostname, compileHostnamePatterns(patterns)); errs != nil {
					b.Error(errs)
				}
			}
		})
	})

	b.Run(bc.name+"/cached", func(b *testing.B) {
		var cache hostnamePatternCache
		cache.get(deepCopyPatterns(bc.patterns))
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			patterns := deepCopyPatterns(bc.patterns)
			for pb.Next() {
				if errs := validateDomainName(bc.hostname, cache.get(patterns)); errs != nil {
					b.Error(errs)
				}
			}
		})
	})
}
