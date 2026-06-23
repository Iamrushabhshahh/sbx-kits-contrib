package spec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeSandbox(t *testing.T) {
	t.Run("populates_manifest_from_sandbox_block", func(t *testing.T) {
		s := specFile{
			Manifest: Manifest{Kind: KindSandbox, SchemaVersion: SchemaVersion, Name: "a"},
			Sandbox: &sandboxBlock{
				Image:      "my-image",
				AIFilename: "AI.md",
				Resources:  &Resources{CPU: 4, MemoryMB: 8192, GPU: "1"},
				Entrypoint: &entrypointBlock{
					Run:  []string{"bin", "--flag"},
					Args: []string{"--extra"},
				},
			},
		}
		require.NoError(t, s.normalize(&warnings{}))
		require.Equal(t, "my-image", s.Template)
		require.Equal(t, "bin", s.Binary)
		require.Equal(t, []string{"--flag", "--extra"}, s.RunOptions)
		require.Equal(t, "AI.md", s.AIFilename)
		require.NotNil(t, s.Resources)
		require.InDelta(t, 4.0, s.Resources.CPU, 0.0001)
		require.Equal(t, int64(8192), s.Resources.MemoryMB)
		require.Equal(t, "1", s.Resources.GPU)
	})

	t.Run("rejects_sandbox_block_on_mixin", func(t *testing.T) {
		s := specFile{
			Manifest: Manifest{Kind: KindMixin, SchemaVersion: SchemaVersion, Name: "m"},
			Sandbox:  &sandboxBlock{Image: "img"},
		}
		require.ErrorContains(t, s.normalize(&warnings{}), "only valid for kind")
	})

	t.Run("rejects_flat_template_field", func(t *testing.T) {
		s := specFile{
			Manifest: Manifest{Kind: KindSandbox, SchemaVersion: SchemaVersion, Name: "a", Template: "img"},
		}
		require.ErrorContains(t, s.normalize(&warnings{}), "sandbox:")
	})

	t.Run("sandbox_requires_sandbox_block", func(t *testing.T) {
		s := specFile{
			Manifest: Manifest{Kind: KindSandbox, SchemaVersion: SchemaVersion, Name: "a"},
		}
		require.ErrorContains(t, s.normalize(&warnings{}), "requires a 'sandbox:' block")
	})
}

// A v2 spec that declares apiKey.proxyManaged directly (no environment block)
// gets Environment.ProxyManaged derived from the markers, so the engine sees
// the in-container sentinel set without a v1 environment.proxyManaged list.
func TestNormalizeDerivesProxyManagedFromV2Markers(t *testing.T) {
	in := []byte(`schemaVersion: "2"
kind: sandbox
name: t
sandbox: {image: x}
credentials:
  - service: openai
    apiKey: {name: OPENAI_API_KEY, proxyManaged: true, inject: [{domain: api.openai.com, header: Authorization, format: "Bearer %s"}]}
  - service: github
    apiKey: {name: GH_TOKEN, inject: [{domain: api.github.com, header: Authorization, format: "Bearer %s"}]}
`)
	a, err := LoadFromBytes(in)
	require.NoError(t, err)
	require.NotNil(t, a.Environment)
	require.Equal(t, []string{"OPENAI_API_KEY"}, a.Environment.ProxyManaged) // github unmarked → excluded
}
