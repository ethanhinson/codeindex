// Command codeindex is the call-graph index CLI: `build` indexes a Go repo into
// SQLite; `callers`/`callees`/`impact`/`enclosing` answer branch-out questions
// with always-fresh reference-based output; `mcp` serves the same queries to
// IDE clients over stdio; `bench` measures throughput and proves incremental
// patches equal a full rebuild.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"codeindex/internal/depmap"
	"codeindex/internal/engine"
	"codeindex/internal/graph"
	"codeindex/internal/mcpserver"
	"codeindex/internal/merkle"
	"codeindex/internal/progress"
	"codeindex/internal/query"
)

const version = "0.2.0"

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr,
			"usage: codeindex <build|refresh|status|callers|callees|impact|dependents|deps|find|grep|tree|depmap|attach|export|import|enclosing|mcp|bench> <repo-root> ...")
		os.Exit(2)
	}
	cmd, root := os.Args[1], os.Args[2]
	switch cmd {
	case "build":
		if err := runBuild(root, hasFlag("--progress")); err != nil {
			fatal(err)
		}
	case "refresh":
		if err := runRefresh(root, hasFlag("--progress")); err != nil {
			fatal(err)
		}
	case "status":
		if err := runStatus(root, hasFlag("--json")); err != nil {
			fatal(err)
		}
	case "export":
		if len(os.Args) < 4 {
			fatal(fmt.Errorf("usage: codeindex export <repo-root> <out.db>"))
		}
		rep, done := reporter("export "+filepath.Base(root), hasFlag("--progress"))
		st, err := engine.Export(root, os.Args[3], rep)
		if err != nil {
			fatal(err)
		}
		done(fmt.Sprintf("exported %s (freshened: %d files, %d symbols)",
			os.Args[3], st.FilesParsed, st.Symbols))
	case "import":
		if len(os.Args) < 4 {
			fatal(fmt.Errorf("usage: codeindex import <repo-root> <artifact.db>"))
		}
		rep, done := reporter("import "+filepath.Base(root), hasFlag("--progress"))
		st, err := engine.Import(root, os.Args[3], rep)
		if err != nil {
			fatal(err)
		}
		done(fmt.Sprintf("imported %s; drift: %d files re-parsed, %d deleted, %d symbols",
			os.Args[3], st.FilesParsed, st.Deleted, st.Symbols))
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
	case "impact":
		if len(os.Args) < 4 {
			fatal(fmt.Errorf("usage: codeindex impact <repo-root> <symbol> [--limit N]"))
		}
		limit := 50
		if len(os.Args) >= 6 && os.Args[4] == "--limit" {
			fmt.Sscanf(os.Args[5], "%d", &limit)
		}
		if err := runImpact(root, os.Args[3], limit); err != nil {
			fatal(err)
		}
	case "dependents", "deps":
		if len(os.Args) < 4 {
			fatal(fmt.Errorf("usage: codeindex %s <repo-root> <anchor> [--limit N]", cmd))
		}
		limit := 50
		if len(os.Args) >= 6 && os.Args[4] == "--limit" {
			fmt.Sscanf(os.Args[5], "%d", &limit)
		}
		var out string
		var err error
		if cmd == "dependents" {
			out, err = query.DependentsText(root, os.Args[3], limit)
		} else {
			out, err = query.DepsText(root, os.Args[3], limit)
		}
		if err != nil {
			fatal(err)
		}
		fmt.Print(out)
	case "depmap":
		// codeindex depmap <dir> --namespace <ns> --version <v> -o <out.db>
		var ns, ver, out string
		for i := 3; i < len(os.Args)-1; i++ {
			switch os.Args[i] {
			case "--namespace":
				ns = os.Args[i+1]
			case "--version":
				ver = os.Args[i+1]
			case "-o":
				out = os.Args[i+1]
			}
		}
		if ns == "" || ver == "" || out == "" {
			fatal(fmt.Errorf("usage: codeindex depmap <dir> --namespace <ns> --version <v> -o <out.db>"))
		}
		nf, nsym, err := depmap.Generate(root, ns, ver, out)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("depmap %s@%s: %d files, %d symbols -> %s\n", ns, ver, nf, nsym, out)
	case "attach":
		// codeindex attach <repo> <map.db> --prefix <dir> | codeindex attach <repo> --auto
		if _, err := query.Fresh(root); err != nil {
			fatal(err)
		}
		if len(os.Args) > 3 && os.Args[3] == "--auto" {
			n, syms, err := depmap.AutoAttach(root, dbPath(root))
			if err != nil {
				fatal(err)
			}
			fmt.Printf("attached %d dependency maps (%d dep symbols total)\n", n, syms)
		} else {
			if len(os.Args) < 4 {
				fatal(fmt.Errorf("usage: codeindex attach <repo> <map.db> [--prefix <dir>] | --auto"))
			}
			prefix := ""
			for i := 4; i < len(os.Args)-1; i++ {
				if os.Args[i] == "--prefix" {
					prefix = os.Args[i+1]
				}
			}
			ns, ver, err := depmap.Attach(dbPath(root), os.Args[3], prefix)
			if err != nil {
				fatal(err)
			}
			fmt.Printf("attached %s@%s\n", ns, ver)
		}
	case "find":
		if len(os.Args) < 4 {
			fatal(fmt.Errorf("usage: codeindex find <repo-root> <query> [--kind k] [--path p] [--limit N]"))
		}
		kind, path := "", ""
		limit := 20
		for i := 4; i < len(os.Args)-1; i++ {
			switch os.Args[i] {
			case "--kind":
				kind = os.Args[i+1]
			case "--path":
				path = os.Args[i+1]
			case "--limit":
				fmt.Sscanf(os.Args[i+1], "%d", &limit)
			}
		}
		out, err := query.FindText(root, os.Args[3], kind, path, limit)
		if err != nil {
			fatal(err)
		}
		fmt.Print(out)
	case "grep":
		if len(os.Args) < 4 {
			fatal(fmt.Errorf("usage: codeindex grep <repo-root> <pattern> [--limit N]"))
		}
		limit := 30
		if len(os.Args) >= 6 && os.Args[4] == "--limit" {
			fmt.Sscanf(os.Args[5], "%d", &limit)
		}
		out, err := query.GrepText(root, os.Args[3], limit)
		if err != nil {
			fatal(err)
		}
		fmt.Print(out)
	case "mcp":
		if err := mcpserver.Run(context.Background(), root, version); err != nil {
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
	case "tree":
		if err := runTree(root); err != nil {
			fatal(err)
		}
	case "enclosing":
		// codeindex enclosing <repo-root> <file> <start>:<end>
		if len(os.Args) < 5 {
			fatal(fmt.Errorf("usage: codeindex enclosing <repo-root> <file> <start>:<end>"))
		}
		var start, end int
		if _, err := fmt.Sscanf(os.Args[4], "%d:%d", &start, &end); err != nil {
			fatal(fmt.Errorf("bad range %q (want start:end): %w", os.Args[4], err))
		}
		if err := runEnclosing(root, os.Args[3], start, end); err != nil {
			fatal(err)
		}
	default:
		fatal(fmt.Errorf("unknown command %q", cmd))
	}
	if info, ok := query.ConsumeColdBuild(); ok {
		fmt.Fprintf(os.Stderr,
			"\n[codeindex: indexed %d files (%d symbols) in %s — first query on this repo; subsequent queries are fast]\n",
			info.FilesParsed, info.Symbols, info.Duration.Round(time.Millisecond))
	}
}

// hasFlag reports whether a bare flag appears anywhere in the args.
func hasFlag(f string) bool {
	for _, a := range os.Args[3:] {
		if a == f {
			return true
		}
	}
	return false
}

// reporter picks the progress surface: JSONL on stdout when --progress was
// given (machine feed owns stdout; summary goes to stderr), a live TTY
// renderer when stderr is interactive, throttled plain lines otherwise.
// The returned finish func prints the final summary on the right stream.
func reporter(label string, jsonl bool) (progress.Reporter, func(string)) {
	switch {
	case jsonl:
		r := progress.NewJSONL(os.Stdout)
		return r, func(sum string) { r.Finish(sum); fmt.Fprintln(os.Stderr, sum) }
	case progress.IsTTY(os.Stderr):
		r := progress.NewTTY(os.Stderr, label)
		return r, func(sum string) { r.Finish(sum) }
	default:
		r := progress.NewPlain(os.Stderr, label)
		return r, func(sum string) { r.Finish(sum) }
	}
}

func dbPath(root string) string { return filepath.Join(root, ".codeindex", "graph.db") }

func ensureDBDir(root string) error {
	return os.MkdirAll(filepath.Join(root, ".codeindex"), 0o755)
}

func runBuild(root string, jsonl bool) error {
	if err := ensureDBDir(root); err != nil {
		return err
	}
	db := dbPath(root)
	os.Remove(db)
	rep, done := reporter("index "+filepath.Base(mustAbs(root)), jsonl)
	st, err := engine.BuildWithProgress(root, db, rep)
	if err != nil {
		return err
	}
	done(fmt.Sprintf("indexed %d files (%d symbols)", st.FilesParsed, st.Symbols))
	return nil
}

// runRefresh builds if the index is missing, incrementally patches
// otherwise — the cheap keep-warm verb (build is a from-scratch rebuild).
func runRefresh(root string, jsonl bool) error {
	if err := ensureDBDir(root); err != nil {
		return err
	}
	db := dbPath(root)
	rep, done := reporter("refresh "+filepath.Base(mustAbs(root)), jsonl)
	if _, err := os.Stat(db); os.IsNotExist(err) {
		st, err := engine.BuildWithProgress(root, db, rep)
		if err != nil {
			return err
		}
		done(fmt.Sprintf("indexed %d files (%d symbols)", st.FilesParsed, st.Symbols))
		return nil
	}
	st, err := engine.PatchWithProgress(root, db, rep)
	if err != nil {
		return err
	}
	done(fmt.Sprintf("refreshed: %d files re-parsed, %d deleted", st.FilesParsed, st.Deleted))
	return nil
}

func mustAbs(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

// runStatus reports index state WITHOUT triggering any indexing — the IDE
// extension's side-effect-free detection primitive.
func runStatus(root string, asJSON bool) error {
	out := map[string]any{"schema_required": graph.SchemaVersion()}
	db := dbPath(root)
	if _, err := os.Stat(db); os.IsNotExist(err) {
		out["state"] = "unindexed"
		return printStatus(out, asJSON)
	}
	// Sidecar first: a live build owns the state.
	if b, err := os.ReadFile(filepath.Join(root, ".codeindex", "status.json")); err == nil {
		var side map[string]any
		if json.Unmarshal(b, &side) == nil {
			if st, _ := side["state"].(string); st == "building" || st == "patching" {
				if ts, _ := side["started_at"].(string); ts != "" {
					if t0, err := time.Parse(time.RFC3339, ts); err == nil && time.Since(t0) > 10*time.Minute {
						side["stale"] = true // crashed builder, most likely
					}
				}
				for k, v := range side {
					out[k] = v
				}
				return printStatus(out, asJSON)
			}
			out["last_indexed"] = side["indexed_at"]
		}
	}
	ver, err := graph.FileSchemaVersion(db)
	if err != nil {
		return err
	}
	out["schema_version"] = ver
	if ver != graph.SchemaVersion() {
		out["state"] = "stale-schema"
		return printStatus(out, asJSON)
	}
	files, symbols, edges, err := graph.IndexCounts(db)
	if err != nil {
		return err
	}
	fi, _ := os.Stat(db)
	out["state"] = "indexed"
	out["files"], out["symbols"], out["edges"] = files, symbols, edges
	if fi != nil {
		out["index_bytes"] = fi.Size()
	}
	return printStatus(out, asJSON)
}

func printStatus(out map[string]any, asJSON bool) error {
	if asJSON {
		b, err := json.MarshalIndent(out, "", " ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	switch out["state"] {
	case "unindexed":
		fmt.Println("unindexed (run: codeindex build .)")
	case "stale-schema":
		fmt.Printf("stale index: schema v%v, this binary uses v%v (next query rebuilds)\n",
			out["schema_version"], out["schema_required"])
	case "building", "patching":
		fmt.Printf("%v: %v %v/%v (started %v)\n",
			out["state"], out["phase"], out["done"], out["total"], out["started_at"])
	default:
		fmt.Printf("indexed: %v files, %v symbols, %v edges (%.1f MB, schema v%v)\n",
			out["files"], out["symbols"], out["edges"],
			float64(toInt64(out["index_bytes"]))/1e6, out["schema_version"])
		if li, ok := out["last_indexed"]; ok && li != nil {
			fmt.Printf("last indexed: %v\n", li)
		}
	}
	return nil
}

func toInt64(v any) int64 {
	if i, ok := v.(int64); ok {
		return i
	}
	return 0
}

// BenchResult is the machine-readable output of `bench` — the full
// performance-spec surface: build throughput, incremental latency, query
// latency incl. the lazy re-check, index size vs source, and peak memory.
type BenchResult struct {
	Repo             string  `json:"repo"`
	Files            int     `json:"files"`
	Symbols          int     `json:"symbols"`
	Lines            int     `json:"lines"`
	Workers          int     `json:"workers"`
	ColdBuildMs      float64 `json:"cold_build_ms"`
	FilesPerSec      float64 `json:"files_per_sec"`
	LinesPerSec      float64 `json:"lines_per_sec"`
	IncrementalMs    float64 `json:"incremental_patch_ms"`
	QueryP50Ms       float64 `json:"query_p50_ms"`
	QueryP95Ms       float64 `json:"query_p95_ms"`
	QuerySymbol      string  `json:"query_symbol"`
	IndexBytes       int64   `json:"index_bytes"`
	SourceBytes      int64   `json:"source_bytes"`
	IndexRatio       float64 `json:"index_ratio"`
	PeakRSSMB        float64 `json:"peak_rss_mb"`
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

	// 4) Query latency incl. lazy re-check (unchanged repo): pick a real
	// symbol with a healthy caller count, run repeated queries, report p50/p95.
	if sym, err := pickQuerySymbol(db); err == nil && sym != "" {
		res.QuerySymbol = sym
		times := make([]float64, 0, 21)
		for i := 0; i < 21; i++ {
			qs := time.Now()
			if _, err := query.CallersText(root, sym, 50); err != nil {
				break
			}
			times = append(times, ms(time.Since(qs)))
		}
		if len(times) > 1 {
			times = times[1:] // drop warm-up
			sortFloats(times)
			res.QueryP50Ms = times[len(times)/2]
			res.QueryP95Ms = times[(len(times)*95)/100]
		}
	}

	// 5) Index size vs walked-source bytes; peak RSS of this process.
	if fi, err := os.Stat(db); err == nil {
		res.IndexBytes = fi.Size()
	}
	if paths, err := merkle.Walk(root); err == nil {
		for _, rel := range paths {
			if fi, err := os.Stat(filepath.Join(root, rel)); err == nil {
				res.SourceBytes += fi.Size()
			}
		}
	}
	if res.SourceBytes > 0 {
		res.IndexRatio = float64(res.IndexBytes) / float64(res.SourceBytes)
	}
	res.PeakRSSMB = peakRSSMB()
	res.Workers = runtime.NumCPU()

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
	out, err := query.CallersText(root, name, limit)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

// runCallees prints what a symbol calls: each callee as a reference to its
// definition (when resolved) plus the call-site line.
func runCallees(root, name string, limit int) error {
	out, err := query.CalleesText(root, name, limit)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

// runImpact prints the composed counts-first blast-radius summary.
func runImpact(root, name string, limit int) error {
	out, err := query.ImpactText(root, name, limit)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

// runEnclosing prints the symbols overlapping a line range with caller counts —
// the edit-hook's data source. Empty result prints nothing and exits 0.
func runEnclosing(root, file string, start, end int) error {
	if _, err := query.Fresh(root); err != nil {
		return err
	}
	st, err := graph.Open(dbPath(root))
	if err != nil {
		return err
	}
	defer st.Close()
	encl, err := st.EnclosingSymbols(file, start, end)
	if err != nil {
		return err
	}
	for _, e := range encl {
		fmt.Printf("sym  %s  %s  %s:%d-%d  callers=%d external=%d\n",
			e.Name, e.Kind, e.File, e.StartLine, e.EndLine, e.Callers, e.ExternalCallers)
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
	fmt.Printf("  files=%d symbols=%d lines=%d workers=%d\n", r.Files, r.Symbols, r.Lines, r.Workers)
	fmt.Printf("  cold build:   %.0f ms  (%.0f files/s, %.0f lines/s)\n",
		r.ColdBuildMs, r.FilesPerSec, r.LinesPerSec)
	fmt.Printf("  incremental:  %.1f ms (single-file patch)\n", r.IncrementalMs)
	fmt.Printf("  query (incl. re-check, %q): p50=%.1f ms  p95=%.1f ms\n",
		r.QuerySymbol, r.QueryP50Ms, r.QueryP95Ms)
	fmt.Printf("  index: %.1f MB = %.2fx source (%.1f MB); peak RSS %.0f MB\n",
		float64(r.IndexBytes)/1e6, r.IndexRatio, float64(r.SourceBytes)/1e6, r.PeakRSSMB)
	fmt.Printf("  incremental == full rebuild: %v\n", r.IncrementalEqual)
	if !r.IncrementalEqual {
		fmt.Printf("  DIFF:\n%s\n", r.Diff)
	}
}

// pickQuerySymbol chooses a representative query target: the symbol whose
// caller count is the median among symbols with >=5 callers (avoids both
// trivial and pathological anchors).
func pickQuerySymbol(db string) (string, error) {
	st, err := graph.Open(db)
	if err != nil {
		return "", err
	}
	defer st.Close()
	return st.MedianCalledSymbol(5)
}

func sortFloats(xs []float64) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
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
