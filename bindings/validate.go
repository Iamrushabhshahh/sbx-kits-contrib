package bindings

import "fmt"

// Validate checks the bindings file for structural well-formedness:
// every binding has at least one discovery entry, and every discovery
// entry has exactly one of Env or File set. Content-level rules
// (service-name pattern, allowedDomains entry format, env-var name
// patterns) are intentionally NOT enforced here — they belong to a
// future RFC, tracked alongside the Phase 3 deferred validation rules
// in docs/specs/2026-05-29-unified-kit-spec-v2-deferred-ideas.md.
func Validate(b *UserBindings) error {
	if b == nil {
		return nil
	}
	for service, binding := range b.Bindings {
		if service == "" {
			return fmt.Errorf("bindings: service name cannot be empty")
		}
		if len(binding.Discovery) == 0 {
			return fmt.Errorf("bindings[%q]: discovery list cannot be empty", service)
		}
		for i, d := range binding.Discovery {
			envSet := len(d.Env) > 0
			fileSet := d.File != nil
			switch {
			case envSet && fileSet:
				return fmt.Errorf("bindings[%q].discovery[%d]: exactly one of env or file must be set, got both", service, i)
			case !envSet && !fileSet:
				return fmt.Errorf("bindings[%q].discovery[%d]: exactly one of env or file must be set, got neither", service, i)
			}
			if fileSet && d.File.Path == "" {
				return fmt.Errorf("bindings[%q].discovery[%d].file.path is required", service, i)
			}
		}
	}
	return nil
}
