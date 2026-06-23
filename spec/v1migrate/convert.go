// Package v1migrate is the one-way schemaVersion "1" → "2" converter that backs
// the migrate-v1-to-v2 tool. It is deliberately a SEPARATE package from spec:
// the production spec loader is (or is becoming) v2-only, while kit authors may
// still hold v1 spec.yaml files they have not migrated yet. Isolating the v1
// decode + folding here keeps it out of the code paths the engine exercises and
// makes the eventual full v1 removal a clean delete of this one subpackage.
//
// Convert loads v1 (or already-v2) spec.yaml bytes through the v1-aware decode +
// normalize pass and re-emits a canonical v2 spec.yaml.
package v1migrate

import (
	"fmt"

	"github.com/docker/sbx-kits-contrib/spec"
	"go.yaml.in/yaml/v3"
)

// Convert loads v1 (or already-v2) spec.yaml bytes through the spec package's
// normalize pass and re-emits a canonical v2 spec.yaml. It returns the
// rewritten YAML and a list of human-readable change descriptions (the spec
// package's own deprecation warnings, plus any script-level drops such as
// settings:). changes is empty when the input already uses only canonical v2
// fields, in which case the returned bytes equal the input (no rewrite needed).
func Convert(data []byte) ([]byte, []string, error) {
	a, err := loadFromBytes(data)
	if err != nil {
		return nil, nil, err
	}

	// The spec loader flattens the sandbox entrypoint (run/args/ttyArgs/
	// pipeMode) into Manifest.Binary/RunOptions and drops ttyArgs/pipeMode, so
	// it cannot faithfully reconstruct the entrypoint block. Re-read the raw
	// entrypoint directly from the source YAML (the v1 `agent:` and v2
	// `sandbox:` blocks share an identical entrypoint shape) and carry it
	// verbatim into the v2 sandbox: block.
	var raw rawSpec
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("re-read entrypoint: %w", err)
	}
	srcSandbox := raw.Sandbox
	if srcSandbox == nil {
		srcSandbox = raw.Agent
	}

	out := buildV2(a, srcSandbox)

	// normalizeLegacySettings (in the spec package) emits the settings:
	// deprecation/lift notice into a.Warnings, so the dead Artifact.Settings
	// detector that used to live here is gone — the notice flows through the
	// warnings fold below.
	changes := append([]string(nil), a.Warnings...)

	if len(changes) == 0 {
		return data, nil, nil
	}

	emitted, err := yaml.Marshal(out)
	if err != nil {
		return nil, nil, fmt.Errorf("emit v2 spec: %w", err)
	}

	// Safety net: the rewritten spec must parse cleanly through the same
	// loader. A decode error here means we produced a malformed spec.yaml —
	// fail loudly rather than overwrite the author's file with garbage.
	if _, err := spec.LoadFromBytes(emitted); err != nil {
		return nil, nil, fmt.Errorf("migrated spec failed to re-parse: %w", err)
	}

	return emitted, changes, nil
}

// buildV2 assembles the canonical v2 output spec from a normalized Artifact.
// The credentials/caps/publishedPorts/agentContext/kind values come from the
// normalized Artifact (where the v1 → v2 consolidation already happened); the
// sandbox entrypoint comes from srcSandbox (the raw source block) to preserve
// the run/args/ttyArgs/pipeMode distinctions the Artifact flattens away.
func buildV2(a *spec.Artifact, srcSandbox *rawSandbox) *outSpec {
	out := &outSpec{
		// Always emit the v2 schema version: a migrated kit is a fully-declared
		// v2 spec. This is reached only when there were v1 constructs to migrate
		// (callers gate on a non-empty change list), so a clean v2 spec is never
		// rewritten just to touch this field.
		SchemaVersion:  "2",
		Kind:           a.Manifest.Kind,
		Name:           a.Manifest.Name,
		Version:        a.Manifest.Version,
		DisplayName:    a.Manifest.DisplayName,
		Description:    a.Manifest.Description,
		SourceURL:      a.Manifest.SourceURL,
		Extends:        a.Extends,
		Locked:         a.Locked,
		Volumes:        a.Manifest.Volumes,
		Security:       a.Manifest.Security,
		PublishedPorts: a.PublishedPorts,
		Caps:           a.Caps,
		Credentials:    a.Credentials,
		Commands:       a.Commands,
		AgentContext:   a.AgentContext,
	}

	hasEntrypoint := srcSandbox != nil && srcSandbox.Entrypoint != nil
	if a.Manifest.Template != "" || a.Manifest.AIFilename != "" || a.Manifest.Resources != nil || hasEntrypoint {
		sb := &outSandbox{
			Image:      a.Manifest.Template,
			AIFilename: a.Manifest.AIFilename,
			Resources:  a.Manifest.Resources,
		}
		if hasEntrypoint {
			sb.Entrypoint = &outEntrypoint{
				Run:      srcSandbox.Entrypoint.Run,
				Args:     srcSandbox.Entrypoint.Args,
				TtyArgs:  srcSandbox.Entrypoint.TtyArgs,
				PipeMode: srcSandbox.Entrypoint.PipeMode,
			}
		}
		out.Sandbox = sb
	}

	if a.Environment != nil && len(a.Environment.Variables) > 0 {
		out.Environment = &outEnv{Variables: a.Environment.Variables}
	}

	return out
}

// outSpec is the canonical v2 spec.yaml emit shape. Field order here is the
// emit order; yaml.Marshal writes struct fields in declaration order. Folded
// and removed v1 blocks (network:, oauth:, settings:, credentials.sources,
// environment.proxyManaged) deliberately have no field here, so they never
// appear in the output.
type outSpec struct {
	SchemaVersion  string               `yaml:"schemaVersion"`
	Kind           string               `yaml:"kind"`
	Name           string               `yaml:"name"`
	Version        string               `yaml:"version,omitempty"`
	DisplayName    string               `yaml:"displayName,omitempty"`
	Description    string               `yaml:"description,omitempty"`
	SourceURL      string               `yaml:"sourceURL,omitempty"`
	Extends        string               `yaml:"extends,omitempty"`
	Locked         []string             `yaml:"locked,omitempty"`
	Sandbox        *outSandbox          `yaml:"sandbox,omitempty"`
	Volumes        []spec.MountSpec     `yaml:"volumes,omitempty"`
	Security       *spec.Security       `yaml:"security,omitempty"`
	PublishedPorts []spec.PublishedPort `yaml:"publishedPorts,omitempty"`
	Caps           *spec.Caps           `yaml:"caps,omitempty"`
	Credentials    []spec.Credential    `yaml:"credentials,omitempty"`
	Environment    *outEnv              `yaml:"environment,omitempty"`
	Commands       *spec.CommandsPolicy `yaml:"commands,omitempty"`
	AgentContext   string               `yaml:"agentContext,omitempty"`
}

// outSandbox is the v2 sandbox: block emit shape.
type outSandbox struct {
	Image      string          `yaml:"image,omitempty"`
	AIFilename string          `yaml:"aiFilename,omitempty"`
	Entrypoint *outEntrypoint  `yaml:"entrypoint,omitempty"`
	Resources  *spec.Resources `yaml:"resources,omitempty"`
}

// outEntrypoint is the sandbox.entrypoint emit shape.
type outEntrypoint struct {
	Run      []string `yaml:"run,omitempty"`
	Args     []string `yaml:"args,omitempty"`
	TtyArgs  []string `yaml:"ttyArgs,omitempty"`
	PipeMode string   `yaml:"pipeMode,omitempty"`
}

// outEnv is the environment: block emit shape, restricted to the canonical v2
// variables: map (the removed proxyManaged list has no field and so is never
// emitted).
type outEnv struct {
	Variables map[string]string `yaml:"variables,omitempty"`
}

// rawSpec / rawSandbox / rawEntrypoint capture only the entrypoint block from
// the source spec.yaml, accepting both the v1 `agent:` and v2 `sandbox:`
// spellings. They exist solely to preserve run/args/ttyArgs/pipeMode through
// the migration, since the normalized Artifact discards that structure.
type rawSpec struct {
	Sandbox *rawSandbox `yaml:"sandbox,omitempty"`
	Agent   *rawSandbox `yaml:"agent,omitempty"`
}

type rawSandbox struct {
	Entrypoint *rawEntrypoint `yaml:"entrypoint,omitempty"`
}

type rawEntrypoint struct {
	Run      []string `yaml:"run,omitempty"`
	Args     []string `yaml:"args,omitempty"`
	TtyArgs  []string `yaml:"ttyArgs,omitempty"`
	PipeMode string   `yaml:"pipeMode,omitempty"`
}
