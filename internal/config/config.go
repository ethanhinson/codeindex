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
	Associations map[string]string `json:"associations,omitempty"`

	// Exclude lists repo-relative paths/globs to skip when indexing, in
	// addition to the built-in defaults (vendored/compiled dirs). A pattern
	// with no wildcard is a path prefix ("docs/generated" skips that
	// whole subtree); a pattern with '*'/'**'/'?' is a glob matched against the
	// repo-relative path ('**' spans directory separators).
	Exclude []string `json:"exclude"`

	// Include lists paths/globs to index even when a default or an Exclude
	// entry would skip them. Include always wins.
	Include []string `json:"include"`

	// Embed selects the semantic-search embedding provider. Zero value =
	// local provider with the bundled model.
	Embed EmbedConfig `json:"embed,omitempty"`
}

// EmbedConfig mirrors embed.Config field-for-field (config stays free of the
// CGO-heavy embed package; cmd wiring copies the fields across).
type EmbedConfig struct {
	Provider  string `json:"provider,omitempty"`    // "local" (default) | "api"
	Model     string `json:"model,omitempty"`       // local: cached model name or path ("" = bundled); api: model name
	APIBase   string `json:"api_base,omitempty"`    // api: endpoint base URL
	APIKeyEnv string `json:"api_key_env,omitempty"` // api: env var holding the credential
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

// Save writes root's config (pretty-printed; the file is committed and
// hand-edited). Used by `codeindex model use` to persist the model choice.
func Save(root string, c Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, FileName), append(b, '\n'), 0o644)
}
