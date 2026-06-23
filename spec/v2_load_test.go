package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// This file pins the v2 load contract for the production (v2-only) spec
// loader. The v1 → v2 fold cases (legacy `agent:`/`network:`/`oauth:`/
// `memory:`/`settings:`/`tmpfs:`/`secrets:`/`egress:` shapes, the v1
// `credentials.sources` and mapping-form `volumes:` shapes, and the
// `schemaVersion: "1"` acceptance) live in spec/v1migrate, where the v1-aware
// loader they exercise now lives. See spec/v1migrate/v1_compat_test.go.

func TestV2Kind_Sandbox_Accepted(t *testing.T) {
	dir := t.TempDir()
	specYAML := `schemaVersion: "2"
kind: sandbox
name: test
sandbox:
  image: example/test:latest
`
	if err := os.WriteFile(filepath.Join(dir, "spec.yaml"), []byte(specYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := LoadFromDirectory(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if art.Manifest.Kind != KindSandbox {
		t.Errorf("Kind = %q; want %q", art.Manifest.Kind, KindSandbox)
	}
	for _, w := range art.Warnings {
		if strings.Contains(w, "kind") {
			t.Errorf("unexpected deprecation warning for v2 kind: %s", w)
		}
	}
}

func TestV2SandboxBlock_Accepted(t *testing.T) {
	dir := t.TempDir()
	specYAML := `schemaVersion: "2"
kind: sandbox
name: test
sandbox:
  image: example/test:latest
  aiFilename: TEST.md
`
	if err := os.WriteFile(filepath.Join(dir, "spec.yaml"), []byte(specYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := LoadFromDirectory(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if art.Manifest.Template != "example/test:latest" {
		t.Errorf("Template = %q; want example/test:latest", art.Manifest.Template)
	}
	for _, w := range art.Warnings {
		if strings.Contains(w, "agent:") {
			t.Errorf("unexpected deprecation warning for v2 sandbox block: %s", w)
		}
	}
}

func TestVolumesType_TmpfsAccepted(t *testing.T) {
	dir := t.TempDir()
	specYAML := `schemaVersion: "2"
kind: sandbox
name: vol-tmpfs
sandbox:
  image: docker/sandbox-templates:shell-docker
volumes:
  - path: /tmp/scratch
    type: tmpfs
    size: 512m
    mode: "1777"
`
	if err := os.WriteFile(filepath.Join(dir, "spec.yaml"), []byte(specYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := LoadFromDirectory(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(art.Manifest.Volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(art.Manifest.Volumes))
	}
	v := art.Manifest.Volumes[0]
	if v.Type != "tmpfs" || v.Path != "/tmp/scratch" || v.Size != "512m" || v.Mode != "1777" {
		t.Errorf("volume mismatch: %#v", v)
	}
}

// TestV2VolumesList_Accepted is the canonical v2 sequence shape: it must
// round-trip into Manifest.Volumes with no deprecation warning.
func TestV2VolumesList_Accepted(t *testing.T) {
	dir := t.TempDir()
	specYAML := `schemaVersion: "2"
kind: sandbox
name: v2-volumes
sandbox:
  image: docker/sandbox-templates:shell-docker
volumes:
  - path: /opt/data
    size: 4g
  - path: /var/lib/docker
`
	if err := os.WriteFile(filepath.Join(dir, "spec.yaml"), []byte(specYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := LoadFromDirectory(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(art.Manifest.Volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(art.Manifest.Volumes))
	}
	if art.Manifest.Volumes[0].Path != "/opt/data" || art.Manifest.Volumes[0].Size != "4g" {
		t.Errorf("volume[0] = %#v; want path=/opt/data size=4g", art.Manifest.Volumes[0])
	}
	for _, w := range art.Warnings {
		if strings.Contains(w, "volumes") {
			t.Errorf("unexpected deprecation warning for v2 volumes list: %s", w)
		}
	}
}

// TestV2Credentials_ListShape exercises the minimal shape — one credential
// with apiKey only. Catches "did the new types decode?" regressions
// without obscuring them under a wall of fields.
func TestV2Credentials_ListShape(t *testing.T) {
	dir := t.TempDir()
	specYAML := `schemaVersion: "2"
kind: sandbox
name: creds-minimal
sandbox:
  image: docker/sandbox-templates:shell-docker
credentials:
  - service: anthropic
    description: "Anthropic API"
    required: true
    apiKey:
      name: ANTHROPIC_API_KEY
      inject:
        - domain: api.anthropic.com
          header: x-api-key
          format: "%s"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "spec.yaml"), []byte(specYAML), 0o644))
	art, err := LoadFromDirectory(dir)
	require.NoError(t, err)
	require.Len(t, art.Credentials, 1)
	c := art.Credentials[0]
	require.Equal(t, "anthropic", c.Service)
	require.True(t, c.Required)
	require.NotNil(t, c.ApiKey)
	require.Equal(t, "ANTHROPIC_API_KEY", c.ApiKey.Name)
	require.Len(t, c.ApiKey.Inject, 1)
	require.Equal(t, "api.anthropic.com", c.ApiKey.Inject[0].Domain)
}

// TestV2Credentials_FullShape exercises every field documented in the
// RFC: apiKey with username (HTTP basic auth for git HTTPS), full oauth
// shape (credentialFile.structure, responseFields, skipIfEnv,
// passthrough+passthroughReason). Schema regressions surface here first.
func TestV2Credentials_FullShape(t *testing.T) {
	dir := t.TempDir()
	specYAML := `schemaVersion: "2"
kind: sandbox
name: creds-full
sandbox:
  image: docker/sandbox-templates:shell-docker
credentials:
  - service: anthropic
    description: "Anthropic API credentials"
    required: true
    apiKey:
      name: ANTHROPIC_API_KEY
      inject:
        - domain: api.anthropic.com
          header: x-api-key
          format: "%s"
    oauth:
      tokenEndpoint:
        host: platform.claude.com
        path: /v1/oauth/token
      sentinels:
        accessToken: sk-ant-oat01-proxy-managed
        refreshToken: sk-ant-ort01-proxy-managed
      credentialFile:
        path: "~/.claude/.credentials.json"
        structure:
          claudeAiOauth:
            accessToken: "{{.AccessToken}}"
            refreshToken: "{{.RefreshToken}}"
            expiresAt: "{{.ExpiresAt}}"
      responseFields:
        accessToken: access_token
        refreshToken: refresh_token
        expiresIn: expires_in
        scope: scope
      skipIfEnv: [ANTHROPIC_API_KEY]

  - service: github
    description: "GitHub credential for git+API access"
    apiKey:
      name: GITHUB_TOKEN
      inject:
        - domain: api.github.com
          header: Authorization
          format: "Bearer %s"
        - domain: github.com
          header: Authorization
          format: "%s"
          username: x-access-token

  - service: workos
    oauth:
      tokenEndpoint:
        host: api.workos.com
        path: /oauth/token
      sentinels: {}
      passthrough: true
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "spec.yaml"), []byte(specYAML), 0o644))
	art, err := LoadFromDirectory(dir)
	require.NoError(t, err)
	require.Len(t, art.Credentials, 3)

	// anthropic — apiKey + full oauth.
	a := art.Credentials[0]
	require.Equal(t, "anthropic", a.Service)
	require.Equal(t, "Anthropic API credentials", a.Description)
	require.True(t, a.Required)
	require.NotNil(t, a.ApiKey)
	require.Equal(t, "ANTHROPIC_API_KEY", a.ApiKey.Name)
	require.NotNil(t, a.OAuth)
	require.Equal(t, "platform.claude.com", a.OAuth.TokenEndpoint.Host)
	require.Equal(t, "/v1/oauth/token", a.OAuth.TokenEndpoint.Path)
	require.Equal(t, "sk-ant-oat01-proxy-managed", a.OAuth.Sentinels.AccessToken)
	require.Equal(t, "sk-ant-ort01-proxy-managed", a.OAuth.Sentinels.RefreshToken)
	require.NotNil(t, a.OAuth.CredentialFile)
	require.Equal(t, "~/.claude/.credentials.json", a.OAuth.CredentialFile.Path)
	require.NotNil(t, a.OAuth.CredentialFile.Structure)
	require.NotNil(t, a.OAuth.ResponseFields)
	require.Equal(t, "access_token", a.OAuth.ResponseFields.AccessToken)
	require.Equal(t, []string{"ANTHROPIC_API_KEY"}, a.OAuth.SkipIfEnv)
	require.False(t, a.OAuth.Passthrough)

	// github — basic auth with username for git HTTPS.
	g := art.Credentials[1]
	require.Equal(t, "github", g.Service)
	require.Len(t, g.ApiKey.Inject, 2)
	require.Equal(t, "x-access-token", g.ApiKey.Inject[1].Username)

	// workos — passthrough OAuth (passthroughReason is deliberately
	// deferred to a later release; this assertion will gain a reason
	// check if/when that field lands).
	w := art.Credentials[2]
	require.True(t, w.OAuth.Passthrough)
}

// TestV1Rejected pins the Phase 6 cutover: the production loader is v2-only.
// A v1 spec is rejected two ways — its v1-only keys hard-fail strict decode,
// and a spec that is otherwise v2-shaped but declares schemaVersion "1" fails
// validation. Authors convert with spec/v1migrate (scripts/migrate-v1-to-v2).
func TestV1Rejected(t *testing.T) {
	t.Run("v1_only_keys_rejected_at_decode", func(t *testing.T) {
		// `kind: agent`, the `agent:`/`network:`/`memory:` blocks are no
		// longer known fields; strict decode rejects them.
		_, err := LoadFromBytes([]byte(`schemaVersion: "1"
kind: agent
name: legacy
agent:
  image: example/test:latest
network:
  allowedDomains: [example.com]
memory: "legacy content"
`))
		require.Error(t, err)
	})

	t.Run("schema_version_1_rejected_at_validate", func(t *testing.T) {
		// A spec that decodes cleanly (only v2-shaped keys) but declares
		// schemaVersion "1" is rejected by the validating loader.
		dir := t.TempDir()
		specYAML := `schemaVersion: "1"
kind: sandbox
name: stale-version
sandbox:
  image: example/test:latest
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "spec.yaml"), []byte(specYAML), 0o644))
		_, err := LoadFromDirectory(dir)
		require.ErrorContains(t, err, "unsupported schemaVersion")
	})
}

// TestV2CapsNetwork_Accepted exercises the v2 caps.network block —
// allow + deny lists with the three P2 formats (exact, exact:port,
// single-label wildcard).
func TestV2CapsNetwork_Accepted(t *testing.T) {
	dir := t.TempDir()
	specYAML := `schemaVersion: "2"
kind: sandbox
name: caps-test
sandbox:
  image: docker/sandbox-templates:shell-docker
caps:
  network:
    allow: [api.anthropic.com, api.openai.com:443, "*.github.com"]
    deny: [malware.example.com]
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "spec.yaml"), []byte(specYAML), 0o644))
	art, err := LoadFromDirectory(dir)
	require.NoError(t, err)
	require.NotNil(t, art.Caps)
	require.NotNil(t, art.Caps.Network)
	require.ElementsMatch(t, []string{"api.anthropic.com", "api.openai.com:443", "*.github.com"}, art.Caps.Network.Allow)
	require.ElementsMatch(t, []string{"malware.example.com"}, art.Caps.Network.Deny)
}
