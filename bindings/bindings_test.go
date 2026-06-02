package bindings

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_ParsesExampleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
bindings:
  anthropic:
    discovery:
      - env: [ANTHROPIC_API_KEY]
    allowedDomains: [api.anthropic.com]

  github:
    discovery:
      - env: [GH_TOKEN, GITHUB_TOKEN]
      - file:
          path: ~/.config/gh/token
    allowedDomains: [api.github.com, github.com]
`), 0o600))

	b, err := Load(path)
	require.NoError(t, err)
	require.NotNil(t, b)
	require.Contains(t, b.Bindings, "anthropic")
	require.Len(t, b.Bindings["anthropic"].Discovery, 1)
	require.Equal(t, []string{"ANTHROPIC_API_KEY"}, b.Bindings["anthropic"].Discovery[0].Env)
	require.Equal(t, []string{"api.anthropic.com"}, b.Bindings["anthropic"].AllowedDomains)

	require.Contains(t, b.Bindings, "github")
	require.Len(t, b.Bindings["github"].Discovery, 2)
	require.Equal(t, []string{"GH_TOKEN", "GITHUB_TOKEN"}, b.Bindings["github"].Discovery[0].Env)
	require.NotNil(t, b.Bindings["github"].Discovery[1].File)
	require.Equal(t, "~/.config/gh/token", b.Bindings["github"].Discovery[1].File.Path)
}

func TestLoad_MissingFileIsError(t *testing.T) {
	_, err := Load("/nonexistent/credentials.yaml")
	require.Error(t, err)
}

func TestLoad_EmptyPathIsError(t *testing.T) {
	_, err := Load("")
	require.Error(t, err)
}

func TestLoad_MalformedYAMLRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.yaml")
	require.NoError(t, os.WriteFile(path, []byte("bindings:\n  : oops\n  - unbalanced"), 0o600))
	_, err := Load(path)
	require.ErrorContains(t, err, "parse")
}

func TestLoad_DiscoveryWithBothEnvAndFileRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
bindings:
  bad:
    discovery:
      - env: [FOO]
        file:
          path: ~/foo
    allowedDomains: [foo.example.com]
`), 0o600))
	_, err := Load(path)
	require.ErrorContains(t, err, "exactly one of env or file")
}

func TestLoad_EmptyDiscoveryAccepted(t *testing.T) {
	// Discovery is optional — a binding with only allowedDomains is the
	// canonical way to express "the value lives in the secret store, I'm
	// just declaring the trust scope here." The resolver consults the
	// store before any user-declared discovery entries so an empty
	// discovery list is well-formed.
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
bindings:
  store-only:
    discovery: []
    allowedDomains: [foo.example.com]
`), 0o600))
	b, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, []string{"foo.example.com"}, b.Bindings["store-only"].AllowedDomains)
	require.Empty(t, b.Bindings["store-only"].Discovery)
}

func TestLoad_FilePathRequired(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
bindings:
  bad:
    discovery:
      - file: {}
    allowedDomains: [foo.example.com]
`), 0o600))
	_, err := Load(path)
	require.ErrorContains(t, err, "file.path is required")
}

func TestDefaultPath_ResolvesXDGConfigHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG_CONFIG_HOME is a non-Windows convention")
	}
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	path := DefaultPath()
	require.True(t, strings.HasPrefix(path, dir),
		"DefaultPath should resolve under XDG_CONFIG_HOME (%q), got %q", dir, path)
	require.True(t, strings.HasSuffix(path, "sbx/credentials.yaml"),
		"DefaultPath should end with sbx/credentials.yaml, got %q", path)
}

func TestDefaultPath_FallsBackToHomeDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("home-dir convention is non-Windows")
	}
	t.Setenv("XDG_CONFIG_HOME", "")

	path := DefaultPath()
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(path, home),
		"DefaultPath should fall back under $HOME (%q), got %q", home, path)
	require.True(t, strings.HasSuffix(path, ".config/sbx/credentials.yaml"),
		"DefaultPath should end with .config/sbx/credentials.yaml on non-Windows, got %q", path)
}
