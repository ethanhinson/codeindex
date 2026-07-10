// Command codeindex is the walking-skeleton CLI: `build` indexes a Go repo into
// SQLite, and `bench` measures parse/patch throughput and proves that an
// incremental patch yields the same graph as a full rebuild.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"codeindex/internal/engine"
	"codeindex/internal/graph"
	"codeindex/internal/merkle"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: codeindex <build|bench> <repo-root> [out.json]")
		os.Exit(2)
	}
	cmd, root := os.Args[1], os.Args[2]
	switch cmd {
	case "build":
		if err := runBuild(root); err != nil {
			fatal(err)
		}
	case "bench":
		out := ""
		if len(os.Args) > 3 {
			out = os.Args[3]
		}
		if err := runBench(root, out); err != nil {
			fatal(err)
		}
	case "query", "callers":
		if len(os.Args) < 4 {
			fatal(fmt.Errorf("usage: codeindex %s <repo-root> <symbol> [--limit N]", cmd))
		}
		limit := 50
		if len(os.Args) >= 6 && os.Args[4] == "--limit" {
			fmt.Sscanf(os.Args[5], "%d", &limit)
		}
		if err := runQuery(root, os.Args[3], limit); err != nil {
			fatal(err)
		}
	case "callees":
		if len(os.Args) < 4 {
			fatal(fmt.Errorf("usage: codeindex callees <repo-root> <symbol> [--limit N]"))
		}
		limit := 50
		if len(os.Args) >= 6 && os.Args[4] == "--limit" {
			fmt.Sscanf(os.Args[5], "%d", &limit)
		}
		if err := runCallees(root, os.Args[3], limit); err != nil {
			fatal(err)
		}
	default:
		fatal(fmt.Errorf("unknown command %q", cmd))
	}
}

func dbPath(root string) string { return filepath.Join(root, ".codeindex", "graph.db") }

func ensureDBDir(root string) error {
	return os.MkdirAll(filepath.Join(root, ".codeindex"), 0o755)
}

func runBuild(root string) error {
	if err := ensureDBDir(root); err != nil {
		return err
	}
	db := dbPath(root)
	os.Remove(db)
	start := time.Now()
	st, err := engine.Build(root, db)
	if err != nil {
		return err
	}
	fmt.Printf("built %s: %d files, %d symbols in %s\n",
		db, st.FilesParsed, st.Symbols, time.Since(start).Round(time.Millisecond))
	return nil
}

// BenchResult is the machine-readable output of `bench`.
type BenchResult struct {
	Repo             string  `json:"repo"`
	Files            int     `json:"files"`
	Symbols          int     `json:"symbols"`
	Lines            int     `json:"lines"`
	ColdBuildMs      float64 `json:"cold_build_ms"`
	FilesPerSec      float64 `json:"files_per_sec"`
	LinesPerSec      float64 `json:"lines_per_sec"`
	IncrementalMs    float64 `json:"incremental_patch_ms"`
	IncrementalEqual bool    `json:"incremental_equals_full"`
	Diff             string  `json:"diff,omitempty"`
}

func runBench(root, out string) error {
	if err := ensureDBDir(root); err != nil {
		return err
	}
	db := dbPath(root)
	os.Remove(db)

	lines, err := engine.CountLines(root)
	if err != nil {
		return err
	}

	// 1) Cold build throughput.
	start := time.Now()
	st, err := engine.Build(root, db)
	if err != nil {
		return err
	}
	cold := time.Since(start)

	res := BenchResult{
		Repo: filepath.Base(root), Files: st.FilesParsed, Symbols: st.Symbols,
		Lines: lines, ColdBuildMs: ms(cold),
		FilesPerSec: perSec(st.FilesParsed, cold), LinesPerSec: perSec(lines, cold),
	}

	// 2) Single-file incremental patch latency. Mutate one file, patch, restore.
	paths, err := merkle.Walk(root)
	if err != nil {
		return err
	}
	if len(paths) > 0 {
		target := filepath.Join(root, paths[len(paths)/2])
		orig, err := os.ReadFile(target)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, append(orig, []byte("\n// codeindex-bench\n")...), 0o644); err != nil {
			return err
		}
		pstart := time.Now()
		if _, err := engine.Patch(root, db); err != nil {
			return err
		}
		res.IncrementalMs = ms(time.Since(pstart))
		_ = os.WriteFile(target, orig, 0o644) // restore

		// 3) Prove incremental == full rebuild on the real repo (post-mutation
		// state is already patched into db; full-rebuild the mutated tree).
		os.WriteFile(target, append(orig, []byte("\n// codeindex-bench\n")...), 0o644)
		full := dbPath(root) + ".full"
		os.Remove(full)
		if _, err := engine.Build(root, full); err != nil {
			return err
		}
		incSnap, err := openSnap(db)
		if err != nil {
			return err
		}
		fullSnap, err := openSnap(full)
		if err != nil {
			return err
		}
		diff := incSnap.Diff(fullSnap)
		res.IncrementalEqual = diff == ""
		res.Diff = diff
		os.Remove(full)
		_ = os.WriteFile(target, orig, 0o644) // restore clean
	}

	printBench(res)
	if out != "" {
		b, _ := json.MarshalIndent(res, "", "  ")
		if err := os.WriteFile(out, b, 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", out)
	}
	return nil
}

// runQuery prints the compact index answer for a symbol: its definition(s) and
// callers as `path:line  signature` references — the plugin's output contract.
func runQuery(root, name string, limit int) error {
	db := dbPath(root)
	st, err := graph.Open(db)
	if err != nil {
		return err
	}
	defer st.Close()

	defs, err := st.Definitions(name)
	if err != nil {
		return err
	}
	for _, d := range defs {
		fmt.Printf("def  %s:%d  %s\n", d.File, d.StartLine, d.Signature)
	}
	if len(defs) == 0 {
		fmt.Printf("def  %s: (not found in index)\n", name)
	}

	callers, err := st.Callers(name)
	if err != nil {
		return err
	}
	fmt.Printf("callers (%d):\n", len(callers))
	for i, c := range callers {
		if i >= limit {
			fmt.Printf("  ... (+%d more; use --limit)\n", len(callers)-limit)
			break
		}
		flag := ""
		if c.Conf == graph.ConfAmbiguous {
			flag = "  [ambiguous]"
		}
		fmt.Printf("  %s:%d  %s%s\n", c.File, c.Line, c.Name, flag)
	}
	return nil
}

// runCallees prints what a symbol calls: each callee as a reference to its
// definition (when resolved) plus the call-site line.
func runCallees(root, name string, limit int) error {
	st, err := graph.Open(dbPath(root))
	if err != nil {
		return err
	}
	defer st.Close()
	callees, err := st.Callees(name)
	if err != nil {
		return err
	}
	fmt.Printf("callees of %s (%d):\n", name, len(callees))
	for i, c := range callees {
		if i >= limit {
			fmt.Printf("  ... (+%d more; use --limit)\n", len(callees)-limit)
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
		fmt.Printf("  %s  -> %s  @call:%d%s\n", c.Name, target, c.CallLine, flag)
	}
	return nil
}

func openSnap(db string) (graph.Snapshot, error) {
	st, err := graph.Open(db)
	if err != nil {
		return graph.Snapshot{}, err
	}
	defer st.Close()
	return st.DumpNormalized()
}

func printBench(r BenchResult) {
	fmt.Printf("\n=== %s ===\n", r.Repo)
	fmt.Printf("  files=%d symbols=%d lines=%d\n", r.Files, r.Symbols, r.Lines)
	fmt.Printf("  cold build:   %.0f ms  (%.0f files/s, %.0f lines/s)\n",
		r.ColdBuildMs, r.FilesPerSec, r.LinesPerSec)
	fmt.Printf("  incremental:  %.1f ms (single-file patch)\n", r.IncrementalMs)
	fmt.Printf("  incremental == full rebuild: %v\n", r.IncrementalEqual)
	if !r.IncrementalEqual {
		fmt.Printf("  DIFF:\n%s\n", r.Diff)
	}
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000.0 }
func perSec(n int, d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return float64(n) / d.Seconds()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
