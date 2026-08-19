// Package workspace implements workspace member and namespace discovery for
// `codeindex init-workspace --scan`.
package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
)

// pySkipDirs are directory names that never contribute an importable
// top-level module for our purposes.
var pySkipDirs = map[string]bool{
	"test":     true,
	"tests":    true,
	"docs":     true,
	"examples": true,
	"setup":    true,
	"conftest": true,
	"noxfile":  true,
	"manage":   true,
	"_version": true,
}

// Namespaces runs every per-language probe against memberRoot and returns the
// concatenation of their results, deduplicated and sorted.
//
// A language whose marker file is absent contributes nothing and is not an
// error; members are single-language in practice, but the probes are
// independent. A marker file that exists but cannot be read or parsed IS an
// error.
// namespacesProbe is the seam every in-package caller of the namespace pass
// goes through, so a test can count how many times a member root is probed.
// Production code never reassigns it.
var namespacesProbe = Namespaces

func Namespaces(memberRoot string) ([]string, error) {
	var all []string
	for _, probe := range []func(string) ([]string, error){
		goNamespaces,
		nodeNamespaces,
		phpNamespaces,
		pythonNamespaces,
	} {
		ns, err := probe(memberRoot)
		if err != nil {
			return nil, err
		}
		all = append(all, ns...)
	}
	return dedupeSorted(all), nil
}

func dedupeSorted(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// readMarker reads a marker file, reporting absence as (nil, false, nil).
func readMarker(dir, name string) ([]byte, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// goNamespaces returns the module path declared by go.mod, parsed with
// modfile rather than a line scan: the path may be quoted or sit in a block.
func goNamespaces(root string) ([]string, error) {
	path := filepath.Join(root, "go.mod")
	data, ok, err := readMarker(root, "go.mod")
	if err != nil || !ok {
		return nil, err
	}
	f, err := modfile.Parse(path, data, nil)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if f.Module == nil {
		return nil, nil
	}
	return []string{f.Module.Mod.Path}, nil
}

// nodeNamespaces returns the package.json "name".
func nodeNamespaces(root string) ([]string, error) {
	data, ok, err := readMarker(root, "package.json")
	if err != nil || !ok {
		return nil, err
	}
	var pkg struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Join(root, "package.json"), err)
	}
	return []string{pkg.Name}, nil
}

// phpNamespaces returns the composer "name" plus every autoload.psr-4 key.
//
// The psr-4 value is string-or-array: laravel-framework maps
// `Illuminate\Support\` to three paths, so a map[string]string decode fails
// outright. Only the keys matter here, so the values are held as RawMessage
// and never interpreted.
func phpNamespaces(root string) ([]string, error) {
	data, ok, err := readMarker(root, "composer.json")
	if err != nil || !ok {
		return nil, err
	}
	var composer struct {
		Name     string `json:"name"`
		Autoload struct {
			PSR4 map[string]json.RawMessage `json:"psr-4"`
		} `json:"autoload"`
	}
	if err := json.Unmarshal(data, &composer); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Join(root, "composer.json"), err)
	}
	out := []string{composer.Name}
	for prefix, raw := range composer.Autoload.PSR4 {
		// Accept both the string and the array form; the value shape is
		// validated but discarded — only the namespace prefix is a namespace.
		trimmed := strings.TrimSpace(string(raw))
		if !strings.HasPrefix(trimmed, `"`) && !strings.HasPrefix(trimmed, "[") {
			continue
		}
		out = append(out, prefix)
	}
	return out, nil
}

// pythonNamespaces returns top-level importable module names, probing src/
// first and falling back to the member root. Both Python members of the
// frozen bench corpus are src-layout, so a root-only rule would silently
// lose them.
func pythonNamespaces(root string) ([]string, error) {
	src := filepath.Join(root, "src")
	if info, err := os.Stat(src); err == nil && info.IsDir() {
		ns, err := pythonModulesIn(src)
		if err != nil {
			return nil, err
		}
		if len(ns) > 0 {
			return ns, nil
		}
	}
	return pythonModulesIn(root)
}

// pythonModulesIn lists importable top-level modules directly under dir: a
// directory containing __init__.py, or a *.py file (module name = basename
// without extension). Dot-prefixed entries and test/tests/docs/examples are
// skipped. A nonexistent dir yields nothing and is not an error.
func pythonModulesIn(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() {
			if pySkipDirs[name] {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, name, "__init__.py")); err == nil {
				out = append(out, name)
			}
			continue
		}
		if filepath.Ext(name) != ".py" {
			continue
		}
		mod := strings.TrimSuffix(name, ".py")
		if pySkipDirs[mod] {
			continue
		}
		out = append(out, mod)
	}
	return out, nil
}
