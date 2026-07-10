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

// CallersText returns definitions + callers of name as reference lines.
func CallersText(root, name string, limit int) (string, error) {
	st, err := open(root)
	if err != nil {
		return "", err
	}
	defer st.Close()
	var b strings.Builder

	defs, err := st.Definitions(name)
	if err != nil {
		return "", err
	}
	for _, d := range defs {
		fmt.Fprintf(&b, "def  %s:%d  %s\n", d.File, d.StartLine, d.Signature)
	}
	if len(defs) == 0 {
		fmt.Fprintf(&b, "def  %s: (not found in index)\n", name)
	}

	callers, err := st.Callers(name)
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
		fmt.Fprintf(&b, "  %s:%d  %s%s\n", c.File, c.Line, c.Name, flag)
	}
	return b.String(), nil
}

// CalleesText returns what name calls, each resolved to its definition.
func CalleesText(root, name string, limit int) (string, error) {
	st, err := open(root)
	if err != nil {
		return "", err
	}
	defer st.Close()
	callees, err := st.Callees(name)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "callees of %s (%d):\n", name, len(callees))
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
		fmt.Fprintf(&b, "  %s  -> %s  @call:%d%s\n", c.Name, target, c.CallLine, flag)
	}
	return b.String(), nil
}

// ImpactText composes a counts-first blast-radius summary: definitions,
// callers, callees. States coverage honestly (call edges only).
func ImpactText(root, name string, limit int) (string, error) {
	st, err := open(root)
	if err != nil {
		return "", err
	}
	defer st.Close()

	defs, err := st.Definitions(name)
	if err != nil {
		return "", err
	}
	callers, err := st.Callers(name)
	if err != nil {
		return "", err
	}
	callees, err := st.Callees(name)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "impact of %s: %d definition(s), %d caller(s), %d callee(s)\n",
		name, len(defs), len(callers), len(callees))
	fmt.Fprintf(&b, "(coverage: call edges only — import/type dependents not included)\n\n")
	for _, d := range defs {
		fmt.Fprintf(&b, "def  %s:%d  %s\n", d.File, d.StartLine, d.Signature)
	}
	if len(defs) == 0 {
		fmt.Fprintf(&b, "def  %s: (not found in index)\n", name)
	}
	fmt.Fprintf(&b, "\ncallers — these break if %s's behavior/signature changes:\n", name)
	for i, c := range callers {
		if i >= limit {
			fmt.Fprintf(&b, "  ... (+%d more)\n", len(callers)-limit)
			break
		}
		flag := ""
		if c.Conf == graph.ConfAmbiguous {
			flag = "  [ambiguous]"
		}
		fmt.Fprintf(&b, "  %s:%d  %s%s\n", c.File, c.Line, c.Name, flag)
	}
	fmt.Fprintf(&b, "\ncallees — what %s depends on:\n", name)
	for i, c := range callees {
		if i >= limit {
			fmt.Fprintf(&b, "  ... (+%d more)\n", len(callees)-limit)
			break
		}
		if c.DefFile == "" {
			continue // unresolved (stdlib/external) — noise in an impact summary
		}
		fmt.Fprintf(&b, "  %s  %s:%d\n", c.Name, c.DefFile, c.DefLine)
	}
	return b.String(), nil
}
