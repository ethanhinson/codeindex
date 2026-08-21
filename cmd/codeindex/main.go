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
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"codeindex/internal/config"
	"codeindex/internal/depmap"
	"codeindex/internal/engine"
	"codeindex/internal/graph"
	"codeindex/internal/mcpserver"
	"codeindex/internal/merkle"
	"codeindex/internal/progress"
	"codeindex/internal/query"
	"codeindex/internal/wsfresh"
	"codeindex/internal/wsquery"
)

const version = "0.2.0"

// usageVerbs is the single-line usage banner printed by the arg-count guard.
//
// The anchor-prefix sentence is load-bearing documentation, not decoration:
// the CLI is the ONLY surface that documents the optional <member-id>: anchor
// prefix. The MCP tool descriptions are frozen by owner ruling 2 and
// deliberately do not mention it (spec assumption 18), so removing it here
// leaves the feature undocumented everywhere.
const usageVerbs = "usage: codeindex <build|refresh|status|workspace-status|callers|callees|impact|dependents|deps|nav|find|grep|search|model|ingest|depmap|export|import|enclosing|serve|mcp|bench|init-workspace> <repo-root> ...\n" +
	"  on a workspace root, an anchor may carry an optional <member-id>: prefix (e.g. api:HandleLogin) to scope the lookup to that member"

// anchorArg is the anchor placeholder shown in the per-verb usage lines of the
// verbs that take one. It names the optional member prefix at every anchor
// site so the documentation is not stranded on the top-level banner alone.
const anchorArg = "<[member-id:]anchor>"

func main() {
	// Test/benchmark isolation escape hatch: when CODEINDEX_DISABLED is set,
	// refuse to run regardless of how the binary was reached (PATH, absolute
	// path, alias). Off by default — no effect on normal use. Used by the A/B
	// harness to guarantee the control arm cannot invoke the index.
	if os.Getenv("CODEINDEX_DISABLED") != "" {
		fmt.Fprintln(os.Stderr, "codeindex: disabled via CODEINDEX_DISABLED")
		os.Exit(127)
	}
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, usageVerbs)
		os.Exit(2)
	}
	cmd, root := os.Args[1], os.Args[2]
	switch cmd {
	case "build":
		if err := dispatchBuild(root, hasFlag("--progress")); err != nil {
			fatal(err)
		}
	case "refresh":
		if err := dispatchRefresh(root, hasFlag("--progress")); err != nil {
			fatal(err)
		}
	case "status":
		if err := dispatchStatus(root, hasFlag("--json")); err != nil {
			fatal(err)
		}
	case "workspace-status":
		if err := dispatchWorkspaceStatus(root, hasFlag("--json")); err != nil {
			fatal(err)
		}
	case "export":
		if err := refuseWorkspaceRoot("export", root); err != nil {
			fatal(err)
		}
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
		if err := refuseWorkspaceRoot("import", root); err != nil {
			fatal(err)
		}
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
			fatal(fmt.Errorf("usage: codeindex %s <repo-root> %s [--limit N] [--json]", cmd, anchorArg))
		}
		a, err := wsquery.CallersStructured(root, os.Args[3], intFlag("--limit", 50))
		if err != nil {
			fatal(err)
		}
		emit(a, hasFlag("--json"))
	case "impact":
		if len(os.Args) < 4 {
			fatal(fmt.Errorf("usage: codeindex impact <repo-root> %s [--limit N] [--json]", anchorArg))
		}
		a, err := wsquery.ImpactStructured(root, os.Args[3], intFlag("--limit", 50))
		if err != nil {
			fatal(err)
		}
		emit(a, hasFlag("--json"))
	case "dependents", "deps":
		if len(os.Args) < 4 {
			fatal(fmt.Errorf("usage: codeindex %s <repo-root> %s [--limit N] [--json]", cmd, anchorArg))
		}
		limit := intFlag("--limit", 50)
		var a answer
		var err error
		if cmd == "dependents" {
			a, err = wsquery.DependentsStructured(root, os.Args[3], limit)
		} else {
			a, err = wsquery.DepsStructured(root, os.Args[3], limit)
		}
		if err != nil {
			fatal(err)
		}
		emit(a, hasFlag("--json"))
	case "depmap":
		if err := refuseWorkspaceRoot("depmap", root); err != nil {
			fatal(err)
		}
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
	case "find":
		if len(os.Args) < 4 {
			fatal(fmt.Errorf("usage: codeindex find <repo-root> <query> [--kind k] [--path p] [--limit N] [--json]"))
		}
		a, err := wsquery.FindStructured(root, os.Args[3], strFlag("--kind"), strFlag("--path"), intFlag("--limit", 20))
		if err != nil {
			fatal(err)
		}
		emit(a, hasFlag("--json"))
	case "nav":
		if len(os.Args) < 4 {
			fatal(fmt.Errorf("usage: codeindex nav <repo-root> %s [--limit N] [--json]", anchorArg))
		}
		a, err := wsquery.NavStructured(root, os.Args[3], intFlag("--limit", 50))
		if err != nil {
			fatal(err)
		}
		emit(a, hasFlag("--json"))
	case "grep":
		if len(os.Args) < 4 {
			fatal(fmt.Errorf("usage: codeindex grep <repo-root> <pattern> [-w] [--limit N] [--json]"))
		}
		a, err := wsquery.GrepStructured(root, os.Args[3], intFlag("--limit", 30), hasFlag("-w") || hasFlag("--word"))
		if err != nil {
			fatal(err)
		}
		emit(a, hasFlag("--json"))
	case "search":
		if len(os.Args) < 4 {
			fatal(fmt.Errorf("usage: codeindex search <repo-root> <concept query> [--hints \"a b c\"] [--error-text \"...\"] [--limit N] [--flat]"))
		}
		out, err := wsquery.SearchText(root, os.Args[3],
			strings.Fields(strFlag("--hints")), strFlag("--error-text"),
			intFlag("--limit", 20), hasFlag("--flat"))
		if err != nil {
			fatal(err)
		}
		fmt.Print(out)
	case "model":
		// `model` manages embedding weights, not an index: the second arg is
		// the verb, not a repo root. use/status take the repo as third arg
		// (default ".").
		if err := runModel(os.Args[2:]); err != nil {
			fatal(err)
		}
	case "ingest":
		if err := refuseWorkspaceRoot("ingest", root); err != nil {
			fatal(err)
		}
		// codeindex ingest <repo> [profile-or-dir] [--check]
		if err := runIngest(root, os.Args[3:]); err != nil {
			fatal(err)
		}
	case "serve":
		if err := refuseWorkspaceRoot("serve", root); err != nil {
			fatal(err)
		}
		addr := "127.0.0.1:7676"
		rest := os.Args[3:]
		for i, a := range rest {
			if a == "--addr" && i+1 < len(rest) {
				addr = rest[i+1]
			}
		}
		if err := runServe(root, addr); err != nil {
			fatal(err)
		}
	case "init-workspace":
		if err := runInitWorkspace(root, os.Args[3:]); err != nil {
			fatal(err)
		}
	case "mcp":
		if err := mcpserver.Run(context.Background(), root, version); err != nil {
			fatal(err)
		}
	case "callees":
		if len(os.Args) < 4 {
			fatal(fmt.Errorf("usage: codeindex callees <repo-root> %s [--limit N] [--json]", anchorArg))
		}
		a, err := wsquery.CalleesStructured(root, os.Args[3], intFlag("--limit", 50))
		if err != nil {
			fatal(err)
		}
		emit(a, hasFlag("--json"))
	case "enclosing":
		// codeindex enclosing <repo-root> <file> <start>:<end>
		if len(os.Args) < 5 {
			fatal(fmt.Errorf("usage: codeindex enclosing <repo-root> <file> <start>:<end> [--json]"))
		}
		var start, end int
		if _, err := fmt.Sscanf(os.Args[4], "%d:%d", &start, &end); err != nil {
			fatal(fmt.Errorf("bad range %q (want start:end): %w", os.Args[4], err))
		}
		a, err := wsquery.EnclosingStructured(root, os.Args[3], start, end)
		if err != nil {
			fatal(err)
		}
		emit(a, hasFlag("--json"))
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

// intFlag returns the value following a flag anywhere in the args, or def.
func intFlag(f string, def int) int {
	for i := 4; i < len(os.Args)-1; i++ {
		if os.Args[i] == f {
			v := def
			fmt.Sscanf(os.Args[i+1], "%d", &v)
			return v
		}
	}
	return def
}

// strFlag returns the value following a flag anywhere in the args, or "".
func strFlag(f string) string {
	for i := 4; i < len(os.Args)-1; i++ {
		if os.Args[i] == f {
			return os.Args[i+1]
		}
	}
	return ""
}

// answer is any structured query result: text render for humans/agents, JSON
// tags for machines.
type answer interface{ Text() string }

// emit prints an answer on stdout in the selected format. The cold-build
// disclosure goes to stderr, so --json stdout is always parseable.
func emit(a answer, asJSON bool) {
	if asJSON {
		b, err := json.MarshalIndent(a, "", "  ")
		if err != nil {
			fatal(err)
		}
		fmt.Println(string(b))
		return
	}
	fmt.Print(a.Text())
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

// isWorkspaceRoot reports whether root carries a workspace manifest.
//
// Detection goes through wsquery.RootKind, never engine.DetectRootKind:
// wsquery's package doc pins DetectRootKind to a single non-test caller, and a
// second branch here would be a second enforcement site for one rule.
//
// A detection ERROR is deliberately reported as "not a workspace" rather than
// propagated. DetectRootKind errors on a root that holds neither a manifest
// nor indexable source, and that diagnosis belongs to the per-repo path the
// verb was already going to take — surfacing it from a root-kind probe would
// replace every verb's own error message with one about workspaces.
func isWorkspaceRoot(root string) bool {
	kind, err := wsquery.RootKind(root)
	return err == nil && kind == engine.RootWorkspace
}

// refuseWorkspaceRoot is the guard the five non-query per-repo verbs
// (export, import, ingest, depmap, serve) call before doing any work. It
// returns nil on a repo root and the single shared refusal message on a
// workspace root.
//
// search is NOT in this list: it refuses inside wsquery.SearchText, so the CLI
// and the MCP handler share one refusal rather than testing the root kind at
// two call sites.
//
// A malformed manifest surfaces here as the config fault it is:
// wsquery.RefuseWorkspaceRoot propagates config.LoadWorkspace's error
// unchanged rather than printing a refusal with an empty member list.
func refuseWorkspaceRoot(verb, root string) error {
	if !isWorkspaceRoot(root) {
		return nil
	}
	return wsquery.RefuseWorkspaceRoot(verb, root)
}

// fanOut runs per against every member of wsRoot's manifest, in manifest
// order, printing a per-member-prefixed line for each.
//
// Three properties are load-bearing (spec §2.1, assumption 14):
//
//   - A member missing from disk is REPORTED BY ID, never skipped silently. A
//     silent skip is how a workspace ends up half-built with the operator
//     believing otherwise.
//   - A member whose per-repo call fails does NOT abort the pass. The failure
//     is printed against that member's id and the loop continues, so the
//     members after the failure still get built.
//   - The returned aggregate error names EVERY failed member. Returning only
//     the first would leave the rest of the failures visible on stdout but
//     absent from the error the caller acts on.
//
// The aggregate error is returned to main's existing fatal, which is
// os.Exit(1) unconditionally. This adds no second exit mechanism: exit 2
// remains the pre-dispatch usage code, since a workspace root is a well-formed
// argument.
func fanOut(verb, wsRoot string, w io.Writer, per func(m config.ResolvedMember) error) error {
	steps, presentCount, err := fanOutSteps(wsRoot)
	if err != nil {
		return err
	}
	var failed []string
	for _, st := range steps {
		if st.Missing {
			fmt.Fprintf(w, "%s: missing — declared root %q is not on disk; skipping %s\n",
				st.ID, st.DeclRoot, verb)
			continue
		}
		fmt.Fprintf(w, "%s: %s %s\n", st.ID, verb, st.Member.AbsRoot)
		if err := per(st.Member); err != nil {
			fmt.Fprintf(w, "%s: %s failed: %v\n", st.ID, verb, err)
			failed = append(failed, st.ID)
		}
	}
	return fanOutErr(verb, failed, presentCount)
}

// fanStep is one member of a fan-out pass: either a declared member missing
// from disk, or a resolved one to run against.
type fanStep struct {
	ID       string
	DeclRoot string
	Missing  bool
	Member   config.ResolvedMember
}

// fanOutSteps resolves wsRoot's manifest into the pass's steps, in MANIFEST
// order so present and missing members interleave in the one order the user
// authored. It also returns the count of present members, which the aggregate
// error reports against.
//
// It is shared by every fan-out renderer — the text pass above and the
// workspace-root --json document — so the missing-by-id and manifest-order
// properties above cannot drift between them.
func fanOutSteps(wsRoot string) ([]fanStep, int, error) {
	ws, err := config.LoadWorkspace(wsRoot)
	if err != nil {
		return nil, 0, err
	}
	present, missing, err := ws.Resolve(wsRoot)
	if err != nil {
		return nil, 0, err
	}
	byID := make(map[string]config.ResolvedMember, len(present))
	for _, m := range present {
		byID[m.Member.ID] = m
	}
	missingSet := make(map[string]bool, len(missing))
	for _, id := range missing {
		missingSet[id] = true
	}
	steps := make([]fanStep, 0, len(ws.Members))
	for _, decl := range ws.Members {
		if missingSet[decl.ID] {
			steps = append(steps, fanStep{ID: decl.ID, DeclRoot: decl.Root, Missing: true})
			continue
		}
		m, ok := byID[decl.ID]
		if !ok {
			continue
		}
		steps = append(steps, fanStep{ID: decl.ID, DeclRoot: decl.Root, Member: m})
	}
	return steps, len(present), nil
}

// fanOutErr is the one construction site for the aggregate error, so the text
// and JSON passes exit 1 with the same wording naming EVERY failed member.
func fanOutErr(verb string, failed []string, presentCount int) error {
	if len(failed) == 0 {
		return nil
	}
	return fmt.Errorf("codeindex %s: %d of %d present member(s) failed: %s",
		verb, len(failed), presentCount, strings.Join(failed, ", "))
}

// dispatchBuild routes build by root kind: the unchanged per-repo build on a
// repo root, the §2.1 fan-out on a workspace root.
func dispatchBuild(root string, jsonl bool) error {
	if !isWorkspaceRoot(root) {
		return runBuild(root, jsonl)
	}
	return fanOut("build", root, os.Stdout, func(m config.ResolvedMember) error {
		return runBuild(m.AbsRoot, jsonl)
	})
}

// dispatchStatus routes status by root kind. On a workspace root it prints the
// per-member fan-out and THEN the workspace-status block, so one command
// answers both halves (§2.1, §6).
func dispatchStatus(root string, asJSON bool) error {
	if !isWorkspaceRoot(root) {
		return runStatus(root, asJSON)
	}
	if asJSON {
		return statusWorkspaceJSON(root, os.Stdout)
	}
	ferr := fanOut("status", root, os.Stdout, func(m config.ResolvedMember) error {
		return runStatus(m.AbsRoot, asJSON)
	})
	// The workspace block runs even when a member's status failed: it reads
	// overlay state, not member indexes, so it is exactly the diagnostic an
	// operator wants when a member just reported a problem. The fan-out's
	// aggregate error is returned afterwards so the exit code still reflects
	// it.
	if err := runWorkspaceStatusBlock(root, asJSON); err != nil && ferr == nil {
		return err
	}
	return ferr
}

// wsStatusMember is one member's entry in the workspace-root --json document:
// its per-repo status, or the reason there isn't one.
type wsStatusMember struct {
	ID string `json:"id"`
	// Missing is the JSON form of the text pass's missing-by-id line: a
	// declared member absent from disk is REPORTED, never silently dropped
	// from the members array.
	Missing bool `json:"missing,omitempty"`
	// Error carries a per-member status failure. Like the text pass, one
	// member's failure does not abort the document — the remaining members
	// and the workspace block are still emitted.
	Error string `json:"error,omitempty"`
	// Status is the per-repo `status --json` object, EMBEDDED unchanged.
	Status map[string]any `json:"status,omitempty"`
}

// wsStatusDoc is the whole `status <workspace-root> --json` surface: exactly
// ONE top-level JSON value carrying both halves the verb answers (§2.1).
type wsStatusDoc struct {
	Members   []wsStatusMember `json:"members"`
	Workspace wsquery.Status   `json:"workspace"`
}

// statusWorkspaceJSON writes the single-document form of `status
// <workspace-root> --json`.
//
// WHY A DOCUMENT RATHER THAN A REFUSAL: the point of accepting a workspace root
// on `status` is that one command answers both halves, and --json is a
// documented flag — so on a newly accepted input it must produce something a
// parser can read. The text pass streams a prose header per member; JSON cannot
// stream that way without emitting N+1 top-level values, which is not JSON by
// any reading. So the JSON pass COLLECTS instead of streaming, and the prose
// headers become the `id` fields they were standing in for.
//
// The three fan-out properties are preserved verbatim, not re-derived: members
// come from the shared fanOutSteps (manifest order, missing reported by id), a
// failing member records its error and the pass continues, and the aggregate
// error from fanOutErr is returned AFTER the document is written so the exit
// code still reflects it through the one fatal()/exit-1 mechanism.
//
// Repo-mode `status --json` does not come through here at all: dispatchStatus
// routes a repo root to runStatus before this function is reached, so that
// output stays byte-identical.
func statusWorkspaceJSON(wsRoot string, w io.Writer) error {
	steps, presentCount, err := fanOutSteps(wsRoot)
	if err != nil {
		return err
	}
	doc := wsStatusDoc{Members: make([]wsStatusMember, 0, len(steps))}
	var failed []string
	for _, st := range steps {
		if st.Missing {
			doc.Members = append(doc.Members, wsStatusMember{ID: st.ID, Missing: true})
			continue
		}
		rep, err := statusReport(st.Member.AbsRoot)
		if err != nil {
			doc.Members = append(doc.Members, wsStatusMember{ID: st.ID, Error: err.Error()})
			failed = append(failed, st.ID)
			continue
		}
		doc.Members = append(doc.Members, wsStatusMember{ID: st.ID, Status: rep})
	}
	// The workspace block is read even when a member failed: it reads overlay
	// state, not member indexes, so it is exactly the diagnostic an operator
	// wants when a member just reported a problem.
	ws, werr := wsquery.WorkspaceStatus(wsRoot)
	if werr != nil {
		// Without the block there is no document to write; the fan-out's
		// own failures still win the error if there were any.
		if ferr := fanOutErr("status", failed, presentCount); ferr != nil {
			return ferr
		}
		return werr
	}
	doc.Workspace = ws
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, string(b))
	return fanOutErr("status", failed, presentCount)
}

// runWorkspaceStatusBlock prints the workspace-status report block for wsRoot
// — the overlay schema version, cross-edge and ambiguity counts, per-member
// stamp state, and the vendor version-skew lines (§6).
//
// It READS STATE AND DOES NOT FRESHEN — wsquery.WorkspaceStatus calls neither
// wsfresh.Freshen nor a query session — which is what makes it usable as a
// diagnostic on a workspace whose freshen is the thing being diagnosed, and
// what makes it safe for dispatchStatus to run it after a member's status has
// already failed.
//
// THIS IS THE ONE RENDERING SITE for the §6 report. `workspace-status` and
// `status <workspace-root>` both land here rather than each formatting a
// report of their own: two renderers for one report is two places for the
// D3 skew lines to drift apart.
func runWorkspaceStatusBlock(wsRoot string, asJSON bool) error {
	st, err := wsquery.WorkspaceStatus(wsRoot)
	if err != nil {
		return err
	}
	if asJSON {
		b, err := json.MarshalIndent(st, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	fmt.Print(st.Text())
	return nil
}

// dispatchWorkspaceStatus is the `workspace-status` verb. It is the mirror of
// the five per-repo verbs' refusal: handed a REPO root it refuses and names
// the repo-mode `status` verb, which is the verb that answers the same
// question there.
func dispatchWorkspaceStatus(root string, asJSON bool) error {
	if !isWorkspaceRoot(root) {
		return wsquery.RefuseRepoRoot("workspace-status", root, "status")
	}
	return runWorkspaceStatusBlock(root, asJSON)
}

// dispatchRefresh routes refresh by root kind.
//
// The workspace branch ITERATES NOTHING: wsfresh.Freshen IS the per-member
// freshen plus the overlay re-resolution gate. A per-member loop here would be
// a second enforcement site for ADR-0012's whole-pass rule, which is precisely
// the drift the gate exists to prevent — so refresh is the one maintenance
// verb that does not fan out (§2.1).
func dispatchRefresh(root string, jsonl bool) error {
	if !isWorkspaceRoot(root) {
		return runRefresh(root, jsonl)
	}
	rep, err := wsfresh.Freshen(root)
	if err != nil {
		return err
	}
	fmt.Printf("workspace refreshed: %d member(s) freshened, %d freshen-failed, %d unindexed, %d missing\n",
		rep.MembersFreshened, rep.MembersFreshenFailed, rep.MembersUnindexed, len(rep.MembersMissing))
	if len(rep.MembersFreshenFailedIDs) > 0 {
		fmt.Printf("  freshen failed: %s\n", strings.Join(rep.MembersFreshenFailedIDs, ", "))
	}
	if len(rep.MembersMissing) > 0 {
		fmt.Printf("  missing: %s\n", strings.Join(rep.MembersMissing, ", "))
	}
	if rep.Resolved {
		fmt.Printf("  cross-repo edges re-resolved (dirty: %s)\n", strings.Join(rep.Dirty, ", "))
	}
	return nil
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
//
// The report is built by statusReport and rendered by printStatus; runStatus is
// the composition of the two. The split exists so the workspace-root --json
// surface can EMBED a member's report in a larger document instead of printing
// a second top-level JSON value (§2.1). Repo-mode output is unchanged: the same
// map reaches the same printer.
func runStatus(root string, asJSON bool) error {
	out, err := statusReport(root)
	if err != nil {
		return err
	}
	return printStatus(out, asJSON)
}

// statusReport computes the per-repo status map for root without printing it.
func statusReport(root string) (map[string]any, error) {
	out := map[string]any{"schema_required": graph.SchemaVersion()}
	db := dbPath(root)
	if _, err := os.Stat(db); os.IsNotExist(err) {
		out["state"] = "unindexed"
		return out, nil
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
				return out, nil
			}
			out["last_indexed"] = side["indexed_at"]
		}
	}
	ver, err := graph.FileSchemaVersion(db)
	if err != nil {
		return nil, err
	}
	out["schema_version"] = ver
	if ver != graph.SchemaVersion() {
		out["state"] = "stale-schema"
		return out, nil
	}
	files, symbols, edges, err := graph.IndexCounts(db)
	if err != nil {
		return nil, err
	}
	fi, _ := os.Stat(db)
	out["state"] = "indexed"
	out["files"], out["symbols"], out["edges"] = files, symbols, edges
	if fi != nil {
		out["index_bytes"] = fi.Size()
	}
	return out, nil
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
