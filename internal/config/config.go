// Package config loads the committed repo configuration (.codeindex.json).
// Associations are a property of the repo — a Drupal codebase always needs
// *.module routed to PHP — so the file lives at the root, is committed, and
// every developer, CI job, and IDE surface inherits it.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FileName is the committed config file at the repo root.
const FileName = ".codeindex.json"

// Config is the repo-level configuration.
type Config struct {
	// Associations maps glob patterns to language names (go, tsjs, python,
	// php) — the VS Code files.associations model. Basename match; patterns
	// containing '/' match the repo-relative path.
	Associations map[string]string `json:"associations"`

	// Exclude lists repo-relative paths/globs to skip when indexing, in
	// addition to the built-in defaults (vendored/compiled dirs). A pattern
	// with no wildcard is a path prefix ("docs/generated" skips that
	// whole subtree); a pattern with '*'/'**'/'?' is a glob matched against the
	// repo-relative path ('**' spans directory separators).
	Exclude []string `json:"exclude"`

	// Include lists paths/globs to index even when a default or an Exclude
	// entry would skip them. Include always wins.
	Include []string `json:"include"`
}

// Load reads root's config. A missing file is an empty config; a malformed
// file is an error — a committed misconfiguration must fail builds loudly,
// not silently index nothing.
func Load(root string) (Config, error) {
	b, err := os.ReadFile(filepath.Join(root, FileName))
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("%s: %w", FileName, err)
	}
	return c, nil
}
