package v1migrate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests pin the v1-specific normalize folds (secrets:/egress: sugar, the
// credentials.sources + network.serviceAuth/serviceDomains + environment.
// proxyManaged consolidation, the standalone oauth: block fold, and
// deriveServiceKey). They were relocated from the spec package in the Phase 6
// cutover, where the v1-aware normalize passes they exercise now live.

func TestNormalizeSecrets(t *testing.T) {
	t.Run("converts_to_credential_sources", func(t *testing.T) {
		s := specFile{
			Manifest: Manifest{Kind: KindMixin, SchemaVersion: "1", Name: "m"},
			Secrets:  []string{"ANTHROPIC_API_KEY", "GH_TOKEN"},
		}
		require.NoError(t, s.normalize(&warnings{}))
		// After normalize, the v1 LegacySources have been folded into
		// Credentials.List as v2 Credential entries.
		services := map[string]Credential{}
		for _, c := range s.Credentials.List {
			services[c.Service] = c
		}
		require.Contains(t, services, "anthropic")
		require.Contains(t, services, "github")
		require.True(t, services["anthropic"].Required)
		require.NotNil(t, services["anthropic"].ApiKey)
		require.Equal(t, "ANTHROPIC_API_KEY", services["anthropic"].ApiKey.Name)
	})

	t.Run("conflict_with_existing_source", func(t *testing.T) {
		s := specFile{
			Manifest: Manifest{Kind: KindMixin, SchemaVersion: "1", Name: "m"},
			Secrets:  []string{"ANTHROPIC_API_KEY"},
			Credentials: credentialsField{
				LegacySources: map[string]CredentialSource{
					"anthropic": {Env: []string{"EXISTING"}},
				},
			},
		}
		require.ErrorContains(t, s.normalize(&warnings{}), "conflicts")
	})
}

func TestNormalizeEgress(t *testing.T) {
	t.Run("converts_to_network_policy", func(t *testing.T) {
		s := specFile{
			Manifest: Manifest{Kind: KindMixin, SchemaVersion: "1", Name: "m"},
			Egress:   map[string]string{"api.anthropic.com": "anthropic"},
		}
		require.NoError(t, s.normalize(&warnings{}))
		// Egress folds into LegacyNetwork.ServiceDomains+ServiceAuth (v1
		// intermediate); normalizeLegacyCredentials then folds that into
		// Credentials.List. Assert against the final canonical shape.
		require.Len(t, s.Credentials.List, 1)
		c := s.Credentials.List[0]
		require.Equal(t, "anthropic", c.Service)
		require.NotNil(t, c.ApiKey)
		require.Len(t, c.ApiKey.Inject, 1)
		require.Equal(t, "api.anthropic.com", c.ApiKey.Inject[0].Domain)
		require.Equal(t, "x-api-key", c.ApiKey.Inject[0].Header)
	})

	t.Run("unknown_service_gets_no_default_auth", func(t *testing.T) {
		s := specFile{
			Manifest: Manifest{Kind: KindMixin, SchemaVersion: "1", Name: "m"},
			Egress:   map[string]string{"custom.example.com": "custom"},
		}
		require.NoError(t, s.normalize(&warnings{}))
		// Service "custom" has no wellKnownAuth entry, so its inject
		// entry has no header. The Credential exists with apiKey.Inject
		// but no Header value.
		require.Len(t, s.Credentials.List, 1)
		c := s.Credentials.List[0]
		require.Equal(t, "custom", c.Service)
		require.NotNil(t, c.ApiKey)
		require.Len(t, c.ApiKey.Inject, 1)
		require.Empty(t, c.ApiKey.Inject[0].Header)
	})
}

func TestNormalize_OAuthOnlyService_DomainsMoveToResourceHosts(t *testing.T) {
	art, err := loadFromBytes([]byte(`
schemaVersion: "1"
kind: agent
name: vertex-test
agent:
  image: x
network:
  serviceDomains:
    aiplatform.googleapis.com: vertex
    europe-west4-aiplatform.googleapis.com: vertex
    oauth2.googleapis.com: vertex
oauth:
  service: vertex
  tokenEndpoint:
    host: oauth2.googleapis.com
    path: /token
  sentinels:
    accessToken: a-sentinel
    refreshToken: r-sentinel
`))
	require.NoError(t, err)

	var vertex *Credential
	for i := range art.Credentials {
		if art.Credentials[i].Service == "vertex" {
			vertex = &art.Credentials[i]
		}
	}
	require.NotNil(t, vertex, "vertex credential should exist")
	require.Nil(t, vertex.ApiKey, "degenerate apiKey must be dropped for an oauth-only service")
	require.NotNil(t, vertex.OAuth)
	require.Equal(t, []string{"aiplatform.googleapis.com", "europe-west4-aiplatform.googleapis.com"}, vertex.OAuth.ResourceHosts,
		"resource hosts moved + sorted; token endpoint NOT included")
	require.Equal(t, "oauth2.googleapis.com", vertex.OAuth.TokenEndpoint.Host)
}

func TestNormalize_BothMechanisms_KeepsApiKey(t *testing.T) {
	art, err := loadFromBytes([]byte(`
schemaVersion: "1"
kind: agent
name: anthropic-test
agent:
  image: x
credentials:
  sources:
    anthropic: {env: [ANTHROPIC_API_KEY]}
network:
  serviceDomains:
    api.anthropic.com: anthropic
  serviceAuth:
    anthropic: {headerName: x-api-key, valueFormat: "%s"}
oauth:
  service: anthropic
  tokenEndpoint: {host: platform.claude.com, path: /v1/oauth/token}
  sentinels: {accessToken: a, refreshToken: r}
`))
	require.NoError(t, err)
	var c *Credential
	for i := range art.Credentials {
		if art.Credentials[i].Service == "anthropic" {
			c = &art.Credentials[i]
		}
	}
	require.NotNil(t, c)
	require.NotNil(t, c.ApiKey, "real apiKey (has env name + header) must be kept")
	require.Equal(t, "ANTHROPIC_API_KEY", c.ApiKey.Name)
	require.NotNil(t, c.OAuth)
	require.Empty(t, c.OAuth.ResourceHosts, "shared hosts stay in apiKey.inject; not duplicated")
}

// TestNormalize_OAuthMerge_SortsAndDedupes exercises the else-branch in
// normalizeLegacyOAuthBlock where c.OAuth != nil. It verifies that when a
// Credential already has an OAuth block with resourceHosts PLUS a routing-only
// apiKey, and a v1 standalone oauth: block is folded in, the domains from the
// apiKey are moved to resourceHosts and the final list is sorted and deduped.
func TestNormalize_OAuthMerge_SortsAndDedupes(t *testing.T) {
	art, err := loadFromBytes([]byte(`
schemaVersion: "1"
kind: agent
name: merge-test
agent:
  image: x
# v2 credential with existing OAuth resourceHosts AND a routing-only apiKey
# (no name, no header on inject entries = routing only).
credentials:
  - service: vertex
    apiKey:
      inject:
        - domain: new.googleapis.com
        - domain: aaa-first.googleapis.com
        - domain: existing.googleapis.com
    oauth:
      resourceHosts:
        - existing.googleapis.com
        - zzz-last.googleapis.com
      tokenEndpoint:
        host: oauth2.googleapis.com
        path: /token
      sentinels:
        accessToken: a
        refreshToken: r
# v1 standalone oauth block for same service triggers the merge branch
oauth:
  service: vertex
  tokenEndpoint:
    host: oauth2.googleapis.com
    path: /token
  sentinels:
    accessToken: a-sentinel
    refreshToken: r-sentinel
`))
	require.NoError(t, err)

	var vertex *Credential
	for i := range art.Credentials {
		if art.Credentials[i].Service == "vertex" {
			vertex = &art.Credentials[i]
		}
	}
	require.NotNil(t, vertex, "vertex credential should exist")
	require.Nil(t, vertex.ApiKey, "routing-only apiKey must be dropped after merge")
	require.NotNil(t, vertex.OAuth)
	// Expect sorted + deduped: aaa-first, existing (deduped), new, zzz-last
	// The token endpoint (oauth2.googleapis.com) is excluded from the move.
	require.Equal(t,
		[]string{"aaa-first.googleapis.com", "existing.googleapis.com", "new.googleapis.com", "zzz-last.googleapis.com"},
		vertex.OAuth.ResourceHosts,
		"merged resourceHosts should be sorted and deduped")
}

func TestNormalizeFoldsProxyManagedToMarker(t *testing.T) {
	a, err := loadFromBytes([]byte(`schemaVersion: "1"
kind: agent
name: t
agent: {image: x}
network:
  serviceDomains: {generativelanguage.googleapis.com: google, api.github.com: github}
  serviceAuth:
    google: {headerName: x-goog-api-key, valueFormat: "%s"}
    github: {headerName: Authorization, valueFormat: "Bearer %s"}
credentials:
  sources:
    google: {env: [GOOGLE_API_KEY, GEMINI_API_KEY]}
    github: {env: [GH_TOKEN]}
environment:
  proxyManaged: [GOOGLE_API_KEY, GEMINI_API_KEY]
`))
	require.NoError(t, err)
	bySvc := map[string]*ApiKey{}
	for i := range a.Credentials {
		bySvc[a.Credentials[i].Service] = a.Credentials[i].ApiKey
	}
	require.NotNil(t, bySvc["google"])
	require.Equal(t, "GOOGLE_API_KEY", bySvc["google"].Name) // primary; GEMINI not preserved
	require.True(t, bySvc["google"].ProxyManaged)            // marked
	require.NotNil(t, bySvc["github"])
	require.False(t, bySvc["github"].ProxyManaged) // not in proxyManaged → unmarked

	// Environment.ProxyManaged is DERIVED from the marked credentials: only
	// google's canonical GOOGLE_API_KEY (GEMINI secondary dropped, github
	// unmarked). This is the in-container sentinel set the engine reads.
	require.NotNil(t, a.Environment)
	require.Equal(t, []string{"GOOGLE_API_KEY"}, a.Environment.ProxyManaged)
}

func TestDeriveServiceKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ANTHROPIC_API_KEY", "anthropic"},
		{"OPENAI_API_KEY", "openai"},
		{"GH_TOKEN", "github"},
		{"GITHUB_TOKEN", "github"},
		{"SOME_SECRET", "some"},
		{"PLAIN_NAME", "plain_name"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			require.Equal(t, tc.expected, deriveServiceKey(tc.input))
		})
	}
}
