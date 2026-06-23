// Command migrate-v1-to-v2 walks a kit artifact directory, rewrites its
// spec.yaml from the schemaVersion "1" field shapes to the canonical
// schemaVersion "2" shapes, and writes a `.bak` of the original. It is the
// mechanical companion to the v2 spec-package breaking changes — kit authors
// run it once to convert their kit without hand-editing every renamed,
// relocated, or consolidated field.
//
// Usage:
//
//	go run ./scripts/migrate-v1-to-v2 <path-to-kit-directory>
//
// This command is only the file-I/O wrapper: the actual v1 → v2 conversion
// lives in the spec/v1migrate package (which loads through the v1-aware decode +
// normalize pass and re-emits canonical v2). See that package for the full list
// of transforms and the rationale for keeping v1 decode isolated from the
// production (v2-only) spec loader.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/docker/sbx-kits-contrib/spec/v1migrate"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: migrate-v1-to-v2 <kit-directory>")
		os.Exit(2)
	}
	if err := migrate(os.Args[1], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// migrate runs the in-place migration on the spec.yaml under kitDir.
// All progress messages are written to w; the only error condition is
// a real I/O or contract failure (missing spec.yaml, undecodable spec,
// refuse to clobber an existing .bak, etc.). A spec that already uses only
// canonical v2 fields is a successful no-op: nothing is rewritten and no
// .bak is created.
func migrate(kitDir string, w io.Writer) error {
	specPath, err := findSpec(kitDir)
	if err != nil {
		return err
	}

	original, err := os.ReadFile(specPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", specPath, err)
	}

	migrated, changes, err := v1migrate.Convert(original)
	if err != nil {
		return fmt.Errorf("%s: %w", specPath, err)
	}

	if len(changes) == 0 {
		fmt.Fprintf(w, "no changes needed in %s\n", specPath)
		return nil
	}

	bakPath := specPath + ".bak"
	if _, err := os.Stat(bakPath); err == nil {
		return fmt.Errorf("%s already exists; refusing to clobber an existing backup", bakPath)
	}
	if err := os.WriteFile(bakPath, original, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", bakPath, err)
	}
	if err := os.WriteFile(specPath, migrated, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", specPath, err)
	}

	fmt.Fprintf(w, "migrated %s (backup at %s)\n", specPath, bakPath)
	for _, c := range changes {
		fmt.Fprintf(w, "  - %s\n", c)
	}
	return nil
}

// findSpec returns the path to spec.yaml (or spec.yml) under kitDir,
// or an error if neither exists.
func findSpec(kitDir string) (string, error) {
	for _, name := range []string{"spec.yaml", "spec.yml"} {
		path := filepath.Join(kitDir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no spec.yaml or spec.yml found in %s", kitDir)
}
