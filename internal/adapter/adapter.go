// Package adapter defines the pluggable per-language parsing seam. Each language
// implements Adapter; the registry maps file extensions to adapters. The full
// engine adds languages by registering more adapters without touching existing ones.
package adapter

import (
	"path/filepath"
	"sort"

	"codeindex/internal/graph"
)

// Adapter parses one language's source into symbols and call sites.
type Adapter interface {
	// Extensions returns the file extensions this adapter handles (e.g. ".go").
	Extensions() []string
	// Parse turns source bytes into a ParsedFile (symbols + raw call sites).
	Parse(path string, src []byte) (*graph.ParsedFile, error)
}

var registry = map[string]Adapter{}

// Register makes an adapter available for its extensions. Later registrations
// for the same extension win (last-writer), which keeps tests simple.
func Register(a Adapter) {
	for _, ext := range a.Extensions() {
		registry[ext] = a
	}
}

// For returns the adapter for a path's extension, or nil if unsupported.
func For(path string) Adapter {
	return registry[filepath.Ext(path)]
}

// Extensions returns the sorted set of registered file extensions — the
// single source of truth for what the repo walk indexes.
func Extensions() []string {
	out := make([]string, 0, len(registry))
	for ext := range registry {
		out = append(out, ext)
	}
	sort.Strings(out)
	return out
}
