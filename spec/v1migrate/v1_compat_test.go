package v1migrate

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests pin the v1 → v2 fold behavior of v1migrate's own v1-aware loader
// (loadFromBytes). They were relocated from the spec package in the Phase 6
// cutover: the production spec loader is v2-only, so the v1 acceptance +
// deprecation-fold contract now lives here, alongside the loader that
// implements it. The whole package is a clean delete once v1 is gone.

// hasWarning reports whether any warning on the artifact contains all of subs.
func hasWarning(art *Artifact, subs ...string) bool {
	for _, w := range art.Warnings {
		all := true
		for _, s := range subs {
			if !strings.Contains(w, s) {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

func TestV1Memory_MapsToAgentContext(t *testing.T) {
	art, err := loadFromBytes([]byte(`schemaVersion: "1"
kind: agent
name: test-agent
agent:
  image: example/test:latest
memory: |
  Legacy v1 memory content
`))
	require.NoError(t, err)
	require.Contains(t, art.AgentContext, "Legacy v1 memory content")
	require.True(t, hasWarning(art, "memory", "agentContext"),
		"expected deprecation warning for memory→agentContext, got %v", art.Warnings)
}

func TestV2AgentContext_WinsOverV1Memory(t *testing.T) {
	art, err := loadFromBytes([]byte(`schemaVersion: "2"
kind: agent
name: test-agent
agent:
  image: example/test:latest
memory: "v1 content"
agentContext: "v2 content"
`))
	require.NoError(t, err)
	require.Equal(t, "v2 content", art.AgentContext, "v2 agentContext wins on conflict")
}

func TestV1Kind_Agent_MapsToSandbox(t *testing.T) {
	art, err := loadFromBytes([]byte(`schemaVersion: "1"
kind: agent
name: test
agent:
  image: example/test:latest
`))
	require.NoError(t, err)
	require.Equal(t, KindSandbox, art.Manifest.Kind, "v1 'agent' normalized to v2 'sandbox'")
	require.True(t, hasWarning(art, "kind", "agent"),
		"expected deprecation warning for kind: agent, got %v", art.Warnings)
}

func TestV1AgentBlock_MapsToSandbox(t *testing.T) {
	art, err := loadFromBytes([]byte(`schemaVersion: "1"
kind: agent
name: test
agent:
  image: example/test:latest
  aiFilename: TEST.md
`))
	require.NoError(t, err)
	require.Equal(t, "example/test:latest", art.Manifest.Template)
	require.True(t, hasWarning(art, "agent:") || hasWarning(art, "agent", "sandbox"),
		"expected deprecation warning for agent: block, got %v", art.Warnings)
}

// TestV1Persistence_Deprecated pins the persistence:-as-deprecation-warning
// shim. The field was a no-op pre-v0.31 (parsed but never consumed); after
// the strict-decode flip in PR #37 it became a hard error with no migration
// path. This shim re-admits it as a deprecation warning, matching the
// pattern established for memory:/kind:agent/agent: block.
func TestV1Persistence_Deprecated(t *testing.T) {
	art, err := loadFromBytes([]byte(`schemaVersion: "1"
kind: agent
name: test
agent:
  image: example/test:latest
persistence: persistent
`))
	require.NoError(t, err)
	require.True(t, hasWarning(art, "persistence"),
		"expected deprecation warning for persistence, got %v", art.Warnings)
}

// TestV1NestedAgentPersistence_Deprecated is the shape that actually shipped
// in real-world kits — `persistence:` indented under the v1 `agent:` block
// (now `sandboxBlock`). Andre's copilot-dotnet kit reported in
// docker/sbx-releases#191 used this exact form. The shim folds the nested
// form through normalizeSandbox with a deprecation warning.
func TestV1NestedAgentPersistence_Deprecated(t *testing.T) {
	art, err := loadFromBytes([]byte(`schemaVersion: "1"
kind: agent
name: test
agent:
  image: example/test:latest
  persistence: persistent
`))
	require.NoError(t, err)
	require.True(t, hasWarning(art, "sandbox.persistence"),
		"expected deprecation warning for sandbox.persistence, got %v", art.Warnings)
}

// TestV1KitDir_Deprecated is the analogue of TestV1Persistence_Deprecated
// for the v1 `kitDir:` field. Same retire-then-strict story; same shim.
func TestV1KitDir_Deprecated(t *testing.T) {
	art, err := loadFromBytes([]byte(`schemaVersion: "1"
kind: agent
name: test
agent:
  image: example/test:latest
kitDir: /opt/whatever
`))
	require.NoError(t, err)
	require.True(t, hasWarning(art, "kitDir"),
		"expected deprecation warning for kitDir, got %v", art.Warnings)
}

// TestV1VolumesMap_Deprecated pins the v1 `volumes:` mapping shim.
// Pre-PR #37, `volumes:` was `map[string]string` from container path to
// (sometimes-empty) size string. The polymorphic volumesField wrapper
// accepts this shape and normalize folds it into Manifest.Volumes as
// MountSpec entries with Type left at its zero value (block-backed,
// matching the v1 default), emitting a deprecation warning.
func TestV1VolumesMap_Deprecated(t *testing.T) {
	art, err := loadFromBytes([]byte(`schemaVersion: "1"
kind: sandbox
name: legacy-volumes
sandbox:
  image: docker/sandbox-templates:shell-docker
volumes:
  /var/lib/docker: ""
  /opt/data: 4g
`))
	require.NoError(t, err)

	// Sorted-by-path: /opt/data first, /var/lib/docker second. /var/lib/docker
	// had no size in the v1 spec, so the Size field stays empty.
	require.Len(t, art.Manifest.Volumes, 2)
	require.Equal(t, []MountSpec{
		{Path: "/opt/data", Size: "4g"},
		{Path: "/var/lib/docker"},
	}, art.Manifest.Volumes)
	require.True(t, hasWarning(art, "volumes"),
		"expected deprecation warning for volumes mapping, got %v", art.Warnings)
}

// TestV1TmpfsMap_Deprecated pins the v1 `tmpfs:` mapping shim. Pre-PR #37
// the shape was a mapping from container path to size string (e.g.
// `{ /tmp/scratch: "512m" }`). The decoder now folds it into the canonical
// v2 Volumes list with Type=tmpfs and emits a deprecation warning.
func TestV1TmpfsMap_Deprecated(t *testing.T) {
	art, err := loadFromBytes([]byte(`schemaVersion: "1"
kind: sandbox
name: legacy-tmpfs
sandbox:
  image: docker/sandbox-templates:shell-docker
tmpfs:
  /tmp/scratch: 512m
  /var/cache: 256m
`))
	require.NoError(t, err)

	// Both entries should land on Volumes with Type=tmpfs, sorted by path.
	require.Len(t, art.Manifest.Volumes, 2)
	require.Equal(t, []MountSpec{
		{Path: "/tmp/scratch", Type: MountTypeTmpfs, Size: "512m"},
		{Path: "/var/cache", Type: MountTypeTmpfs, Size: "256m"},
	}, art.Manifest.Volumes)
	require.True(t, hasWarning(art, "tmpfs"),
		"expected deprecation warning for tmpfs, got %v", art.Warnings)
}

// TestV1Credentials_FullShape_RoundTripsToList exercises the parallel-load
// contract for the credentials redesign: a realistic v1 spec.yaml using
// credentials.sources + network.serviceAuth + network.serviceDomains +
// environment.proxyManaged loads cleanly, normalizes to a single v2
// credentials[] entry per service, and produces one deprecation warning
// per legacy block touched.
func TestV1Credentials_FullShape_RoundTripsToList(t *testing.T) {
	art, err := loadFromBytes([]byte(`schemaVersion: "1"
kind: sandbox
name: creds-v1
sandbox:
  image: docker/sandbox-templates:shell-docker
credentials:
  sources:
    anthropic:
      env: [ANTHROPIC_API_KEY]
      required: true
network:
  serviceDomains:
    api.anthropic.com: anthropic
  serviceAuth:
    anthropic:
      headerName: x-api-key
      valueFormat: "%s"
environment:
  proxyManaged: [ANTHROPIC_API_KEY]
`))
	require.NoError(t, err)

	// The four v1 surfaces fold into one v2 credentials[] entry.
	require.Len(t, art.Credentials, 1)
	c := art.Credentials[0]
	require.Equal(t, "anthropic", c.Service)
	require.True(t, c.Required)
	require.NotNil(t, c.ApiKey)
	require.Equal(t, "ANTHROPIC_API_KEY", c.ApiKey.Name)
	require.Len(t, c.ApiKey.Inject, 1)
	require.Equal(t, "api.anthropic.com", c.ApiKey.Inject[0].Domain)
	require.Equal(t, "x-api-key", c.ApiKey.Inject[0].Header)
	require.Equal(t, "%s", c.ApiKey.Inject[0].Format)

	// One deprecation warning per legacy block.
	require.True(t, hasWarning(art, "credentials.sources"), "expected credentials.sources warning, got %v", art.Warnings)
	require.True(t, hasWarning(art, "network.serviceAuth"), "expected network.serviceAuth warning, got %v", art.Warnings)
	require.True(t, hasWarning(art, "network.serviceDomains"), "expected network.serviceDomains warning, got %v", art.Warnings)
	require.True(t, hasWarning(art, "environment.proxyManaged"), "expected environment.proxyManaged warning, got %v", art.Warnings)
}

// TestV1OAuth_StandaloneBlock_RoundTripsToCredentials exercises the
// parallel-load contract for the standalone top-level oauth: block →
// credentials[].oauth migration.
func TestV1OAuth_StandaloneBlock_RoundTripsToCredentials(t *testing.T) {
	art, err := loadFromBytes([]byte(`schemaVersion: "1"
kind: sandbox
name: oauth-v1
sandbox:
  image: docker/sandbox-templates:shell-docker
oauth:
  service: openai
  tokenEndpoint: {host: auth.openai.com, path: /oauth/token}
  sentinels: {accessToken: oai-oat01-proxy-managed, refreshToken: oai-ort01-proxy-managed}
`))
	require.NoError(t, err)

	require.Len(t, art.Credentials, 1)
	c := art.Credentials[0]
	require.Equal(t, "openai", c.Service)
	require.NotNil(t, c.OAuth)
	require.Equal(t, "auth.openai.com", c.OAuth.TokenEndpoint.Host)
	require.Equal(t, "/oauth/token", c.OAuth.TokenEndpoint.Path)
	require.True(t, hasWarning(art, "oauth:", "standalone"),
		"expected standalone oauth: deprecation warning, got %v", art.Warnings)
}

// TestV1NetworkAllowedDomains_RoundTripsToCapsNetwork exercises the
// parallel-load contract for the network -> caps.network rename: a v1
// spec with network.allowedDomains / network.deniedDomains loads
// cleanly, normalizes to caps.network.allow / .deny, and produces one
// deprecation warning per legacy field.
func TestV1NetworkAllowedDomains_RoundTripsToCapsNetwork(t *testing.T) {
	art, err := loadFromBytes([]byte(`schemaVersion: "1"
kind: sandbox
name: net-v1
sandbox:
  image: docker/sandbox-templates:shell-docker
network:
  allowedDomains: [api.anthropic.com]
  deniedDomains: [malware.example.com]
`))
	require.NoError(t, err)

	require.NotNil(t, art.Caps)
	require.NotNil(t, art.Caps.Network)
	require.Contains(t, art.Caps.Network.Allow, "api.anthropic.com")
	require.ElementsMatch(t, []string{"malware.example.com"}, art.Caps.Network.Deny)
	require.True(t, hasWarning(art, "network.allowedDomains"), "expected network.allowedDomains warning, got %v", art.Warnings)
	require.True(t, hasWarning(art, "network.deniedDomains"), "expected network.deniedDomains warning, got %v", art.Warnings)
}

// TestV1Settings_AbsorbedWithDeprecationWarning pins the v1 `settings:` block
// behavior: it must PARSE (the LegacySettings shim absorbs it) and normalize
// must emit a deprecation warning naming `settings` rather than strict-reject.
// The canonical Artifact.Settings surface was removed in Phase 4; the block is
// absorbed-and-dropped here. Relocated from the spec package in Phase 6.
func TestV1Settings_AbsorbedWithDeprecationWarning(t *testing.T) {
	art, err := loadFromBytes([]byte(`schemaVersion: "1"
kind: mixin
name: settings-v1
settings:
  containerSettings:
    enableFeatureX: true
`))
	require.NoError(t, err, "v1 settings: must load (absorb-and-warn), not strict-reject")
	require.True(t, hasWarning(art, "settings"),
		"v1 settings: must emit a deprecation warning; got %v", art.Warnings)
}
