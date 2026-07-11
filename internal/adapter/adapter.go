// Package adapter defines the pluggable per-language parsing seam. Each language
// implements Adapter; the registry maps file extensions to adapters. The full
// engine adds languages by registering more adapters without touching existing ones.
package adapter

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"codeindex/internal/graph"
)

// Adapter parses one language's source into symbols and call sites.
type Adapter interface {
	// Name is the language name association configs reference (e.g. "php").
	Name() string
	// Extensions returns the file extensions this adapter handles (e.g. ".go").
	Extensions() []string
	// Parse turns source bytes into a ParsedFile (symbols + raw call sites).
	Parse(path string, src []byte) (*graph.ParsedFile, error)
}

var (
	registry = map[string]Adapter{}
	byName   = map[string]Adapter{}
	// assocRules route paths by glob pattern, checked BEFORE extensions —
	// explicit repo intent (.codeindex.json) beats built-ins. Set per
	// build/patch; index mutation is serialized upstream (query.mu).
	assocRules []assocRule
)

type assocRule struct {
	pattern string // contains '/': match repo-relative path; else basename
	a       Adapter
}

// exactRoutes maps individual repo-relative paths to adapters — filled from
// content sniffing at walk time (a .module file whose head says <?php).
var exactRoutes = map[string]Adapter{}

// Register makes an adapter available for its extensions. Later registrations
// for the same extension win (last-writer), which keeps tests simple.
func Register(a Adapter) {
	byName[a.Name()] = a
	for _, ext := range a.Extensions() {
		registry[ext] = a
	}
}

// SetAssociations installs the repo's pattern->language routes, replacing
// any prior set. Unknown language names are an error listing the valid ones
// — a typo must not silently skip files.
func SetAssociations(assoc map[string]string) error {
	rules := make([]assocRule, 0, len(assoc))
	for pattern, lang := range assoc {
		a, ok := byName[lang]
		if !ok {
			return fmt.Errorf("%s: unknown language %q for pattern %q (valid: %s)",
				"associations", lang, pattern, strings.Join(Names(), ", "))
		}
		rules = append(rules, assocRule{pattern: pattern, a: a})
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].pattern < rules[j].pattern })
	assocRules = rules
	return nil
}

// SetExactRoutes installs per-path routes discovered by content sniffing,
// replacing the prior set. Unknown language names are skipped (sniffing only
// produces registered names; this is belt-and-braces, not user input).
func SetExactRoutes(routes map[string]string) {
	m := make(map[string]Adapter, len(routes))
	for path, lang := range routes {
		if a, ok := byName[lang]; ok {
			m[path] = a
		}
	}
	exactRoutes = m
}

// ForName returns the adapter registered under a language name, or nil.
func ForName(lang string) Adapter { return byName[lang] }

// Names returns the sorted registered language names.
func Names() []string {
	out := make([]string, 0, len(byName))
	for n := range byName {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// For returns the adapter for a repo-relative path: association rules first,
// then the extension registry; nil if unsupported.
func For(path string) Adapter {
	for _, r := range assocRules {
		target := filepath.Base(path)
		if strings.ContainsRune(r.pattern, '/') {
			target = filepath.ToSlash(path)
		}
		if ok, _ := filepath.Match(r.pattern, target); ok {
			return r.a
		}
	}
	if a, ok := registry[filepath.Ext(path)]; ok {
		return a
	}
	return exactRoutes[filepath.ToSlash(path)]
}

// Indexable reports whether the walk should include a repo-relative path —
// the registry (extensions + associations) is the single source of truth.
func Indexable(path string) bool { return For(path) != nil }

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
