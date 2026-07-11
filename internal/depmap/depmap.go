// Package depmap builds and attaches versioned, symbols-only dependency maps:
// per-namespace@version artifacts (definitions + per-file hashes, no edges)
// that give project calls into dependencies real resolution targets, while
// hash verification lets locally modified deps overlay the static map.
package depmap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeindex/internal/adapter"
	"codeindex/internal/config"
	"codeindex/internal/graph"
	"codeindex/internal/merkle"
)

// Generate builds a map file from a dependency source directory.
func Generate(depDir, namespace, version, outPath string) (files, symbols int, err error) {
	os.Remove(outPath)
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return 0, 0, err
	}
	st, err := graph.Open(outPath)
	if err != nil {
		return 0, 0, err
	}
	defer st.Close()
	if err := st.WriteDepMeta(namespace, version); err != nil {
		return 0, 0, err
	}

	// A dep tree carries its own associations (a vendored Drupal module
	// needs *.module routed to PHP when mapped, same as a project would).
	cfg, err := config.Load(depDir)
	if err != nil {
		return 0, 0, err
	}
	if err := adapter.SetAssociations(cfg.Associations); err != nil {
		return 0, 0, err
	}
	paths, err := merkle.Walk(depDir)
	if err != nil {
		return 0, 0, err
	}
	for _, rel := range paths {
		full := filepath.Join(depDir, rel)
		src, err := os.ReadFile(full)
		if err != nil {
			return 0, 0, err
		}
		a := adapter.For(rel)
		if a == nil {
			continue
		}
		pf, err := a.Parse(rel, src)
		if err != nil {
			continue // a broken dep file is skipped, not fatal
		}
		hash, size, mtime, err := graph.HashFile(full)
		if err != nil {
			return 0, 0, err
		}
		// Symbols only — calls/deps are dropped (dep internals are not our
		// refactoring surface, and dep symbols must never source edges).
		if err := st.PutDepSymbols(namespace, version, graph.CleanRel(rel), hash, size, mtime, pf.Symbols); err != nil {
			return 0, 0, err
		}
		files++
		symbols += len(pf.Symbols)
	}
	return files, symbols, nil
}

// CachePath is the per-namespace@version cache location for generated maps.
func CachePath(namespace, version string) string {
	home, _ := os.UserHomeDir()
	safe := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(namespace)
	return filepath.Join(home, ".codeindex", "depmaps", safe+"@"+version+".db")
}

// GenerateCached generates (or reuses) the cached map for a dep.
func GenerateCached(depDir, namespace, version string) (string, error) {
	out := CachePath(namespace, version)
	if _, err := os.Stat(out); err == nil {
		return out, nil // version-keyed cache hit
	}
	if _, _, err := Generate(depDir, namespace, version, out); err != nil {
		return "", err
	}
	return out, nil
}

// Attach imports a map into a repo index at prefix and re-resolves affected
// edges (unresolved calls into the dep upgrade to dep-resolved).
func Attach(repoDB, mapPath, prefix string) (ns, ver string, err error) {
	st, err := graph.Open(repoDB)
	if err != nil {
		return "", "", err
	}
	defer st.Close()
	ns, ver, names, err := st.AttachMap(mapPath, graph.CleanRel(prefix))
	if err != nil {
		return "", "", err
	}
	return ns, ver, st.ReResolve(names)
}

// AutoModule is one dependency discovered from vendor metadata.
type AutoModule struct {
	Namespace string
	Version   string
	Dir       string // repo-relative dependency root
}

// DiscoverGoVendor parses vendor/modules.txt ("# <module> <version>").
func DiscoverGoVendor(repo string) ([]AutoModule, error) {
	b, err := os.ReadFile(filepath.Join(repo, "vendor", "modules.txt"))
	if err != nil {
		return nil, err
	}
	var out []AutoModule
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, "# ") {
			continue
		}
		fields := strings.Fields(line[2:])
		if len(fields) < 2 || !strings.HasPrefix(fields[1], "v") {
			continue
		}
		dir := filepath.Join("vendor", fields[0])
		if fi, err := os.Stat(filepath.Join(repo, dir)); err == nil && fi.IsDir() {
			out = append(out, AutoModule{Namespace: fields[0], Version: fields[1], Dir: dir})
		}
	}
	return out, nil
}

// DiscoverComposer parses composer.lock packages against vendor/<name>.
func DiscoverComposer(repo string) ([]AutoModule, error) {
	b, err := os.ReadFile(filepath.Join(repo, "composer.lock"))
	if err != nil {
		return nil, err
	}
	var lock struct {
		Packages []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(b, &lock); err != nil {
		return nil, err
	}
	var out []AutoModule
	for _, p := range lock.Packages {
		dir := filepath.Join("vendor", p.Name)
		if fi, err := os.Stat(filepath.Join(repo, dir)); err == nil && fi.IsDir() {
			out = append(out, AutoModule{Namespace: p.Name, Version: p.Version, Dir: dir})
		}
	}
	return out, nil
}

// AutoAttach discovers vendor metadata (Go modules.txt, composer.lock),
// generates/reuses cached maps, and attaches them all.
func AutoAttach(repo, repoDB string) (attached, totalSyms int, err error) {
	var mods []AutoModule
	if gm, err := DiscoverGoVendor(repo); err == nil {
		mods = append(mods, gm...)
	}
	if cm, err := DiscoverComposer(repo); err == nil {
		mods = append(mods, cm...)
	}
	if len(mods) == 0 {
		return 0, 0, fmt.Errorf("no vendor metadata found (vendor/modules.txt or composer.lock)")
	}
	st, err := graph.Open(repoDB)
	if err != nil {
		return 0, 0, err
	}
	defer st.Close()
	// Attach all maps first, then re-resolve affected names ONCE — per-module
	// re-resolution multiplies UPDATE passes over a large edges table.
	nameSet := map[string]struct{}{}
	for _, m := range mods {
		mapPath, err := GenerateCached(filepath.Join(repo, m.Dir), m.Namespace, m.Version)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ! %s@%s: %v (skipped)\n", m.Namespace, m.Version, err)
			continue
		}
		_, _, names, err := st.AttachMap(mapPath, graph.CleanRel(m.Dir))
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ! attach %s: %v (skipped)\n", m.Namespace, err)
			continue
		}
		for _, n := range names {
			nameSet[n] = struct{}{}
		}
		attached++
	}
	all := make([]string, 0, len(nameSet))
	for n := range nameSet {
		all = append(all, n)
	}
	if err := st.ReResolve(all); err != nil {
		return attached, 0, err
	}
	totalSyms, err = st.DepSymbolCount()
	return attached, totalSyms, err
}

// VerifyOverlay checks covered files against their recorded state and
// overlays locally modified ones (the hacked-dep case). Restores are handled
// identically: content returning to the map hash re-parses to map-equivalent
// rows and clears the modified flag. Skips silently above the file threshold
// (verification then happens only at attach/build — documented).
const verifyThreshold = 25000

func VerifyOverlay(repo, repoDB string) error {
	st, err := graph.Open(repoDB)
	if err != nil {
		return err
	}
	defer st.Close()
	covered, err := st.DepFiles()
	if err != nil || len(covered) == 0 || len(covered) > verifyThreshold {
		return err
	}
	var affected []string
	for _, d := range covered {
		full := filepath.Join(repo, d.Path)
		fi, err := os.Stat(full)
		if err != nil {
			continue // deleted dep file: leave map rows (restore via git)
		}
		if fi.Size() == d.Size && fi.ModTime().UnixNano() == d.Mtime {
			continue // fast path: unchanged
		}
		hash, size, mtime, err := graph.HashFile(full)
		if err != nil {
			continue
		}
		if hash == d.CurHash {
			continue // touched but identical
		}
		src, _ := os.ReadFile(full)
		a := adapter.For(d.Path)
		if a == nil {
			// Odd-extension dep file (hacked .module etc): sniff its head.
			if len(src) > 0 {
				a = adapter.ForName(adapter.SniffLang(src[:min(len(src), 1024)]))
			}
		}
		if a == nil {
			continue
		}
		pf, err := a.Parse(d.Path, src)
		if err != nil {
			continue
		}
		names, err := st.OverlayDepFile(d, hash, size, mtime, pf.Symbols)
		if err != nil {
			return err
		}
		affected = append(affected, names...)
	}
	return st.ReResolve(affected)
}
