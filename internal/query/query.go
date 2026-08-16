// Package query is the shared query surface for the CLI and the MCP server:
// fresh-on-query behavior, formatted reference-based answers, and in-process
// serialization of index writes (SQLite is single-writer; the MCP server is
// long-lived and may receive concurrent tool calls).
package query

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"codeindex/internal/depmap"
	"codeindex/internal/engine"
	"codeindex/internal/graph"
	codeindexruntime "codeindex/internal/runtime"
	"codeindex/internal/search"
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

// FreshInfo says what Fresh actually did, so query surfaces can disclose an
// implicit cold build instead of just having been mysteriously slow.
type FreshInfo struct {
	Built       bool // full build (index was missing)
	FilesParsed int
	Symbols     int
	Duration    time.Duration
}

// lastCold remembers a Fresh-triggered full build until a surface discloses
// it (one disclosure per build: the first query result carries it).
var lastCold *FreshInfo

// ConsumeColdBuild returns and clears the pending cold-build disclosure.
func ConsumeColdBuild() (FreshInfo, bool) {
	mu.Lock()
	defer mu.Unlock()
	if lastCold == nil {
		return FreshInfo{}, false
	}
	info := *lastCold
	lastCold = nil
	return info, true
}

// Fresh makes the index reflect the working tree: builds if missing, patches
// otherwise. Safe for concurrent use.
func Fresh(root string) (FreshInfo, error) {
	mu.Lock()
	defer mu.Unlock()
	start := time.Now()
	if err := os.MkdirAll(filepath.Join(root, ".codeindex"), 0o755); err != nil {
		return FreshInfo{}, err
	}
	if _, err := os.Stat(dbPath(root)); os.IsNotExist(err) {
		st, err := engine.Build(root, dbPath(root))
		info := FreshInfo{Built: true, FilesParsed: st.FilesParsed, Symbols: st.Symbols,
			Duration: time.Since(start)}
		if err == nil {
			lastCold = &info
		}
		return info, err
	}
	st, err := engine.Patch(root, dbPath(root))
	if err != nil {
		return FreshInfo{}, err
	}
	// Hacked-dep overlay: verify covered vendor files (size+mtime fast path)
	// and shadow locally modified ones. No-op when no maps are attached.
	if err := depmap.VerifyOverlay(root, dbPath(root)); err != nil {
		return FreshInfo{}, err
	}
	return FreshInfo{FilesParsed: st.FilesParsed, Symbols: st.Symbols,
		Duration: time.Since(start)}, nil
}

func open(root string) (*graph.Store, error) {
	if _, err := Fresh(root); err != nil {
		return nil, err
	}
	st, err := graph.Open(dbPath(root))
	if err != nil {
		return nil, err
	}
	// Runtime spool pickup rides the freshness contract: unseen profiles in
	// .codeindex/runtime/ ingest before the query answers, exactly like
	// changed files patch the index. Ledger-deduped, so the common case is
	// a cheap directory scan.
	mu.Lock()
	_, ierr := codeindexruntime.IngestSpool(st, root)
	mu.Unlock()
	if ierr != nil {
		fmt.Fprintf(os.Stderr, "codeindex: runtime spool ingestion: %v\n", ierr)
	}
	return st, nil
}

// Callers answers definitions + callers of an anchor ("name" or
// "Type.method"/"Type::method"), plus the files referencing it (one-call
// completeness: the def+callers answer usually pairs with "which files
// reference X", so no follow-up probe is needed).
func Callers(root, anchor string, limit int) (*CallersAnswer, error) {
	name, parent := SplitAnchor(anchor)
	st, err := open(root)
	if err != nil {
		return nil, err
	}
	defer st.Close()

	defs, err := st.Definitions(name, parent)
	if err != nil {
		return nil, err
	}
	callers, err := st.Callers(name, parent)
	if err != nil {
		return nil, err
	}
	files, err := st.ReferencingFiles(name)
	if err != nil {
		return nil, err
	}

	a := &CallersAnswer{
		Anchor:               anchor,
		Definitions:          defRefs(defs),
		CallersTotal:         len(callers),
		Callers:              make([]CallerRef, 0),
		ReferencedFilesTotal: len(files),
		ReferencedFiles:      make([]string, 0),
	}
	for i, c := range callers {
		if i >= limit {
			break
		}
		a.Callers = append(a.Callers, CallerRef{
			Name: c.Name, Parent: c.Parent, QName: c.QName(),
			File: c.File, Line: c.Line, Ambiguous: c.Conf == graph.ConfAmbiguous,
		})
	}
	for i, f := range files {
		if i >= limit {
			break
		}
		a.ReferencedFiles = append(a.ReferencedFiles, f)
	}
	return a, nil
}

// CallersText renders Callers in the stable text format.
func CallersText(root, anchor string, limit int) (string, error) {
	a, err := Callers(root, anchor, limit)
	if err != nil {
		return "", err
	}
	return a.Text(), nil
}

// depInfo resolves provenance for a dep-resolved definition file:
// ("namespace@version", locally-modified). Empty for project files.
func depInfo(st *graph.Store, defFile string) (string, bool) {
	if defFile == "" {
		return "", false
	}
	ns, ver, modified, err := st.DepProvenance(defFile)
	if err != nil || ns == "" {
		return "", false
	}
	return ns + "@" + ver, modified
}

// Callees answers what the anchor calls, each resolved to its definition.
func Callees(root, anchor string, limit int) (*CalleesAnswer, error) {
	name, parent := SplitAnchor(anchor)
	st, err := open(root)
	if err != nil {
		return nil, err
	}
	defer st.Close()
	callees, err := st.Callees(name, parent)
	if err != nil {
		return nil, err
	}
	a := &CalleesAnswer{Anchor: anchor, Total: len(callees), Callees: make([]CalleeRef, 0)}
	for i, c := range callees {
		if i >= limit {
			break
		}
		dep, modified := depInfo(st, c.DefFile)
		a.Callees = append(a.Callees, CalleeRef{
			Name: c.Name, Parent: c.DefParent, QName: c.QName(),
			CallLine: c.CallLine, DefFile: c.DefFile, DefLine: c.DefLine,
			Ambiguous: c.Conf == graph.ConfAmbiguous, Dep: dep, DepModified: modified,
		})
	}
	return a, nil
}

// CalleesText renders Callees in the stable text format.
func CalleesText(root, anchor string, limit int) (string, error) {
	a, err := Callees(root, anchor, limit)
	if err != nil {
		return "", err
	}
	return a.Text(), nil
}

// Find is ranked symbol search under partial knowledge (locate mode for
// vague/common names — distinctive full names stay with plain grep).
func Find(root, q, kind, path string, limit int) (*FindAnswer, error) {
	st, err := open(root)
	if err != nil {
		return nil, err
	}
	defer st.Close()
	results, err := search.Find(st, q, search.Opts{Kind: kind, Path: path, Limit: limit})
	if err != nil {
		return nil, err
	}
	a := &FindAnswer{Query: q, Total: len(results), Results: make([]FindRef, 0, len(results))}
	for _, r := range results {
		dep, modified := "", false
		if r.Sym.Tier == 1 {
			dep, modified = depInfo(st, r.Sym.File)
		}
		a.Results = append(a.Results, FindRef{
			QName: r.Sym.QName(), Kind: string(r.Sym.Kind),
			File: r.Sym.File, Line: r.Sym.StartLine,
			Callers: r.Callers, Dep: dep, DepModified: modified, Match: r.Match,
		})
	}
	return a, nil
}

// FindText renders Find in the stable text format.
func FindText(root, q string, kind, path string, limit int) (string, error) {
	a, err := Find(root, q, kind, path, limit)
	if err != nil {
		return "", err
	}
	return a.Text(), nil
}

// Grep is enriched grep: content hits attributed to enclosing symbols,
// deduped with counts, defs-first. word restricts matches to word
// boundaries (rg -w semantics), so `Handler` doesn't match `HandlerFunc`.
func Grep(root, pattern string, limit int, word bool) (*GrepAnswer, error) {
	st, err := open(root)
	if err != nil {
		return nil, err
	}
	defer st.Close()
	groups, raw, backend, err := search.Grep(st, root, pattern, limit, word)
	if err != nil {
		return nil, err
	}
	a := &GrepAnswer{Pattern: pattern, Word: word, RawHits: raw, Backend: backend, Groups: make([]GrepRef, 0, len(groups))}
	for _, g := range groups {
		qname := ""
		if g.Sym != nil {
			qname = g.Sym.QName()
		}
		a.Groups = append(a.Groups, GrepRef{
			QName: qname, File: g.File, Line: g.First.Line, Hits: g.Hits, IsDef: g.IsDef,
		})
	}
	return a, nil
}

// GrepText renders Grep in the stable text format.
func GrepText(root, pattern string, limit int, word bool) (string, error) {
	a, err := Grep(root, pattern, limit, word)
	if err != nil {
		return "", err
	}
	return a.Text(), nil
}

// Nav is the over-retrieval union: callers + find + grep in one pass, one
// answer serving every navigation-shaped question about the anchor (where
// defined, who calls, which files reference). Retrieval is ~ms per op, so
// running all three always beats deciding which one to run — measured
// (bench/scout FINDINGS): the union matches the per-tool formatter ceiling
// with zero routing. Grep is word-boundary here: the union arm was re-run
// with -w on all three task sets (bench/scout FINDINGS, 2026-08-16) and
// held 100% success with F1 up on every set — short anchors like `File`
// stop matching superstrings. When the graph has no call edges for the
// anchor, grep's enclosing-symbol attribution stands in for callers.
func Nav(root, anchor string, limit int) (*NavAnswer, error) {
	name, parent := SplitAnchor(anchor)
	st, err := open(root)
	if err != nil {
		return nil, err
	}
	defer st.Close()

	defs, err := st.Definitions(name, parent)
	if err != nil {
		return nil, err
	}
	callers, err := st.Callers(name, parent)
	if err != nil {
		return nil, err
	}
	finds, err := search.Find(st, anchor, search.Opts{Limit: limit})
	if err != nil {
		return nil, err
	}
	groups, _, _, err := search.Grep(st, root, regexp.QuoteMeta(name), 0, true)
	if err != nil {
		return nil, err
	}

	a := &NavAnswer{Anchor: anchor, Definitions: defRefs(defs),
		Matches: make([]FindRef, 0, len(finds)),
		Callers: make([]CallerRef, 0), Files: make([]string, 0)}
	for _, r := range finds {
		dep, modified := "", false
		if r.Sym.Tier == 1 {
			dep, modified = depInfo(st, r.Sym.File)
		}
		a.Matches = append(a.Matches, FindRef{
			QName: r.Sym.QName(), Kind: string(r.Sym.Kind),
			File: r.Sym.File, Line: r.Sym.StartLine,
			Callers: r.Callers, Dep: dep, DepModified: modified, Match: r.Match,
		})
	}
	// Definitions fallback: every exact-name owner from find (the measured
	// POLICY: "where is X defined" wants ALL owners of the bare name).
	if len(a.Definitions) == 0 {
		for _, r := range a.Matches {
			if r.Match == "exact" {
				a.Definitions = append(a.Definitions, DefRef{
					Name: name, QName: r.QName, Kind: r.Kind, File: r.File, Line: r.Line,
				})
			}
		}
	}

	a.CallersTotal = len(callers)
	for i, c := range callers {
		if i >= limit {
			break
		}
		a.Callers = append(a.Callers, CallerRef{
			Name: c.Name, Parent: c.Parent, QName: c.QName(),
			File: c.File, Line: c.Line, Ambiguous: c.Conf == graph.ConfAmbiguous,
		})
	}
	if len(callers) == 0 && len(groups) > 0 {
		a.CallersFromGrep = true
		a.CallersTotal = len(groups)
		for i, g := range groups {
			if i >= limit {
				break
			}
			qname := "<outside any symbol>"
			nm := qname
			if g.Sym != nil {
				qname, nm = g.Sym.QName(), g.Sym.Name
			}
			a.Callers = append(a.Callers, CallerRef{
				Name: nm, QName: qname, File: g.File, Line: g.First.Line,
			})
		}
	}

	// Files: find hits, then grep hits — deduped, order preserved.
	seen := map[string]bool{}
	var files []string
	for _, r := range a.Matches {
		if !seen[r.File] {
			seen[r.File] = true
			files = append(files, r.File)
		}
	}
	for _, g := range groups {
		if !seen[g.File] {
			seen[g.File] = true
			files = append(files, g.File)
		}
	}
	a.FilesTotal = len(files)
	if len(files) > limit {
		files = files[:limit]
	}
	a.Files = files
	return a, nil
}

// NavText renders Nav in the stable text format.
func NavText(root, anchor string, limit int) (string, error) {
	a, err := Nav(root, anchor, limit)
	if err != nil {
		return "", err
	}
	return a.Text(), nil
}

// Dependents answers who imports/extends/implements the anchor.
func Dependents(root, anchor string, limit int) (*DependentsAnswer, error) {
	name, _ := SplitAnchor(anchor)
	st, err := open(root)
	if err != nil {
		return nil, err
	}
	defer st.Close()
	deps, err := st.Dependents(name)
	if err != nil {
		return nil, err
	}
	a := &DependentsAnswer{Anchor: anchor, Total: len(deps), Dependents: make([]DependentRef, 0)}
	for i, d := range deps {
		if i >= limit {
			break
		}
		a.Dependents = append(a.Dependents, DependentRef{
			Kind: string(d.Kind), QName: d.QName(), File: d.File, Line: d.Line,
		})
	}
	return a, nil
}

// DependentsText renders Dependents in the stable text format.
func DependentsText(root, anchor string, limit int) (string, error) {
	a, err := Dependents(root, anchor, limit)
	if err != nil {
		return "", err
	}
	return a.Text(), nil
}

// Deps answers what the anchor depends on. File anchors list the file's
// imports; symbol anchors list extends/implements (+ the file's imports,
// as a separate labeled section).
func Deps(root, anchor string, limit int) (*DepsAnswer, error) {
	st, err := open(root)
	if err != nil {
		return nil, err
	}
	defer st.Close()

	a := &DepsAnswer{Anchor: anchor, Sections: make([]DepSection, 0)}
	section := func(label string, ds []graph.Dep) {
		s := DepSection{Label: label, Total: len(ds), Deps: make([]DepRef, 0)}
		for i, d := range ds {
			if i >= limit {
				break
			}
			s.Deps = append(s.Deps, DepRef{
				Kind: string(d.Kind), Target: d.Target,
				DefFile: d.DefFile, DefLine: d.DefLine, Line: d.Line,
			})
		}
		a.Sections = append(a.Sections, s)
	}

	if ok, err := st.HasFile(anchor); err != nil {
		return nil, err
	} else if ok {
		imports, err := st.FileImports(anchor)
		if err != nil {
			return nil, err
		}
		section("imports of "+anchor, imports)
		return a, nil
	}

	name, parent := SplitAnchor(anchor)
	sd, err := st.SymbolDeps(name, parent)
	if err != nil {
		return nil, err
	}
	section("deps of "+anchor, sd)
	// context: the defining file's imports
	defs, err := st.Definitions(name, parent)
	if err != nil {
		return nil, err
	}
	if len(defs) > 0 {
		imports, err := st.FileImports(defs[0].File)
		if err != nil {
			return nil, err
		}
		section("file imports ("+defs[0].File+")", imports)
	}
	return a, nil
}

// DepsText renders Deps in the stable text format.
func DepsText(root, anchor string, limit int) (string, error) {
	a, err := Deps(root, anchor, limit)
	if err != nil {
		return "", err
	}
	return a.Text(), nil
}

// impactCoverage states what edge kinds the impact summary covers.
const impactCoverage = "call + import/extends/implements edges; type-usage references not included"

// Impact composes a counts-first blast-radius summary: definitions, callers,
// callees, dependents. States coverage honestly.
func Impact(root, anchor string, limit int) (*ImpactAnswer, error) {
	name, parent := SplitAnchor(anchor)
	st, err := open(root)
	if err != nil {
		return nil, err
	}
	defer st.Close()

	defs, err := st.Definitions(name, parent)
	if err != nil {
		return nil, err
	}
	callers, err := st.Callers(name, parent)
	if err != nil {
		return nil, err
	}
	callees, err := st.Callees(name, parent)
	if err != nil {
		return nil, err
	}
	dependents, err := st.Dependents(name)
	if err != nil {
		return nil, err
	}

	a := &ImpactAnswer{
		Anchor:          anchor,
		Coverage:        impactCoverage,
		Definitions:     defRefs(defs),
		CallersTotal:    len(callers),
		Callers:         make([]CallerRef, 0),
		DependentsTotal: len(dependents),
		Dependents:      make([]DependentRef, 0),
		CalleesTotal:    len(callees),
		Callees:         make([]CalleeRef, 0),
	}
	for i, c := range callers {
		if i >= limit {
			break
		}
		a.Callers = append(a.Callers, CallerRef{
			Name: c.Name, Parent: c.Parent, QName: c.QName(),
			File: c.File, Line: c.Line, Ambiguous: c.Conf == graph.ConfAmbiguous,
		})
	}
	for i, d := range dependents {
		if i >= limit {
			break
		}
		a.Dependents = append(a.Dependents, DependentRef{
			Kind: string(d.Kind), QName: d.QName(), File: d.File, Line: d.Line,
		})
	}
	for i, c := range callees {
		if i >= limit {
			break
		}
		dep, modified := depInfo(st, c.DefFile)
		a.Callees = append(a.Callees, CalleeRef{
			Name: c.Name, Parent: c.DefParent, QName: c.QName(),
			CallLine: c.CallLine, DefFile: c.DefFile, DefLine: c.DefLine,
			Ambiguous: c.Conf == graph.ConfAmbiguous, Dep: dep, DepModified: modified,
		})
	}
	return a, nil
}

// ImpactText renders Impact in the stable text format.
func ImpactText(root, anchor string, limit int) (string, error) {
	a, err := Impact(root, anchor, limit)
	if err != nil {
		return "", err
	}
	return a.Text(), nil
}

// Enclosing answers which symbols overlap a line range, with caller counts —
// the edit-hook's data source.
func Enclosing(root, file string, start, end int) (*EnclosingAnswer, error) {
	st, err := open(root)
	if err != nil {
		return nil, err
	}
	defer st.Close()
	encl, err := st.EnclosingSymbols(file, start, end)
	if err != nil {
		return nil, err
	}
	a := &EnclosingAnswer{File: file, Start: start, End: end, Symbols: make([]EnclosingRef, 0, len(encl))}
	for _, e := range encl {
		a.Symbols = append(a.Symbols, EnclosingRef{
			Name: e.Name, Parent: e.Parent, Kind: string(e.Kind), File: e.File,
			StartLine: e.StartLine, EndLine: e.EndLine,
			Callers: e.Callers, ExternalCallers: e.ExternalCallers,
		})
	}
	return a, nil
}

func defRefs(defs []graph.Symbol) []DefRef {
	out := make([]DefRef, 0, len(defs))
	for _, d := range defs {
		out = append(out, DefRef{
			Name: d.Name, Parent: d.Parent, QName: d.QName(), Kind: string(d.Kind),
			File: d.File, Line: d.StartLine, Signature: oneLine(d.Signature),
		})
	}
	return out
}

// oneLine collapses whitespace runs (multi-line TS/PHP signatures) — the
// output contract is one result per line.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
