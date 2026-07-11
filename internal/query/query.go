// Package query is the shared query surface for the CLI and the MCP server:
// fresh-on-query behavior, formatted reference-based answers, and in-process
// serialization of index writes (SQLite is single-writer; the MCP server is
// long-lived and may receive concurrent tool calls).
package query

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"codeindex/internal/engine"
	"codeindex/internal/graph"
)

// mu serializes ensureFresh (the only index-mutating step) across concurrent
// callers. Queries after the patch are read-only and safe.
var mu sync.Mutex

func dbPath(root string) string { return filepath.Join(root, ".codeindex", "graph.db") }

// SplitAnchor parses a possibly-qualified anchor: "Type.method" or
// "Type::method" -> (method, Type); bare "name" -> (name, "").
func SplitAnchor(anchor string) (name, parent string) {
	if i := strings.Index(anchor, "::"); i > 0 {
		return anchor[i+2:], anchor[:i]
	}
	if i := strings.LastIndex(anchor, "."); i > 0 {
		return anchor[i+1:], anchor[:i]
	}
	return anchor, ""
}

// Fresh makes the index reflect the working tree: builds if missing, patches
// otherwise. Safe for concurrent use.
func Fresh(root string) error {
	mu.Lock()
	defer mu.Unlock()
	if err := os.MkdirAll(filepath.Join(root, ".codeindex"), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(dbPath(root)); os.IsNotExist(err) {
		_, err := engine.Build(root, dbPath(root))
		return err
	}
	_, err := engine.Patch(root, dbPath(root))
	return err
}

func open(root string) (*graph.Store, error) {
	if err := Fresh(root); err != nil {
		return nil, err
	}
	return graph.Open(dbPath(root))
}

// CallersText returns definitions + callers of an anchor ("name" or
// "Type.method"/"Type::method") as reference lines with qualified names.
func CallersText(root, anchor string, limit int) (string, error) {
	name, parent := SplitAnchor(anchor)
	st, err := open(root)
	if err != nil {
		return "", err
	}
	defer st.Close()
	var b strings.Builder

	defs, err := st.Definitions(name, parent)
	if err != nil {
		return "", err
	}
	for _, d := range defs {
		fmt.Fprintf(&b, "def  %s  %s:%d  %s\n", d.QName(), d.File, d.StartLine, d.Signature)
	}
	if len(defs) == 0 {
		fmt.Fprintf(&b, "def  %s: (not found in index)\n", anchor)
	}

	callers, err := st.Callers(name, parent)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&b, "callers (%d):\n", len(callers))
	for i, c := range callers {
		if i >= limit {
			fmt.Fprintf(&b, "  ... (+%d more; raise limit)\n", len(callers)-limit)
			break
		}
		flag := ""
		if c.Conf == graph.ConfAmbiguous {
			flag = "  [ambiguous]"
		}
		fmt.Fprintf(&b, "  %s:%d  %s%s\n", c.File, c.Line, c.QName(), flag)
	}
	return b.String(), nil
}

// CalleesText returns what name calls, each resolved to its definition.
func CalleesText(root, anchor string, limit int) (string, error) {
	name, parent := SplitAnchor(anchor)
	st, err := open(root)
	if err != nil {
		return "", err
	}
	defer st.Close()
	callees, err := st.Callees(name, parent)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "callees of %s (%d):\n", anchor, len(callees))
	for i, c := range callees {
		if i >= limit {
			fmt.Fprintf(&b, "  ... (+%d more; raise limit)\n", len(callees)-limit)
			break
		}
		target := "unresolved"
		if c.DefFile != "" {
			target = fmt.Sprintf("%s:%d", c.DefFile, c.DefLine)
		}
		flag := ""
		if c.Conf == graph.ConfAmbiguous {
			flag = "  [ambiguous]"
		}
		fmt.Fprintf(&b, "  %s  -> %s  @call:%d%s\n", c.QName(), target, c.CallLine, flag)
	}
	return b.String(), nil
}

// DependentsText returns who imports/extends/implements the anchor.
func DependentsText(root, anchor string, limit int) (string, error) {
	name, _ := SplitAnchor(anchor)
	st, err := open(root)
	if err != nil {
		return "", err
	}
	defer st.Close()
	deps, err := st.Dependents(name)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "dependents of %s (%d):\n", anchor, len(deps))
	for i, d := range deps {
		if i >= limit {
			fmt.Fprintf(&b, "  ... (+%d more; raise limit)\n", len(deps)-limit)
			break
		}
		fmt.Fprintf(&b, "  %-10s %s:%d  %s\n", d.Kind, d.File, d.Line, d.QName())
	}
	return b.String(), nil
}

// DepsText returns what the anchor depends on. File anchors list the file's
// imports; symbol anchors list extends/implements (+ the file's imports, labeled).
func DepsText(root, anchor string, limit int) (string, error) {
	st, err := open(root)
	if err != nil {
		return "", err
	}
	defer st.Close()

	var b strings.Builder
	emit := func(label string, ds []graph.Dep) {
		fmt.Fprintf(&b, "%s (%d):\n", label, len(ds))
		for i, d := range ds {
			if i >= limit {
				fmt.Fprintf(&b, "  ... (+%d more; raise limit)\n", len(ds)-limit)
				break
			}
			target := d.Target
			if d.DefFile != "" {
				target = fmt.Sprintf("%s (%s:%d)", d.Target, d.DefFile, d.DefLine)
			}
			fmt.Fprintf(&b, "  %-10s %s  @%d\n", d.Kind, target, d.Line)
		}
	}

	if ok, err := st.HasFile(anchor); err != nil {
		return "", err
	} else if ok {
		imports, err := st.FileImports(anchor)
		if err != nil {
			return "", err
		}
		emit("imports of "+anchor, imports)
		return b.String(), nil
	}

	name, parent := SplitAnchor(anchor)
	sd, err := st.SymbolDeps(name, parent)
	if err != nil {
		return "", err
	}
	emit("deps of "+anchor, sd)
	// context: the defining file's imports
	defs, err := st.Definitions(name, parent)
	if err != nil {
		return "", err
	}
	if len(defs) > 0 {
		imports, err := st.FileImports(defs[0].File)
		if err != nil {
			return "", err
		}
		emit("file imports ("+defs[0].File+")", imports)
	}
	return b.String(), nil
}

// ImpactText composes a counts-first blast-radius summary: definitions,
// callers, callees, dependents. States coverage honestly.
func ImpactText(root, anchor string, limit int) (string, error) {
	name, parent := SplitAnchor(anchor)
	st, err := open(root)
	if err != nil {
		return "", err
	}
	defer st.Close()

	defs, err := st.Definitions(name, parent)
	if err != nil {
		return "", err
	}
	callers, err := st.Callers(name, parent)
	if err != nil {
		return "", err
	}
	callees, err := st.Callees(name, parent)
	if err != nil {
		return "", err
	}

	dependents, err := st.Dependents(name)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "impact of %s: %d definition(s), %d caller(s), %d callee(s), %d dependent(s)\n",
		anchor, len(defs), len(callers), len(callees), len(dependents))
	fmt.Fprintf(&b, "(coverage: call + import/extends/implements edges; type-usage references not included)\n\n")
	for _, d := range defs {
		fmt.Fprintf(&b, "def  %s  %s:%d  %s\n", d.QName(), d.File, d.StartLine, d.Signature)
	}
	if len(defs) == 0 {
		fmt.Fprintf(&b, "def  %s: (not found in index)\n", anchor)
	}
	fmt.Fprintf(&b, "\ncallers — these break if %s's behavior/signature changes:\n", anchor)
	for i, c := range callers {
		if i >= limit {
			fmt.Fprintf(&b, "  ... (+%d more)\n", len(callers)-limit)
			break
		}
		flag := ""
		if c.Conf == graph.ConfAmbiguous {
			flag = "  [ambiguous]"
		}
		fmt.Fprintf(&b, "  %s:%d  %s%s\n", c.File, c.Line, c.QName(), flag)
	}
	fmt.Fprintf(&b, "\ndependents — who imports/extends/implements %s:\n", anchor)
	for i, d := range dependents {
		if i >= limit {
			fmt.Fprintf(&b, "  ... (+%d more)\n", len(dependents)-limit)
			break
		}
		fmt.Fprintf(&b, "  %-10s %s:%d  %s\n", d.Kind, d.File, d.Line, d.QName())
	}
	fmt.Fprintf(&b, "\ncallees — what %s depends on:\n", anchor)
	for i, c := range callees {
		if i >= limit {
			fmt.Fprintf(&b, "  ... (+%d more)\n", len(callees)-limit)
			break
		}
		if c.DefFile == "" {
			continue // unresolved (stdlib/external) — noise in an impact summary
		}
		fmt.Fprintf(&b, "  %s  %s:%d\n", c.QName(), c.DefFile, c.DefLine)
	}
	return b.String(), nil
}
