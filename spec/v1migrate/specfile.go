package v1migrate

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

// specFile is the on-disk YAML schema for spec.yaml.
type specFile struct {
	Manifest `yaml:",inline"`
	// Volumes is the polymorphic-decode wrapper for the `volumes:` YAML
	// key, handling both the v1 mapping shape and the v2 sequence shape.
	// Manifest.Volumes carries `yaml:"-"` so this field owns the decode;
	// normalize folds Volumes.List + Volumes.LegacyMap into the canonical
	// Manifest.Volumes slice.
	Volumes  volumesField  `yaml:"volumes,omitempty"`
	Extends  string        `yaml:"extends,omitempty"`
	Mixins   []string      `yaml:"mixins,omitempty"`
	Locked   []string      `yaml:"locked,omitempty"`
	Licenses []string      `yaml:"licenses,omitempty"`
	Sandbox  *sandboxBlock `yaml:"sandbox,omitempty"`
	// LegacyAgent holds the v1 `agent:` block. The normalize step
	// migrates its contents to Sandbox with a deprecation warning. Drop
	// in the Phase 6 schema-cutover commit.
	LegacyAgent *sandboxBlock     `yaml:"agent,omitempty"`
	Secrets     []string          `yaml:"secrets,omitempty"`
	Egress      map[string]string `yaml:"egress,omitempty"`
	// Credentials is the polymorphic-decode wrapper handling both v1
	// (mapping with sources:) and v2 (sequence of Credential) shapes.
	// normalizeLegacyCredentials folds the v1 surface plus the
	// LegacyNetwork / LegacyOAuth / Environment.ProxyManaged
	// shims into Artifact.Credentials.
	Credentials credentialsField `yaml:"credentials,omitempty"`
	// PublishedPorts is the v2 canonical top-level `publishedPorts:` list.
	// Decoded directly from YAML; normalize also promotes the v1
	// LegacyNetwork.PublishedPorts shim into this slice.
	PublishedPorts []PublishedPort `yaml:"publishedPorts,omitempty"`
	// LegacyNetwork absorbs the v1 top-level `network:` block. normalize
	// folds its serviceDomains/serviceAuth fields into Credentials, its
	// allowedDomains/deniedDomains into Caps.Network, and its publishedPorts
	// into the top-level PublishedPorts. Removed in the Phase 6 schema cutover.
	LegacyNetwork *NetworkPolicy     `yaml:"network,omitempty"`
	Environment   *EnvironmentPolicy `yaml:"environment,omitempty"`
	// LegacySettings absorbs the v1 `settings:` block. There is no v2 field
	// to map it into — the container-settings behavior was lifted into each
	// kit's initFiles/commands.startup (Phase 4) — so normalizeLegacySettings
	// drops it with a deprecation warning. Kept only so KnownFields(true)
	// strict decode still admits a stray `settings:` block instead of hard-
	// rejecting it. Removed in the Phase 6 schema cutover.
	LegacySettings *SettingsPolicy `yaml:"settings,omitempty"`
	Commands       *CommandsPolicy `yaml:"commands,omitempty"`
	// Caps is the v2 capabilities block (caps.network and any future
	// caps.* surfaces). Decoded directly from YAML; the normalize step
	// also populates Caps.Network from the v1 network.allowedDomains/
	// deniedDomains shim (LegacyNetwork) when those are present.
	Caps *Caps `yaml:"caps,omitempty"`
	// LegacyOAuth absorbs the v1 standalone top-level `oauth:` block.
	// normalize folds it into Credentials[].OAuth (matched by service)
	// or synthesizes a new Credential entry if no entry exists for its
	// service yet. Removed in the Phase 6 schema cutover.
	LegacyOAuth  *OAuthPolicy `yaml:"oauth,omitempty"`
	AgentContext string       `yaml:"agentContext,omitempty"`
	// LegacyMemory holds the v1 `memory:` field. The normalize step
	// migrates it to AgentContext with a deprecation warning. Drop in
	// the Phase 6 schema-cutover commit.
	LegacyMemory string `yaml:"memory,omitempty"`
	// LegacyPersistence holds the v1 `persistence:` field. The field was
	// declared, parsed, inherited, displayed, but never consumed by any
	// runtime decision (see sandboxes commit 05e5b4eef adopting PR #37).
	// It was removed from the canonical types in PR #37, but that same PR
	// also flipped on strict YAML decoding — turning what had been a silent
	// no-op into a hard error for any kit author whose spec still carried
	// the line. The normalize step now drops it with a deprecation warning
	// to give those kits one release to migrate. Drop in the Phase 6
	// schema-cutover commit.
	LegacyPersistence string `yaml:"persistence,omitempty"`
	// LegacyKitDir holds the v1 `kitDir:` field. Same story as
	// LegacyPersistence — declared but never consumed, removed in PR #37,
	// re-admitted here as a deprecation-warning shim. Drop in the Phase 6
	// schema-cutover commit.
	LegacyKitDir string `yaml:"kitDir,omitempty"`
	// LegacyTmpfs holds the v1 `tmpfs:` block as a mapping from container
	// path to size string (e.g. `{ /tmp/scratch: "512m" }`). The v1 shape
	// was first replaced by `Tmpfs []MountSpec` (PR #37) and then deleted
	// entirely by PR #59 in favor of `volumes:` entries with `type: tmpfs`.
	// The strict-decode flip turned a no-op into a hard rejection;
	// normalize folds entries into Manifest.Volumes with Type=Tmpfs and
	// emits a deprecation warning. Drop in the Phase 6 schema-cutover
	// commit.
	LegacyTmpfs map[string]string `yaml:"tmpfs,omitempty"`
}

// credentialsField is the specFile-level polymorphic wrapper for the
// `credentials:` YAML key. It handles both v1 (mapping with sources:
// inside) and v2 (sequence of Credential) shapes. The normalize step
// reads LegacySources (if present) plus the Legacy fields under network:
// and environment:, constructs []Credential, and stores into
// Artifact.Credentials.
//
// Phase 1's two-yaml-tag pattern (used for memory/agentContext and
// agent/sandbox) doesn't apply here because v1 and v2 share the same
// `credentials:` YAML tag with different value kinds — only a custom
// UnmarshalYAML can disambiguate.
type credentialsField struct {
	// List is populated when credentials: is a sequence (v2 spelling).
	List []Credential

	// LegacySources is populated when credentials: is a mapping with
	// sources: under it (v1 spelling). Each entry carries the env/file
	// discovery hints the v1 shape used.
	LegacySources map[string]CredentialSource
}

func (c *credentialsField) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		return node.Decode(&c.List)
	case yaml.MappingNode:
		var v1 struct {
			Sources map[string]CredentialSource `yaml:"sources"`
		}
		if err := node.Decode(&v1); err != nil {
			return err
		}
		c.LegacySources = v1.Sources
		return nil
	case 0:
		return nil
	default:
		return fmt.Errorf("credentials: must be a list (v2) or a mapping with sources: (v1)")
	}
}

// volumesField is the specFile-level polymorphic wrapper for the `volumes:`
// YAML key. PR #37 replaced the v1 mapping shape
// (`volumes: { /path: "size" }`) with the v2 sequence shape
// (`volumes: [{ path: /path, size: "100m" }]`), then flipped on strict
// decoding in the same commit. Strict decode hard-fails the v1 mapping
// shape with a type-mismatch error rather than a "field not found"; this
// wrapper accepts both shapes and lets normalize fold the legacy form into
// Manifest.Volumes with a deprecation warning.
type volumesField struct {
	// List is populated when volumes: is a sequence (v2 spelling).
	List []MountSpec

	// LegacyMap is populated when volumes: is a mapping (v1 spelling):
	// each key is the container mount path, each value is a size string
	// (or empty when the v1 spec carried no size).
	LegacyMap map[string]string
}

func (v *volumesField) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		return node.Decode(&v.List)
	case yaml.MappingNode:
		var m map[string]string
		if err := node.Decode(&m); err != nil {
			return err
		}
		v.LegacyMap = m
		return nil
	case 0:
		return nil
	default:
		return fmt.Errorf("volumes: must be a list (v2) or a mapping (v1)")
	}
}

// sandboxBlock groups sandbox-specific configuration (formerly the
// `agent:` block in v1). The Go type was renamed alongside the YAML
// field rename to keep call sites legible.
type sandboxBlock struct {
	Image      string           `yaml:"image,omitempty"`
	Build      *BuildConfig     `yaml:"build,omitempty"`
	Entrypoint *entrypointBlock `yaml:"entrypoint,omitempty"`
	AIFilename string           `yaml:"aiFilename,omitempty"`
	Resources  *Resources       `yaml:"resources,omitempty"`
	// LegacyPersistence holds the v1 `persistence:` field that lived inside
	// the (then-)agent block. PR #37 deleted it (declared but never
	// consumed) and flipped on strict decoding in the same commit, turning
	// the silent no-op into a hard error for any kit that still had it.
	// normalizeSandbox drops it with a deprecation warning. Drop in the
	// Phase 6 schema-cutover commit alongside LegacyAgent.
	LegacyPersistence string `yaml:"persistence,omitempty"`
}

// entrypointBlock describes the agent's process launch configuration.
type entrypointBlock struct {
	Run      []string `yaml:"run,omitempty"`
	Args     []string `yaml:"args,omitempty"`
	TtyArgs  []string `yaml:"ttyArgs,omitempty"`
	PipeMode string   `yaml:"pipeMode,omitempty"`
}
