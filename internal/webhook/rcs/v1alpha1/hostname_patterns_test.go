package webhooks

import (
	"context"
	"sync"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	cappmeta "github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestCompileHostnamePatterns(t *testing.T) {
	compiled := compileHostnamePatterns([]cappv1alpha1.HostnamePattern{
		{Match: allowedHostnamePattern, Explanation: "example"},
		{Match: `(`},
	})
	require.Len(t, compiled, 2)

	assert.NoError(t, compiled[0].err)
	require.NotNil(t, compiled[0].re)
	assert.Equal(t, `^(?:`+allowedHostnamePattern+`)$`, compiled[0].re.String())
	assert.Equal(t, "example", compiled[0].pattern.Explanation)

	assert.Error(t, compiled[1].err)
	assert.Nil(t, compiled[1].re)
	assert.Equal(t, `(`, compiled[1].pattern.Match)
}

func TestHostnamePatternCacheReusesCompiledPatterns(t *testing.T) {
	var cache hostnamePatternCache
	patterns := []cappv1alpha1.HostnamePattern{{Match: allowedHostnamePattern}, {Match: orgPattern}}

	first := cache.get(patterns)
	require.Len(t, first, 2)

	// An equal pattern list (a fresh copy, as a new CappConfig read would return) must hit the cache.
	second := cache.get([]cappv1alpha1.HostnamePattern{{Match: allowedHostnamePattern}, {Match: orgPattern}})
	assert.Same(t, &first[0], &second[0], "expected cached compiled patterns to be reused")
	assert.Same(t, first[0].re, second[0].re)
	assert.Same(t, first[1].re, second[1].re)
}

func TestHostnamePatternCacheRecompilesOnChange(t *testing.T) {
	tests := []struct {
		name    string
		initial []cappv1alpha1.HostnamePattern
		updated []cappv1alpha1.HostnamePattern
	}{
		{
			name:    "match changed",
			initial: []cappv1alpha1.HostnamePattern{{Match: comPattern}},
			updated: []cappv1alpha1.HostnamePattern{{Match: orgPattern}},
		},
		{
			name:    "explanation changed",
			initial: []cappv1alpha1.HostnamePattern{{Match: comPattern, Explanation: "old"}},
			updated: []cappv1alpha1.HostnamePattern{{Match: comPattern, Explanation: "new"}},
		},
		{
			name:    "pattern added",
			initial: []cappv1alpha1.HostnamePattern{{Match: comPattern}},
			updated: []cappv1alpha1.HostnamePattern{{Match: comPattern}, {Match: orgPattern}},
		},
		{
			name:    "all patterns removed",
			initial: []cappv1alpha1.HostnamePattern{{Match: comPattern}},
			updated: []cappv1alpha1.HostnamePattern{},
		},
		{
			name:    "patterns reordered",
			initial: []cappv1alpha1.HostnamePattern{{Match: comPattern}, {Match: orgPattern}},
			updated: []cappv1alpha1.HostnamePattern{{Match: orgPattern}, {Match: comPattern}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cache hostnamePatternCache
			cache.get(tt.initial)

			got := cache.get(tt.updated)
			require.Len(t, got, len(tt.updated))
			for i, cp := range got {
				assert.Equal(t, tt.updated[i], cp.pattern)
			}
		})
	}
}

func TestHostnamePatternCacheIsNotAliasedToCaller(t *testing.T) {
	var cache hostnamePatternCache
	patterns := []cappv1alpha1.HostnamePattern{{Match: comPattern}}
	first := cache.get(patterns)

	// Mutating the caller's slice in place must not make the cache think it is still up to date.
	patterns[0].Match = orgPattern
	got := cache.get(patterns)

	require.Len(t, got, 1)
	assert.Equal(t, orgPattern, got[0].pattern.Match)
	assert.NotSame(t, first[0].re, got[0].re)
}

func TestHostnamePatternCacheConcurrentAccess(t *testing.T) {
	var cache hostnamePatternCache
	sets := [][]cappv1alpha1.HostnamePattern{
		{{Match: comPattern}},
		{{Match: orgPattern}, {Match: `.*\.net`}},
	}

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(set []cappv1alpha1.HostnamePattern) {
			defer wg.Done()
			for range 100 {
				got := cache.get(set)
				if assert.Len(t, got, len(set)) {
					for j, cp := range got {
						assert.Equal(t, set[j], cp.pattern)
					}
				}
			}
		}(sets[i%len(sets)])
	}
	wg.Wait()
}

// TestCappValidatorPicksUpCappConfigPatternChanges checks the cache end to end: after the CappConfig
// patterns are updated, the next admission request is validated against the new patterns.
func TestCappValidatorPicksUpCappConfigPatternChanges(t *testing.T) {
	ctx := context.Background()
	validator := newCappValidator(t, newScheme(t), admission.NewDecoder(newScheme(t)))

	newHostnameCapp := func(hostname string) cappv1alpha1.Capp {
		return *withSkipDNSCheck(newCapp(hostname))
	}
	setPatterns := func(patterns []cappv1alpha1.HostnamePattern) {
		t.Helper()
		config := &cappv1alpha1.CappConfig{}
		require.NoError(t, validator.Client.Get(ctx, client.ObjectKey{Name: cappmeta.CappConfigName, Namespace: cappmeta.CappNS}, config))
		config.Spec.AllowedHostnamePatterns = patterns
		require.NoError(t, validator.Client.Update(ctx, config))
	}

	setPatterns([]cappv1alpha1.HostnamePattern{{Match: `.*\.example\.com`}})
	resp := validator.handle(ctx, admissionv1.Create, newHostnameCapp(exampleHostname), nil)
	assert.True(t, resp.Allowed, resp.Result.Message)
	resp = validator.handle(ctx, admissionv1.Create, newHostnameCapp("myapp.other.com"), nil)
	assert.False(t, resp.Allowed)

	setPatterns([]cappv1alpha1.HostnamePattern{{Match: `.*\.other\.com`}})
	resp = validator.handle(ctx, admissionv1.Create, newHostnameCapp("myapp.other.com"), nil)
	assert.True(t, resp.Allowed, resp.Result.Message)
	resp = validator.handle(ctx, admissionv1.Create, newHostnameCapp(exampleHostname), nil)
	assert.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, errMustMatchAllowedPatterns)
}
