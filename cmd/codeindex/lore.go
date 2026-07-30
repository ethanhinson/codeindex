package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codeindex/internal/lore"
	"codeindex/internal/lore/gitinfo"
	"codeindex/internal/lore/index"
)

func loreDBPath(root string) string {
	return filepath.Join(root, ".codeindex", "lore.db")
}

// loreReindex is every lore subcommand's preamble: locate layers, patch the
// derived index, hand back the open store. Parse errors are fail-open (the
// report carries them; doctor surfaces them).
func loreReindex(root string) (lore.Layout, *index.Store, index.Report, error) {
	l, err := lore.NewLayout(root)
	if err != nil {
		return lore.Layout{}, nil, index.Report{}, err
	}
	st, rep, err := index.Reindex(l, loreDBPath(root))
	return l, st, rep, err
}

const loreUsage = "usage: codeindex lore <repo-root> " +
	"<add|show|search|for|backlog|promote|supersede|doctor|init|capture|event> ..."

func runLore(root string, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(loreUsage)
	}
	switch args[0] {
	case "add":
		return loreAdd(root, args[1:], out)
	case "show":
		return loreShow(root, args[1:], out)
	case "search":
		return loreSearch(root, args[1:], out)
	case "for":
		return loreFor(root, args[1:], out)
	case "backlog":
		return loreBacklog(root, args[1:], out)
	case "promote":
		return lorePromote(root, args[1:], out)
	case "supersede":
		return loreSupersede(root, args[1:], out)
	case "doctor":
		return loreDoctor(root, args[1:], out)
	case "init":
		return loreInit(root, args[1:], out)
	case "capture":
		return loreCapture(root, args[1:], out)
	case "event":
		return loreEvent(root, args[1:])
	default:
		return fmt.Errorf("unknown lore subcommand %q\n%s", args[0], loreUsage)
	}
}

// --- init ---

// loreHostContract is the behavioral contract injected into every host surface
// (Cursor .mdc rule, Codex AGENTS.md block). Single source of truth so the
// three hosts never drift.
const loreHostContract = `This repo keeps lore: committed decisions, work items, and notes (.lore/).
Before architectural choices or when past decisions are referenced, search lore
first: codeindex lore <root> search '<query>' (or the lore_search / lore_for_symbol
MCP tools). When a decision is made or a non-obvious root cause found, record it
with codeindex lore <root> add decision --title "..." --body - (include rejected
alternatives). Active decisions are constraints, not suggestions.
MCP: codeindex mcp <repo-root>`

const loreReadme = `# .lore/ — project decisions, work items, and notes

Records are Markdown files with YAML frontmatter, one file per record,
managed by ` + "`codeindex lore`" + ` and reviewed like code (they land via PRs).

- decisions/ — why the code is the way it is; status: active | superseded | rejected
- items/     — known work; status: open | done | dropped; priority p0..p3
- notes/     — gotchas, conventions, context

Useful commands:
  codeindex lore . add decision --title "..." --body - --anchor symbol:Foo
  codeindex lore . search "query"
  codeindex lore . for internal/some/pkg/
  codeindex lore . backlog
  codeindex lore . doctor
`

func loreInit(root string, args []string, out io.Writer) error {
	host := stringFlag(args, "--host")

	if host == "" {
		// No-flag path: unchanged behavior.
		return loreInitScaffold(root, args, out)
	}

	// With --host: scaffold first (tolerate already-initialized), then host surface.
	if err := loreInitScaffoldIdempotent(root, out); err != nil {
		return err
	}
	switch host {
	case "cursor":
		return initCursor(root, out)
	case "codex":
		return initCodex(root, out)
	case "claude":
		fmt.Fprintln(out, "Claude Code plugin: see plugin/README.md — the plugin ships the hooks; lore init does not duplicate them.")
		return nil
	case "all":
		if err := initCursor(root, out); err != nil {
			return err
		}
		if err := initCodex(root, out); err != nil {
			return err
		}
		fmt.Fprintln(out, "Claude Code plugin: see plugin/README.md — the plugin ships the hooks; lore init does not duplicate them.")
		return nil
	default:
		return fmt.Errorf("unknown host %q (valid: cursor, codex, claude, all)", host)
	}
}

// loreInitScaffoldCore sets up the .lore/ scaffold. When idempotent is true,
// a pre-existing README is silently accepted (used by --host flows). When false,
// it reports "already initialized" and returns (original no-flag behavior).
func loreInitScaffoldCore(root string, out io.Writer, idempotent bool) error {
	l, err := lore.NewLayout(root)
	if err != nil {
		return err
	}
	readme := filepath.Join(l.RepoDir, "README.md")
	if _, err := os.Stat(readme); err == nil {
		if idempotent {
			return nil
		}
		fmt.Fprintln(out, "already initialized:", l.RepoDir)
		return nil
	}
	for _, t := range []lore.Type{lore.TypeDecision, lore.TypeItem, lore.TypeNote} {
		if err := os.MkdirAll(l.Dir("repo", t), 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(readme, []byte(loreReadme), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "initialized %s (decisions/ items/ notes/)\n", l.RepoDir)
	fmt.Fprintln(out, "note: keep .codeindex/ gitignored — the lore index (lore.db) is derived")
	return nil
}

// loreInitScaffold is the original no-flag loreInit behavior.
func loreInitScaffold(root string, args []string, out io.Writer) error {
	return loreInitScaffoldCore(root, out, false)
}

// loreInitScaffoldIdempotent sets up .lore/ dirs and README if not already present, silently.
func loreInitScaffoldIdempotent(root string, out io.Writer) error {
	return loreInitScaffoldCore(root, out, true)
}

const cursorMDCTemplate = `---
description: Project lore — decisions, work items, notes
alwaysApply: true
---

` + loreHostContract + `
`

// initCursor writes .cursor/rules/lore.mdc with alwaysApply frontmatter and the contract.
func initCursor(root string, out io.Writer) error {
	dir := filepath.Join(root, ".cursor", "rules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "lore.mdc")
	if err := os.WriteFile(path, []byte(cursorMDCTemplate), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote %s\n", path)
	return nil
}

const codexBlockStart = "<!-- codeindex-lore:start (managed by codeindex lore init — do not hand-edit) -->"
const codexBlockEnd = "<!-- codeindex-lore:end -->"

// initCodex appends (or replaces in-place) a marker-delimited block in AGENTS.md.
func initCodex(root string, out io.Writer) error {
	agentsPath := filepath.Join(root, "AGENTS.md")
	block := codexBlockStart + "\n" + loreHostContract + "\n" + codexBlockEnd

	existing, err := os.ReadFile(agentsPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	content := string(existing)
	startIdx := strings.Index(content, codexBlockStart)
	if startIdx >= 0 {
		// Replace existing block in place — but only if the end marker is present and follows start.
		endIdx := strings.Index(content, codexBlockEnd)
		if endIdx < 0 || endIdx < startIdx {
			return fmt.Errorf("AGENTS.md has a codeindex-lore start marker without a matching end marker; remove the codeindex-lore markers by hand and re-run")
		}
		content = content[:startIdx] + block + content[endIdx+len(codexBlockEnd):]
	} else {
		// Append to file (with a newline separator if file is non-empty).
		if len(content) > 0 && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		if len(content) > 0 {
			content += "\n"
		}
		content += block + "\n"
	}

	if err := os.WriteFile(agentsPath, []byte(content), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote managed lore block in %s\n", agentsPath)
	return nil
}

// --- capture ---

func loreCapture(root string, args []string, out io.Writer) error {
	if !boolIn(args, "--stdin") {
		return errors.New("usage: codeindex lore <repo> capture --stdin")
	}
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil // fail-open: hooks must never surface errors
	}
	l, err := lore.NewLayout(root)
	if err != nil {
		return nil
	}
	path, err := lore.CaptureSession(l, raw, time.Now().UTC())
	if err != nil || path == "" {
		return nil
	}
	fmt.Fprintf(out, "captured %s\n", path)
	return nil
}

// --- event ---

// loreEvent appends one JSON line to <OverlayDir>/events.jsonl.
// --type and --status are required (usage error when missing).
// All storage failures are fail-open: return nil so CI is never broken.
func loreEvent(root string, args []string) error {
	typ := stringFlag(args, "--type")
	status := stringFlag(args, "--status")
	if typ == "" || status == "" {
		return errors.New("usage: codeindex lore <repo> event --type <t> --status <ok|failed> [--commit <sha>] [--detail <text>]")
	}

	detail := stringFlag(args, "--detail")
	commit := stringFlag(args, "--commit")
	if commit == "" {
		// Default to gitinfo Head() when available.
		l, err := lore.NewLayout(root)
		if err == nil {
			g := gitinfo.New(l.RepoRoot)
			if g.Available() {
				if sha, err := g.Head(); err == nil {
					commit = sha
				}
			}
		}
	}

	l, err := lore.NewLayout(root)
	if err != nil {
		return nil // fail-open
	}
	if err := os.MkdirAll(l.OverlayDir, 0o755); err != nil {
		return nil // fail-open
	}
	eventsPath := filepath.Join(l.OverlayDir, "events.jsonl")

	type eventRecord struct {
		SHA     string `json:"sha"`
		Type    string `json:"type"`
		Status  string `json:"status"`
		Detail  string `json:"detail,omitempty"`
		Created string `json:"created"`
	}
	ev := eventRecord{
		SHA:     commit,
		Type:    typ,
		Status:  status,
		Detail:  detail,
		Created: time.Now().UTC().Format(time.RFC3339),
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return nil // fail-open
	}

	f, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil // fail-open
	}
	defer f.Close()
	line = append(line, '\n')
	_, _ = f.Write(line)
	return nil
}

// --- flag helpers (plain args scan, matching main.go's style) ---

func stringFlag(args []string, name string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
}

func multiFlag(args []string, name string) []string {
	var out []string
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name {
			out = append(out, args[i+1])
		}
	}
	return out
}

func boolIn(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}

// --- record construction helpers ---

// recordFromFlags builds a new record of typ from add-style flags.
func recordFromFlags(typ lore.Type, args []string) (lore.Record, error) {
	title := stringFlag(args, "--title")
	if title == "" {
		return lore.Record{}, fmt.Errorf("--title is required")
	}
	body := stringFlag(args, "--body")
	if body == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return lore.Record{}, err
		}
		body = string(b)
	}
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	rec := lore.Record{
		ID: lore.NewID(typ), Type: typ, Title: title,
		Status: lore.DefaultStatus(typ),
		Date:   time.Now().UTC().Format("2006-01-02"),
		Body:   body, Priority: stringFlag(args, "--priority"),
		Tags: multiFlag(args, "--tag"), BlockedBy: multiFlag(args, "--blocked-by"),
	}
	for _, a := range multiFlag(args, "--anchor") {
		kind, val, ok := strings.Cut(a, ":")
		if !ok || (kind != "path" && kind != "symbol") {
			return lore.Record{}, fmt.Errorf("bad --anchor %q (want path:P or symbol:S)", a)
		}
		if kind == "path" {
			rec.Anchors = append(rec.Anchors, lore.Anchor{Path: val})
		} else {
			rec.Anchors = append(rec.Anchors, lore.Anchor{Symbol: val})
		}
	}
	for _, r := range multiFlag(args, "--ref") {
		kind, val, ok := strings.Cut(r, ":")
		if !ok {
			return lore.Record{}, fmt.Errorf("bad --ref %q (want kind:value)", r)
		}
		rec.Refs = append(rec.Refs, lore.Ref{Kind: kind, Value: val})
	}
	return rec, nil
}

// writeNewRecord marshals rec and writes it into the layer/type directory,
// disambiguating same-day slug collisions with the ID tail.
func writeNewRecord(l lore.Layout, rec lore.Record, layer string) (string, error) {
	dir := l.Dir(layer, rec.Type)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := rec.Date + "-" + lore.Slug(rec.Title) + ".md"
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		name = rec.Date + "-" + lore.Slug(rec.Title) + "-" + rec.ID[len(rec.ID)-6:] + ".md"
		path = filepath.Join(dir, name)
	}
	b, err := rec.Marshal()
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, b, 0o644)
}

// --- add ---

func loreAdd(root string, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: codeindex lore <repo> add <decision|item|note> --title T ...")
	}
	typ := lore.Type(args[0])
	if typ != lore.TypeDecision && typ != lore.TypeItem && typ != lore.TypeNote {
		return fmt.Errorf("unknown record type %q (want decision|item|note)", args[0])
	}
	rec, err := recordFromFlags(typ, args[1:])
	if err != nil {
		return err
	}
	layer := "repo"
	if boolIn(args, "--private") {
		layer = "overlay"
	}
	l, err := lore.NewLayout(root)
	if err != nil {
		return err
	}
	path, err := writeNewRecord(l, rec, layer)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "created %s %s\n", rec.ID, path)
	return nil
}

// --- show ---

func loreShow(root string, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: codeindex lore <repo> show <id>")
	}
	_, st, _, err := loreReindex(root)
	if err != nil {
		return err
	}
	defer st.Close()
	r, ok, err := st.Get(args[0])
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no record %q", args[0])
	}
	flags := ""
	if r.Stale {
		flags = "  STALE"
	}
	fmt.Fprintf(out, "%s  %s/%s  %s  %s%s\n", r.ID, r.Type, orDash(r.Status),
		r.Layer, r.File, flags)
	if r.Survived > 0 {
		fmt.Fprintf(out, "confidence: %.2f (survived %d commits)\n", r.Confidence, r.Survived)
	}

	// Collect commit ref values for event lookup.
	var commitRefs []string
	for _, ref := range r.Refs {
		if ref.Kind == "commit" {
			commitRefs = append(commitRefs, ref.Value)
		}
	}
	if len(commitRefs) > 0 {
		events, err := st.EventsForSHAPrefixes(commitRefs)
		if err == nil {
			for _, ev := range events {
				sha7 := ev.SHA
				if len(sha7) > 7 {
					sha7 = sha7[:7]
				}
				fmt.Fprintf(out, "event: %s %s (%s)\n", ev.Type, ev.Status, sha7)
			}
		}
	}

	fmt.Fprintln(out)
	b, err := r.Record.Marshal()
	if err != nil {
		return err
	}
	if _, err := out.Write(b); err != nil {
		return err
	}
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// --- JSON output ---

type loreJSON struct {
	ID         string  `json:"id"`
	Type       string  `json:"type"`
	Title      string  `json:"title"`
	Status     string  `json:"status,omitempty"`
	Date       string  `json:"date"`
	Layer      string  `json:"layer"`
	File       string  `json:"file"`
	Stale      bool    `json:"stale,omitempty"`
	Unratified bool    `json:"unratified,omitempty"`
	Score      float64 `json:"score,omitempty"`
	Snippet    string  `json:"snippet,omitempty"`
	Priority   string  `json:"priority,omitempty"`
	Blocked    bool    `json:"blocked,omitempty"`
}

func toJSON(out io.Writer, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(b))
	return err
}

// recordFlags returns the trailing flag string for a record (e.g. "  STALE",
// "  UNRATIFIED", or both combined). These suffixes are appended to text
// output lines for search, for, and backlog.
func recordFlags(stale, ratified bool) string {
	flags := ""
	if stale {
		flags += "  STALE"
	}
	if !ratified {
		flags += "  UNRATIFIED"
	}
	return flags
}

// --- search ---

func loreSearch(root string, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: codeindex lore <repo> search <query> [--limit N] [--json]")
	}
	limit := 10
	if v := stringFlag(args, "--limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	_, st, _, err := loreReindex(root)
	if err != nil {
		return err
	}
	defer st.Close()
	all, err := st.All()
	if err != nil {
		return err
	}
	hits := index.Search(all, args[0], time.Now().UTC(), limit)
	if boolIn(args, "--json") {
		js := make([]loreJSON, 0, len(hits))
		for _, h := range hits {
			js = append(js, loreJSON{ID: h.Rec.ID, Type: string(h.Rec.Type),
				Title: h.Rec.Title, Status: h.Rec.Status, Date: h.Rec.Date,
				Layer: h.Rec.Layer, File: h.Rec.File, Stale: h.Rec.Stale,
				Unratified: !h.Rec.Ratified,
				Score: h.Score, Snippet: h.Snippet})
		}
		return toJSON(out, js)
	}
	for _, h := range hits {
		fmt.Fprintf(out, "%s  %.2f  [%s/%s]  %s — %s%s\n",
			h.Rec.ID, h.Score, h.Rec.Layer, orDash(h.Rec.Status), h.Rec.Title, h.Snippet,
			recordFlags(h.Rec.Stale, h.Rec.Ratified))
	}
	return nil
}

// --- for ---

func loreFor(root string, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: codeindex lore <repo> for <path-or-symbol> [--json]")
	}
	_, st, _, err := loreReindex(root)
	if err != nil {
		return err
	}
	defer st.Close()
	all, err := st.All()
	if err != nil {
		return err
	}
	matched := index.RecordsForAnchor(all, args[0])
	if boolIn(args, "--json") {
		js := make([]loreJSON, 0, len(matched))
		for _, r := range matched {
			js = append(js, loreJSON{ID: r.ID, Type: string(r.Type), Title: r.Title,
				Status: r.Status, Date: r.Date, Layer: r.Layer, File: r.File, Stale: r.Stale,
				Unratified: !r.Ratified})
		}
		return toJSON(out, js)
	}
	for _, r := range matched {
		fmt.Fprintf(out, "%s  [%s/%s]  %s%s\n", r.ID, r.Layer, orDash(r.Status), r.Title,
			recordFlags(r.Stale, r.Ratified))
	}
	return nil
}

// --- promote ---

func lorePromote(root string, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: codeindex lore <repo> promote <id>")
	}
	l, st, _, err := loreReindex(root)
	if err != nil {
		return err
	}
	defer st.Close()
	r, ok, err := st.Get(args[0])
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no record %q", args[0])
	}
	if r.Layer == "repo" {
		return fmt.Errorf("%s is already in the committed layer (%s)", r.ID, r.File)
	}
	dest := l.Dir("repo", r.Type) // session records promote as their parsed type (note)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	newPath := filepath.Join(dest, filepath.Base(r.File))
	if _, err := os.Stat(newPath); err == nil {
		base := strings.TrimSuffix(filepath.Base(r.File), ".md")
		newPath = filepath.Join(dest, base+"-"+r.ID[len(r.ID)-6:]+".md")
	}
	b, err := os.ReadFile(r.File)
	if err != nil {
		return err
	}
	if err := os.WriteFile(newPath, b, 0o644); err != nil {
		return err
	}
	if err := os.Remove(r.File); err != nil {
		return err
	}
	fmt.Fprintf(out, "promoted %s %s\n", r.ID, newPath)
	return nil
}

// --- supersede ---

func loreSupersede(root string, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: codeindex lore <repo> supersede <old-id> --title T ...")
	}
	l, st, _, err := loreReindex(root)
	if err != nil {
		return err
	}
	defer st.Close()
	old, ok, err := st.Get(args[0])
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no record %q", args[0])
	}
	if old.Type != lore.TypeDecision {
		return fmt.Errorf("%s is a %s; only decisions are superseded", old.ID, old.Type)
	}
	rec, err := recordFromFlags(lore.TypeDecision, args[1:])
	if err != nil {
		return err
	}
	rec.Supersedes = old.ID
	path, err := writeNewRecord(l, rec, "repo")
	if err != nil {
		return err
	}
	// Durable transition on the old record: rewrite its file.
	ob, err := os.ReadFile(old.File)
	if err != nil {
		return err
	}
	or, err := lore.Parse(ob, old.Type)
	if err != nil {
		return err
	}
	or.Status = "superseded"
	or.SupersededBy = rec.ID
	nb, err := or.Marshal()
	if err != nil {
		return err
	}
	if err := os.WriteFile(old.File, nb, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "created %s %s\n", rec.ID, path)
	fmt.Fprintf(out, "superseded %s\n", old.ID)
	return nil
}

// --- backlog ---

func loreBacklog(root string, args []string, out io.Writer) error {
	_, st, _, err := loreReindex(root)
	if err != nil {
		return err
	}
	defer st.Close()
	all, err := st.All()
	if err != nil {
		return err
	}
	openIDs := map[string]bool{}
	for _, r := range all {
		if r.Type == lore.TypeItem && r.Status == "open" {
			openIDs[r.ID] = true
		}
	}
	anchor := stringFlag(args, "--for")
	var items []index.StoredRecord
	for _, r := range all {
		if r.Type != lore.TypeItem || r.Status != "open" {
			continue
		}
		items = append(items, r)
	}
	if anchor != "" {
		items = index.RecordsForAnchor(items, anchor)
	}
	blocked := func(r index.StoredRecord) bool {
		for _, b := range r.BlockedBy {
			if openIDs[b] {
				return true
			}
		}
		return false
	}
	prio := func(p string) string {
		if p == "" {
			return "p2"
		}
		return p
	}
	sort.SliceStable(items, func(i, j int) bool {
		pi, pj := prio(items[i].Priority), prio(items[j].Priority)
		if pi != pj {
			return pi < pj
		}
		bi, bj := blocked(items[i]), blocked(items[j])
		if bi != bj {
			return !bi
		}
		return items[i].Date < items[j].Date
	})
	if boolIn(args, "--json") {
		js := make([]loreJSON, 0, len(items))
		for _, r := range items {
			js = append(js, loreJSON{ID: r.ID, Type: string(r.Type), Title: r.Title,
				Status: r.Status, Date: r.Date, Layer: r.Layer, File: r.File,
				Priority: prio(r.Priority), Blocked: blocked(r), Unratified: !r.Ratified})
		}
		return toJSON(out, js)
	}
	for _, r := range items {
		state := "ready"
		if blocked(r) {
			state = "BLOCKED"
		}
		fmt.Fprintf(out, "%s  %s  %s  %s%s\n", r.ID, prio(r.Priority), state, r.Title,
			recordFlags(r.Stale, r.Ratified))
	}
	return nil
}

// --- doctor ---

func loreDoctor(root string, args []string, out io.Writer) error {
	_, st, rep, err := loreReindex(root)
	if err != nil {
		return err
	}
	defer st.Close()
	findings := 0
	for _, fe := range rep.Errors {
		fmt.Fprintf(out, "parse-error  %s  %v\n", fe.Path, fe.Err)
		findings++
	}
	for _, d := range rep.Duplicates {
		// Report entries are "<id>: <path1>, <path2>"; print as columns.
		id, paths, _ := strings.Cut(d, ": ")
		fmt.Fprintf(out, "duplicate-id  %s  %s\n", id, paths)
		findings++
	}
	all, err := st.All()
	if err != nil {
		return err
	}
	stale, err := index.StaleRecords(root, dbPath(root), all)
	if err != nil {
		return err
	}
	byID := map[string]index.StoredRecord{}
	for _, r := range all {
		byID[r.ID] = r
	}
	for _, r := range all {
		if stale[r.ID] {
			fmt.Fprintf(out, "stale-anchor  %s  %s\n", r.ID, r.Title)
			findings++
			if err := st.SetStale(r.ID, true); err != nil {
				return err
			}
		} else if r.Stale {
			if err := st.SetStale(r.ID, false); err != nil {
				return err
			}
		}
		if r.Supersedes != "" {
			if _, ok := byID[r.Supersedes]; !ok {
				fmt.Fprintf(out, "dangling-ref  %s  supersedes %s\n", r.ID, r.Supersedes)
				findings++
			}
		}
		for _, b := range r.BlockedBy {
			if _, ok := byID[b]; !ok {
				fmt.Fprintf(out, "dangling-ref  %s  blocked_by %s\n", r.ID, b)
				findings++
			}
		}
		if r.SupersededBy != "" && r.Status != "superseded" {
			fmt.Fprintf(out, "inconsistent  %s  superseded_by set but status is %s\n",
				r.ID, orDash(r.Status))
			findings++
		}
		// Churn-suspect: when churnLines > 3× current total line count of anchored files.
		if r.ChurnLines > 0 {
			totalLines := countAnchorLines(r)
			if r.ChurnLines > 3*totalLines {
				fmt.Fprintf(out, "churn-suspect  %s  %s\n", r.ID, r.Title)
				findings++
			}
		}
	}
	if findings == 0 {
		fmt.Fprintln(out, "ok: no findings")
	} else {
		fmt.Fprintf(out, "%d finding(s)\n", findings)
	}
	return nil
}

// countAnchorLines counts the total number of lines across all files reachable
// under the path anchors of r. Missing paths contribute 0 lines. Only path
// anchors are counted; symbol anchors are ignored.
func countAnchorLines(r index.StoredRecord) int {
	total := 0
	for _, a := range r.Anchors {
		if a.Path == "" {
			continue
		}
		// Walk all files under the anchor path.
		_ = filepath.Walk(a.Path, func(p string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			f, err := os.Open(p)
			if err != nil {
				return nil
			}
			defer f.Close()
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				total++
			}
			return nil
		})
	}
	return total
}
