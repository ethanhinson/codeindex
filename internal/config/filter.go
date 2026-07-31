package config

import (
	"regexp"
	"strings"
)

// DefaultExcludeDirs are directory basenames pruned from the walk by default:
// version control, dependency vendoring, and compiled/generated output. A
// repo's Include list can re-admit any of them.
var DefaultExcludeDirs = []string{
	".git", ".hg", ".svn",
	"node_modules", "vendor", "bower_components",
	"dist", "build", "out", "target", "coverage",
	".next", ".nuxt", ".svelte-kit", ".output",
	"testdata", ".codeindex",
}

// DefaultExcludeGlobs are file globs skipped by default (minified bundles).
var DefaultExcludeGlobs = []string{"**/*.min.js", "**/*.min.css"}

// Filter decides which repo-relative paths are indexed. Precedence:
// Include (always wins) > Exclude / defaults.
type Filter struct {
	skipDirs    map[string]bool  // default directory basenames
	excludeGlob []*regexp.Regexp // glob-form exclude patterns (defaults + user)
	excludePfx  []string         // wildcard-free exclude patterns (path prefixes)
	includeGlob []*regexp.Regexp
	includePfx  []string
	includeLits []string // literal prefixes of include patterns, for dir protection
}

// NewFilter builds a Filter from a Config.
func NewFilter(cfg Config) *Filter {
	f := &Filter{skipDirs: map[string]bool{}}
	for _, d := range DefaultExcludeDirs {
		f.skipDirs[d] = true
	}
	for _, g := range DefaultExcludeGlobs {
		f.excludeGlob = append(f.excludeGlob, globToRegexp(g))
	}
	for _, p := range cfg.Exclude {
		if hasGlob(p) {
			f.excludeGlob = append(f.excludeGlob, globToRegexp(p))
		} else {
			f.excludePfx = append(f.excludePfx, cleanPat(p))
		}
	}
	for _, p := range cfg.Include {
		f.includeLits = append(f.includeLits, literalPrefix(p))
		if hasGlob(p) {
			f.includeGlob = append(f.includeGlob, globToRegexp(p))
		} else {
			f.includePfx = append(f.includePfx, cleanPat(p))
		}
	}
	return f
}

// LoadFilter loads root's config and returns its Filter. A missing config
// yields the default filter; a malformed config is an error.
func LoadFilter(root string) (*Filter, error) {
	cfg, err := Load(root)
	if err != nil {
		return nil, err
	}
	return NewFilter(cfg), nil
}

// Included reports whether rel is force-included (overrides any exclusion).
func (f *Filter) Included(rel string) bool {
	for _, p := range f.includePfx {
		if rel == p || strings.HasPrefix(rel, p+"/") {
			return true
		}
	}
	for _, re := range f.includeGlob {
		if re.MatchString(rel) {
			return true
		}
	}
	return false
}

func (f *Filter) excluded(rel string) bool {
	for _, p := range f.excludePfx {
		if rel == p || strings.HasPrefix(rel, p+"/") {
			return true
		}
	}
	for _, re := range f.excludeGlob {
		if re.MatchString(rel) {
			return true
		}
	}
	return false
}

// SkipDir reports whether the directory at repo-relative path rel (basename
// base) should be pruned. A default/excluded directory is still descended if an
// Include pattern could match something beneath it.
func (f *Filter) SkipDir(rel, base string) bool {
	if f.protectsUnder(rel) {
		return false
	}
	if f.skipDirs[base] {
		return true
	}
	return f.excluded(rel)
}

// SkipFile reports whether the file at repo-relative path rel should be
// skipped. Include always wins; otherwise a file is skipped if it sits under a
// default-skip directory (reached only because an Include protected the dir) or
// matches an exclude.
func (f *Filter) SkipFile(rel string) bool {
	if f.Included(rel) {
		return false
	}
	if f.underDefaultSkipDir(rel) {
		return true
	}
	return f.excluded(rel)
}

// underDefaultSkipDir reports whether any ancestor directory of rel is a
// default-skip basename.
func (f *Filter) underDefaultSkipDir(rel string) bool {
	segs := strings.Split(rel, "/")
	for _, seg := range segs[:len(segs)-1] {
		if f.skipDirs[seg] {
			return true
		}
	}
	return false
}

// protectsUnder reports whether any Include pattern could match a path under
// dirRel, so the walk must descend into an otherwise-skipped directory.
func (f *Filter) protectsUnder(dirRel string) bool {
	for _, lit := range f.includeLits {
		if lit == "" {
			return true // pattern begins with a wildcard — could match anywhere
		}
		if dirRel == lit || strings.HasPrefix(lit, dirRel+"/") || strings.HasPrefix(dirRel, lit+"/") {
			return true
		}
	}
	return false
}

func hasGlob(p string) bool { return strings.ContainsAny(p, "*?[") }

func cleanPat(p string) string { return strings.Trim(p, "/") }

// literalPrefix returns the leading wildcard-free path portion of a glob,
// trimmed to a directory boundary — used to decide directory protection.
func literalPrefix(p string) string {
	i := strings.IndexAny(p, "*?[")
	if i < 0 {
		return cleanPat(p)
	}
	lit := p[:i]
	if j := strings.LastIndex(lit, "/"); j >= 0 {
		lit = lit[:j]
	} else {
		lit = ""
	}
	return cleanPat(lit)
}

// globToRegexp compiles a path glob into an anchored regexp. '*' matches within
// a path segment, '**' spans separators, '?' matches one non-separator char.
func globToRegexp(glob string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); {
		c := glob[i]
		switch c {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				j := i + 2
				if j < len(glob) && glob[j] == '/' {
					b.WriteString("(?:.*/)?")
					i = j + 1
				} else {
					b.WriteString(".*")
					i = j
				}
			} else {
				b.WriteString("[^/]*")
				i++
			}
		case '?':
			b.WriteString("[^/]")
			i++
		case '.', '+', '(', ')', '|', '{', '}', '^', '$', '\\', '[', ']':
			b.WriteByte('\\')
			b.WriteByte(c)
			i++
		default:
			b.WriteByte(c)
			i++
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}
