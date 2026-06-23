package v1migrate

import (
	"bytes"
	"fmt"

	"go.yaml.in/yaml/v3"
)

// loadFromBytes decodes a v1 (or already-v2) spec.yaml and applies the v1->v2
// normalize passes, returning the canonical Artifact. It is v1migrate's own copy
// of the spec package's former v1-aware loader: the production spec.LoadFromBytes
// is v2-only after the Phase 6 cutover, so the migration tool keeps its own
// v1-capable decode here. The whole package is a clean delete once v1 is gone.
func loadFromBytes(data []byte) (*Artifact, error) {
	var sf specFile
	// Strict decoding: unknown keys are a hard error, matching the production
	// loader — a stray key is almost always a typo or an unmigrated v1 field.
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&sf); err != nil {
		return nil, fmt.Errorf("invalid spec.yaml: %w", err)
	}

	w := &warnings{}
	if err := sf.normalize(w); err != nil {
		return nil, fmt.Errorf("normalize: %w", err)
	}

	return &Artifact{
		Manifest:       sf.Manifest,
		Extends:        sf.Extends,
		Mixins:         sf.Mixins,
		Locked:         sf.Locked,
		Licenses:       sf.Licenses,
		PublishedPorts: sf.PublishedPorts,
		Caps:           sf.Caps,
		Credentials:    sf.Credentials.List,
		Environment:    sf.Environment,
		Commands:       sf.Commands,
		AgentContext:   sf.AgentContext,
		Warnings:       w.messages,
	}, nil
}
