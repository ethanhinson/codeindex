package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeindex/internal/config"

	"golang.org/x/mod/modfile"
	"gopkg.in/yaml.v3"
)

// memberMarkers are the files whose presence at a candidate's own top level
// qualifies it as a member (guard 2). The Python entries mirror the marker
// files a src-layout or flat-layout project roots itself with; namespaces.go
// derives Python namespaces from the tree, not from these, so they are listed
// here rather than shared.
var memberMarkers = []string{
	"go.mod",
	"package.json",
	"composer.json",
	"pyproject.toml",
	"setup.py",
	"setup.cfg",
}

// excludedDirBasenames is DefaultExcludeDirs as a set, for guard 1's
// traversal test.
var excludedDirBasenames = func() map[string]bool {
	m := make(map[string]bool, len(config.DefaultExcludeDirs))
	for _, d := range config.DefaultExcludeDirs {
		m[d] = true
	}
	return m
}()

// Members discovers a monorepo's declared members under root.
//
// Five declaration sources are read, in this order: go.work, pnpm-workspace.yaml,
// composer.json path repositories, lerna.json, and package.json workspaces
// (array form and the yarn object form). A source that is absent contributes
// nothing and is not an error; a source that exists but does not parse IS an
// error, matching Namespaces' posture.
//
// Each source yields glob-ish patterns which are expanded with filepath.Glob
// relative to root and then filtered by three over-collection guards, in
// order: directories-only (plus explicit DefaultExcludeDirs traversal),
// marker-at-the-candidate's-own-top-level, and subset suppression. A glob is
// not the repo filter, so the exclusion is applied here explicitly.
//
// Returned roots are slash-separated and relative to root; ids are derived
// from the candidate basename. Callers that merge into an existing manifest
// keep authored ids — id derivation here applies only to members the scan
// introduces.
func Members(root string) ([]config.Member, error) {
	var patterns []string
	for _, source := range []func(string) ([]string, error){
		goWorkPatterns,
		pnpmPatterns,
		composerPathPatterns,
		lernaPatterns,
		packageJSONWorkspacePatterns,
	} {
		p, err := source(root)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, p...)
	}

	candidates, err := expand(root, patterns)
	if err != nil {
		return nil, err
	}

	// Guards 1 and 2 are per-candidate; guard 3 compares candidates, so it
	// runs once the surviving set is known.
	type kept struct {
		rel        string
		namespaces []string
	}
	var survivors []kept
	for _, rel := range candidates {
		ok, err := isMemberDir(root, rel)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		ns, err := Namespaces(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		survivors = append(survivors, kept{rel: rel, namespaces: ns})
	}

	usedIDs := map[string]bool{}
	out := make([]config.Member, 0, len(survivors))
	for i, c := range survivors {
		suppressed := false
		for j := range survivors {
			if j != i && isStrictSubset(c.namespaces, survivors[j].namespaces) {
				suppressed = true
				break
			}
		}
		if suppressed {
			continue
		}
		out = append(out, config.Member{
			ID:         uniqueID(memberID(c.rel), usedIDs),
			Root:       c.rel,
			Namespaces: c.namespaces,
		})
	}
	return out, nil
}

// expand turns patterns into deduplicated slash-relative candidate paths in
// expansion order. Patterns are matched relative to root; a pattern that
// climbs out of root, or matches nothing, contributes nothing.
func expand(root string, patterns []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, pat := range patterns {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pat)))
		if err != nil {
			return nil, fmt.Errorf("expand %q: %w", pat, err)
		}
		for _, m := range matches {
			rel, err := filepath.Rel(root, m)
			if err != nil {
				continue
			}
			rel = filepath.ToSlash(rel)
			if rel == "." || strings.HasPrefix(rel, "../") {
				continue
			}
			if seen[rel] {
				continue
			}
			seen[rel] = true
			out = append(out, rel)
		}
	}
	return out, nil
}

// isMemberDir applies guards 1 and 2 to one candidate.
func isMemberDir(root, rel string) (bool, error) {
	// Guard 1a: a glob hit that is not a directory is not a member.
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return false, nil
	}
	if !info.IsDir() {
		return false, nil
	}
	// Guard 1b: no candidate may traverse an excluded basename.
	for _, seg := range strings.Split(rel, "/") {
		if excludedDirBasenames[seg] {
			return false, nil
		}
	}
	// Guard 2: the marker must sit at the candidate's own top level. A
	// marker one level down qualifies that child, not this candidate.
	for _, marker := range memberMarkers {
		_, ok, err := readMarker(filepath.Join(root, filepath.FromSlash(rel)), marker)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// isStrictSubset is guard 3's test: a candidate whose namespace set is a
// strict subset of another candidate's is dropped, which is what removes a
// monorepo's own root package when it re-exports its children. Strictness
// matters — an equal-set test would drop both halves of a duplicate pair and
// leave the monorepo with no member at all. Namespaces arrive deduplicated
// from Namespaces, so a shorter length is a sound strictness proxy.
func isStrictSubset(sub, super []string) bool {
	if len(sub) >= len(super) {
		return false
	}
	have := make(map[string]bool, len(super))
	for _, s := range super {
		have[s] = true
	}
	for _, s := range sub {
		if !have[s] {
			return false
		}
	}
	return true
}

// memberID sanitizes a candidate's basename to [A-Za-z0-9._-], mapping every
// other rune to '-', and lowercases it.
func memberID(rel string) string {
	base := filepath.Base(filepath.FromSlash(rel))
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	id := b.String()
	if id == "" {
		return "member"
	}
	return id
}

// uniqueID suffixes -2, -3, … on collision, in expansion order.
func uniqueID(id string, used map[string]bool) string {
	candidate := id
	for n := 2; used[candidate]; n++ {
		candidate = fmt.Sprintf("%s-%d", id, n)
	}
	used[candidate] = true
	return candidate
}

// --- declaration sources ---

// goWorkPatterns returns the go.work use directives. Parsed with modfile
// rather than a line scan: use paths may be quoted or sit in a block.
func goWorkPatterns(root string) ([]string, error) {
	path := filepath.Join(root, "go.work")
	data, ok, err := readMarker(root, "go.work")
	if err != nil || !ok {
		return nil, err
	}
	f, err := modfile.ParseWork(path, data, nil)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var out []string
	for _, u := range f.Use {
		if u == nil || u.Path == "" {
			continue
		}
		out = append(out, u.Path)
	}
	return out, nil
}

// pnpmPatterns returns pnpm-workspace.yaml's packages list.
func pnpmPatterns(root string) ([]string, error) {
	data, ok, err := readMarker(root, "pnpm-workspace.yaml")
	if err != nil || !ok {
		return nil, err
	}
	var ws struct {
		Packages []string `yaml:"packages"`
	}
	if err := yaml.Unmarshal(data, &ws); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Join(root, "pnpm-workspace.yaml"), err)
	}
	return ws.Packages, nil
}

// composerPathPatterns returns the url of every repositories[] entry whose
// type is "path"; other repository types are not local members.
func composerPathPatterns(root string) ([]string, error) {
	data, ok, err := readMarker(root, "composer.json")
	if err != nil || !ok {
		return nil, err
	}
	var composer struct {
		Repositories []struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"repositories"`
	}
	if err := json.Unmarshal(data, &composer); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Join(root, "composer.json"), err)
	}
	var out []string
	for _, r := range composer.Repositories {
		if r.Type == "path" && r.URL != "" {
			out = append(out, r.URL)
		}
	}
	return out, nil
}

// lernaPatterns returns lerna.json's packages list. The corpus's only
// monorepo (nest) declares its members solely here.
func lernaPatterns(root string) ([]string, error) {
	data, ok, err := readMarker(root, "lerna.json")
	if err != nil || !ok {
		return nil, err
	}
	var lerna struct {
		Packages []string `json:"packages"`
	}
	if err := json.Unmarshal(data, &lerna); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Join(root, "lerna.json"), err)
	}
	return lerna.Packages, nil
}

// packageJSONWorkspacePatterns returns package.json's workspaces, accepting
// both the array form and yarn's object form {"packages": [...]}. The field
// is held as RawMessage because the two shapes cannot share a decode target.
func packageJSONWorkspacePatterns(root string) ([]string, error) {
	data, ok, err := readMarker(root, "package.json")
	if err != nil || !ok {
		return nil, err
	}
	path := filepath.Join(root, "package.json")
	var pkg struct {
		Workspaces json.RawMessage `json:"workspaces"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(pkg.Workspaces) == 0 {
		return nil, nil
	}
	var asArray []string
	if err := json.Unmarshal(pkg.Workspaces, &asArray); err == nil {
		return asArray, nil
	}
	var asObject struct {
		Packages []string `json:"packages"`
	}
	if err := json.Unmarshal(pkg.Workspaces, &asObject); err != nil {
		return nil, fmt.Errorf("parse %s: workspaces is neither an array nor an object: %w", path, err)
	}
	return asObject.Packages, nil
}
