package spec

import (
	"fmt"
	"sort"
)

// normalize converts sugar fields in specFile into canonical Artifact fields.
// Non-fatal validation issues (e.g. not-yet-implemented forward-compat blocks)
// are collected on w; callers surface them via Artifact.Warnings.
func (s *specFile) normalize(w *warnings) error {
	s.normalizeMixins(w)
	if err := s.normalizeSandbox(w); err != nil {
		return err
	}
	s.deriveProxyManagedEnv()
	s.normalizeVolumes()
	return nil
}

// deriveProxyManagedEnv sets Environment.ProxyManaged to the names of the
// credentials whose apiKey is marked ProxyManaged. This is the canonical
// source of the in-container sentinel set: v2 specs declare the marker
// directly on apiKey.proxyManaged, and the engine consumer reads the derived
// Environment.ProxyManaged list. It deliberately OVERRIDES any decoded list so
// there is one source of truth.
func (s *specFile) deriveProxyManagedEnv() {
	var names []string
	seen := map[string]bool{}
	for _, c := range s.Credentials.List {
		if c.ApiKey != nil && c.ApiKey.ProxyManaged && c.ApiKey.Name != "" && !seen[c.ApiKey.Name] {
			seen[c.ApiKey.Name] = true
			names = append(names, c.ApiKey.Name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		if s.Environment != nil {
			s.Environment.ProxyManaged = nil
		}
		return
	}
	if s.Environment == nil {
		s.Environment = &EnvironmentPolicy{}
	}
	s.Environment.ProxyManaged = names
}

// normalizeVolumes folds the specFile-level volumes sequence into the
// canonical Manifest.Volumes slice. The wrapper carries the decode so
// Manifest.Volumes (yaml:"-") stays the canonical Go-level destination.
func (s *specFile) normalizeVolumes() {
	if len(s.Volumes.List) > 0 {
		s.Manifest.Volumes = append(s.Manifest.Volumes, s.Volumes.List...)
		s.Volumes.List = nil
	}
}

// normalizeMixins records that the forward-looking `mixins:` field was
// declared. Mixin composition (resolve the extends chain, apply the kit's
// own fields, then apply mixins in declaration order) is not wired in this
// release; the field is accepted so kits and the published v2 docs can use
// it, but it has no runtime effect yet. The value is carried through to
// Artifact.Mixins unchanged.
func (s *specFile) normalizeMixins(w *warnings) {
	if len(s.Mixins) == 0 {
		return
	}
	w.notImplemented("mixins", "mixin composition is accepted in the schema but not yet applied by the runtime")
}

// normalizeSandbox populates Manifest fields from the sandbox: block. The flat
// `template`/`binary`/`runOptions`/`aiFilename` fields are rejected — the
// sandbox: block is the only canonical home in v2.
func (s *specFile) normalizeSandbox(w *warnings) error {
	isSandbox := s.Kind == KindSandbox

	if s.Template != "" || s.Binary != "" || len(s.RunOptions) > 0 {
		return fmt.Errorf("use the 'sandbox:' block instead of flat 'template'/'binary'/'runOptions' fields")
	}
	if s.AIFilename != "" {
		return fmt.Errorf("use 'sandbox.aiFilename' instead of flat 'aiFilename' field")
	}

	if s.Sandbox != nil && !isSandbox {
		return fmt.Errorf("'sandbox:' block is only valid for kind %q, not %q", KindSandbox, s.Kind)
	}

	if s.Sandbox == nil {
		if isSandbox {
			return fmt.Errorf("kind %q requires a 'sandbox:' block with at least 'sandbox.image'", KindSandbox)
		}
		return nil
	}

	s.Template = s.Sandbox.Image
	s.AIFilename = s.Sandbox.AIFilename
	s.Resources = s.Sandbox.Resources
	s.Build = s.Sandbox.Build

	// `sandbox.build` is accepted in the schema (so kits and the published v2
	// docs can declare it) but the runtime does not build images from it yet.
	// Warn whenever it is used, and reject build-only kits: an image source is
	// still required this release, so a kit that supplies only `build:` would
	// otherwise fail later with the generic "template is required" error.
	if s.Build != nil {
		w.notImplemented("sandbox.build", "Dockerfile builds are accepted in the schema but not yet built by the runtime; the image is taken from sandbox.image")
		if s.Sandbox.Image == "" {
			return fmt.Errorf("sandbox.build is accepted in the schema but not yet implemented — specify sandbox.image")
		}
	}

	if s.Sandbox.Entrypoint != nil {
		if len(s.Sandbox.Entrypoint.Run) > 0 {
			s.Binary = s.Sandbox.Entrypoint.Run[0]
			if len(s.Sandbox.Entrypoint.Run) > 1 {
				s.RunOptions = s.Sandbox.Entrypoint.Run[1:]
			}
		}
		if len(s.Sandbox.Entrypoint.Args) > 0 {
			s.RunOptions = append(s.RunOptions, s.Sandbox.Entrypoint.Args...)
		}
	}

	return nil
}
