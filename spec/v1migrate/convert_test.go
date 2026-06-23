package v1migrate_test

import (
	"strings"
	"testing"

	"github.com/docker/sbx-kits-contrib/spec"
	"github.com/docker/sbx-kits-contrib/spec/v1migrate"
)

// TestConvert_V1FoldsToV2 is a self-contained smoke test of the converter: a
// minimal v1 spec exercising the headline transforms (kind/agent rename,
// network.allowedDomains -> caps.network.allow) must emit a canonical v2 spec
// that loads cleanly as schemaVersion "2", and report the deprecations.
func TestConvert_V1FoldsToV2(t *testing.T) {
	v1 := []byte(`schemaVersion: "1"
kind: agent
name: demo
agent:
  image: example/demo:latest
network:
  allowedDomains:
    - "example.com:443"
`)

	out, changes, err := v1migrate.Convert(v1)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected deprecation change lines for a v1 spec, got none")
	}

	// The emitted spec must be canonical v2 and re-load cleanly (no v1 folding).
	if !strings.Contains(string(out), `schemaVersion: "2"`) {
		t.Errorf("emitted spec is not schemaVersion 2:\n%s", out)
	}
	if !strings.Contains(string(out), "caps:") || !strings.Contains(string(out), "allow:") {
		t.Errorf("network.allowedDomains was not folded to caps.network.allow:\n%s", out)
	}
	art, err := spec.LoadFromBytes(out)
	if err != nil {
		t.Fatalf("emitted spec failed to re-load: %v", err)
	}
	if art.Manifest.SchemaVersion != "2" {
		t.Errorf("re-loaded artifact schemaVersion = %q, want 2", art.Manifest.SchemaVersion)
	}
	if art.Manifest.Kind != string(spec.KindSandbox) {
		t.Errorf("v1 kind:agent did not map to sandbox; got %q", art.Manifest.Kind)
	}
}

// TestConvert_AlreadyV2IsNoOp confirms a clean v2 spec yields no changes (the
// converter must not churn canonical v2 input).
func TestConvert_AlreadyV2IsNoOp(t *testing.T) {
	v2 := []byte(`schemaVersion: "2"
kind: sandbox
name: demo
sandbox:
  image: example/demo:latest
caps:
  network:
    allow:
      - "example.com:443"
`)

	_, changes, err := v1migrate.Convert(v2)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected no changes on a clean v2 spec, got %v", changes)
	}
}
