# Lore Host Integration (Plan 2 of 3) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lore reaches the host agents: an MCP `lore_*` tool family on the existing server, `related_lore` appended to impact/callers answers, `lore capture --stdin` for ambient hook capture, and packaging for Claude Code (plugin), Cursor (rules + MCP), and Codex (AGENTS.md + MCP) via `lore init --host`.

**Architecture:** All host surfaces are thin adapters over `internal/lore/index`. The MCP server calls the index directly (no dependency on cmd/). Hooks are one-line shell calls into the binary, silent on any failure. Packaging follows the plugin's measured adoption law: always-visible availability notes (hooks + MCP descriptions), lean commands, no heavy skills — the v3 A/B gate measured a fat skill apparatus net-negative.

**Tech Stack:** Go (existing deps only), Python 3 for Claude Code hooks (matches existing plugin hooks), Markdown for rules/commands.

**Spec:** `docs/superpowers/specs/2026-07-29-lore-engine-design.md` (Capture, Query surface/MCP, Host integration sections). Backlog item: `itm-01KYR17XECTSCDR5DZX5DXAWTJ`.

## Global Constraints

- Module path `codeindex`; no new Go dependencies; no SQLite build tags.
- `internal/mcpserver` may import `internal/lore` and `internal/lore/index` but NEVER `cmd/codeindex` (package main). Shared logic needed by both moves INTO `internal/lore/index`.
- Hooks fail-open and silent: any error → exit 0, no output, never block the host agent.
- Token discipline (measured, see plugin/README.md): the lore prompt note is ≤80 tokens and only injected when `.lore/` exists in the repo; MCP tool descriptions carry the when-to-use law; no SKILL.md files in this plan.
- MCP tool results are compact references + text blocks, mirroring existing tools' `text()` helper and trust language.
- `lore capture` is metadata-only: no LLM calls, best-effort JSON parsing, silent skip on anything malformed.
- Tests: `go test ./internal/lore/... ./internal/mcpserver/ ./cmd/codeindex/` — no tags, no env beyond `CODEINDEX_HOME` in fixtures.
- Commit after every task with a `lore:` prefix.

---

### Task 1: Move anchor matching into the index package

**Files:**
- Create: `internal/lore/index/match.go`
- Modify: `cmd/codeindex/lore.go` (delete local `anchorMatches`, call `index.AnchorMatches`)
- Test: `internal/lore/index/match_test.go`

**Interfaces:**
- Consumes: `lore.Anchor` (existing).
- Produces: `func AnchorMatches(a lore.Anchor, q string) bool` — exported, exact same semantics as the current private `anchorMatches` in cmd/codeindex/lore.go (symbols exact; paths either-direction prefix after trimming trailing `/`; empty path never matches). Also `func RecordsForAnchor(recs []StoredRecord, q string) []StoredRecord` — the filter loop currently inlined in `loreFor`/`loreBacklog`, extracted so the MCP server reuses it.

- [ ] **Step 1: Write the failing test**

```go
package index

import (
	"testing"

	"codeindex/internal/lore"
)

func TestAnchorMatches(t *testing.T) {
	cases := []struct {
		a    lore.Anchor
		q    string
		want bool
	}{
		{lore.Anchor{Symbol: "ResolveImports"}, "ResolveImports", true},
		{lore.Anchor{Symbol: "ResolveImports"}, "Resolve", false},
		{lore.Anchor{Path: "internal/engine/"}, "internal/engine/resolver.go", true},
		{lore.Anchor{Path: "internal/engine/resolver.go"}, "internal/engine/", true},
		{lore.Anchor{Path: ""}, "anything", false},
		{lore.Anchor{Path: "docs/"}, "internal/", false},
	}
	for _, c := range cases {
		if got := AnchorMatches(c.a, c.q); got != c.want {
			t.Fatalf("AnchorMatches(%+v, %q) = %v, want %v", c.a, c.q, got, c.want)
		}
	}
}

func TestRecordsForAnchor(t *testing.T) {
	recs := []StoredRecord{
		{Record: lore.Record{ID: "dec-A", Anchors: []lore.Anchor{{Path: "internal/engine/"}}}},
		{Record: lore.Record{ID: "dec-B", Anchors: []lore.Anchor{{Symbol: "Foo"}}}},
		{Record: lore.Record{ID: "dec-C"}},
	}
	got := RecordsForAnchor(recs, "internal/engine/x.go")
	if len(got) != 1 || got[0].ID != "dec-A" {
		t.Fatalf("got %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/lore/index/ -run 'TestAnchorMatches|TestRecordsForAnchor' -v`
Expected: FAIL with "undefined: AnchorMatches".

- [ ] **Step 3: Implement**

`internal/lore/index/match.go`:

```go
package index

import (
	"strings"

	"codeindex/internal/lore"
)

// AnchorMatches reports whether record anchor a covers query anchor q:
// symbols match exactly; paths match on either-direction prefix.
func AnchorMatches(a lore.Anchor, q string) bool {
	if a.Symbol != "" {
		return a.Symbol == q
	}
	ap := strings.TrimSuffix(a.Path, "/")
	qp := strings.TrimSuffix(q, "/")
	return ap != "" && (strings.HasPrefix(qp, ap) || strings.HasPrefix(ap, qp))
}

// RecordsForAnchor filters records to those with an anchor covering q.
func RecordsForAnchor(recs []StoredRecord, q string) []StoredRecord {
	var out []StoredRecord
	for _, r := range recs {
		for _, a := range r.Anchors {
			if AnchorMatches(a, q) {
				out = append(out, r)
				break
			}
		}
	}
	return out
}
```

In `cmd/codeindex/lore.go`: delete the private `anchorMatches`, replace its three call sites (`loreFor`, `loreBacklog`) with `index.AnchorMatches` / `index.RecordsForAnchor` where the shape fits (loreFor's whole filter loop becomes `matched := index.RecordsForAnchor(all, args[0])`).

- [ ] **Step 4: Run all affected tests**

Run: `go test ./internal/lore/... ./cmd/codeindex/ -run 'TestAnchor|TestRecordsFor|TestLore' && go vet ./...`
Expected: PASS, vet clean — CLI behavior unchanged.

- [ ] **Step 5: Commit**

```bash
git add internal/lore/index/match.go internal/lore/index/match_test.go cmd/codeindex/lore.go
git commit -m "lore: export anchor matching from index for MCP reuse"
```

---

### Task 2: `lore capture --stdin`

**Files:**
- Create: `internal/lore/capture.go`, `internal/lore/capture_test.go`
- Modify: `cmd/codeindex/lore.go` (register `case "capture"`)
- Test: append one CLI test to `cmd/codeindex/lore_test.go`

**Interfaces:**
- Produces: `func CaptureSession(l Layout, raw []byte, now time.Time) (string, error)` in package `lore` — parses a best-effort JSON payload (Claude Code Stop-hook shape) and appends a metadata note to `<SessionsDir>/<YYYY-MM-DD>-<sid8>.md`. Returns the file path written ("" and nil error when the payload is skipped as trivial/unusable — fail-open).
- Payload handling (all fields optional; unknown JSON → treat whole input as freeform text body):
  - `session_id` (string): first 8 chars become the filename suffix and note heading; missing → `unknown`.
  - `cwd` (string): recorded in the body.
  - `last_assistant_message` or `summary` (string): first 500 chars recorded under `## Last activity`.
- File format: a valid lore note (frontmatter id `note-` + ULID, title `Session <sid8> <date>`, date) so Reindex picks it up as layer `session` with zero special-casing. If the day-file exists, append `\n---\n## <HH:MM> UTC\n<body>` instead of writing frontmatter again (Reindex re-hashes and re-parses the whole file; appended sections live in the same record's body).
- CLI: `codeindex lore <repo> capture --stdin` reads os.Stdin, calls CaptureSession, prints `captured <path>` or nothing (exit 0) when skipped.

- [ ] **Step 1: Write the failing tests**

```go
package lore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func captureLayout(t *testing.T) Layout {
	t.Helper()
	t.Setenv("CODEINDEX_HOME", t.TempDir())
	l, err := NewLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestCaptureSessionWritesNote(t *testing.T) {
	l := captureLayout(t)
	now := time.Date(2026, 7, 29, 15, 4, 0, 0, time.UTC)
	raw := []byte(`{"session_id":"abcd1234efgh","cwd":"/w","last_assistant_message":"Fixed the resolver bug."}`)
	path, err := CaptureSession(l, raw, now)
	if err != nil || path == "" {
		t.Fatalf("capture: %q %v", path, err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "id: note-") || !strings.Contains(s, "Session abcd1234") ||
		!strings.Contains(s, "Fixed the resolver bug.") {
		t.Fatalf("note content:\n%s", s)
	}
	if filepath.Base(path) != "2026-07-29-abcd1234.md" {
		t.Fatalf("filename %q", filepath.Base(path))
	}
	// Round-trip: the written file must parse as a valid note.
	if _, err := Parse(b, TypeNote); err != nil {
		t.Fatalf("captured note does not parse: %v", err)
	}
}

func TestCaptureSessionAppendsSameDay(t *testing.T) {
	l := captureLayout(t)
	now := time.Date(2026, 7, 29, 15, 0, 0, 0, time.UTC)
	raw := []byte(`{"session_id":"abcd1234","summary":"first"}`)
	p1, _ := CaptureSession(l, raw, now)
	raw2 := []byte(`{"session_id":"abcd1234","summary":"second"}`)
	p2, err := CaptureSession(l, raw2, now.Add(time.Hour))
	if err != nil || p1 != p2 {
		t.Fatalf("append: %q vs %q, %v", p1, p2, err)
	}
	b, _ := os.ReadFile(p2)
	if !strings.Contains(string(b), "first") || !strings.Contains(string(b), "second") ||
		strings.Count(string(b), "id: note-") != 1 {
		t.Fatalf("appended file:\n%s", b)
	}
}

func TestCaptureSessionFreeformAndEmpty(t *testing.T) {
	l := captureLayout(t)
	now := time.Date(2026, 7, 29, 15, 0, 0, 0, time.UTC)
	if p, err := CaptureSession(l, []byte("plain text observation"), now); err != nil || p == "" {
		t.Fatalf("freeform: %q %v", p, err)
	}
	if p, err := CaptureSession(l, []byte("   \n"), now); err != nil || p != "" {
		t.Fatalf("empty input must skip silently: %q %v", p, err)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/lore/ -run TestCapture -v`
Expected: FAIL with "undefined: CaptureSession".

- [ ] **Step 3: Implement `internal/lore/capture.go`**

```go
package lore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// capturePayload is the best-effort shape of a host Stop-hook payload.
type capturePayload struct {
	SessionID            string `json:"session_id"`
	Cwd                  string `json:"cwd"`
	LastAssistantMessage string `json:"last_assistant_message"`
	Summary              string `json:"summary"`
}

// CaptureSession appends a metadata-only session note to the overlay's
// sessions layer. Fail-open by design: unusable input returns ("", nil).
// No LLM calls — this is the cheap ambient channel; curation happens later
// via promote.
func CaptureSession(l Layout, raw []byte, now time.Time) (string, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", nil
	}
	var p capturePayload
	body := ""
	if err := json.Unmarshal(raw, &p); err != nil {
		body = text // freeform: keep the raw text as the observation
	} else {
		msg := p.LastAssistantMessage
		if msg == "" {
			msg = p.Summary
		}
		if msg == "" && p.Cwd == "" {
			return "", nil // JSON with nothing usable
		}
		if len(msg) > 500 {
			msg = msg[:500]
		}
		if p.Cwd != "" {
			body = "cwd: " + p.Cwd + "\n"
		}
		if msg != "" {
			body += "\n## Last activity\n" + msg + "\n"
		}
	}
	sid := p.SessionID
	if sid == "" {
		sid = "unknown"
	}
	if len(sid) > 8 {
		sid = sid[:8]
	}
	dir := l.SessionsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	date := now.Format("2006-01-02")
	path := filepath.Join(dir, date+"-"+sid+".md")
	if _, err := os.Stat(path); err == nil {
		entry := fmt.Sprintf("\n---\n## %s UTC\n%s\n", now.Format("15:04"), body)
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return "", err
		}
		defer f.Close()
		if _, err := f.WriteString(entry); err != nil {
			return "", err
		}
		return path, nil
	}
	rec := Record{
		ID: NewID(TypeNote), Type: TypeNote,
		Title: "Session " + sid + " " + date, Date: date,
		Body: body + "\n",
	}
	b, err := rec.Marshal()
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, b, 0o644)
}
```

Note: the append entry's `---` lines are inside the record BODY (after the closing frontmatter delimiter), so `Parse` treats them as content — the round-trip test in Step 1 guards this.

CLI in `cmd/codeindex/lore.go` — register `case "capture": return loreCapture(root, args[1:], out)` and:

```go
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
```

CLI test (append to `cmd/codeindex/lore_test.go`) — pipe via a temp stdin swap is awkward; test the layer boundary instead: capture is engine-tested above, so the CLI test just asserts the usage error path:

```go
func TestLoreCaptureRequiresStdinFlag(t *testing.T) {
	root := loreTestRepo(t)
	var buf bytes.Buffer
	if err := runLore(root, []string{"capture"}, &buf); err == nil {
		t.Fatal("want usage error without --stdin")
	}
}
```

- [ ] **Step 4: Run all tests**

Run: `go test ./internal/lore/... ./cmd/codeindex/ && go vet ./cmd/codeindex/ ./internal/lore/...`
Expected: PASS, vet clean.

- [ ] **Step 5: Commit**

```bash
git add internal/lore/capture.go internal/lore/capture_test.go cmd/codeindex/lore.go cmd/codeindex/lore_test.go
git commit -m "lore: capture --stdin ambient session notes (fail-open, metadata-only)"
```

---

### Task 3: MCP `lore_*` tool family

**Files:**
- Create: `internal/mcpserver/lore_tools.go`
- Test: `internal/mcpserver/lore_tools_test.go`
- Modify: `internal/mcpserver/mcpserver.go` (one call: `addLoreTools(s, repo)` at the end of `New`, before `return s`)

**Interfaces:**
- Consumes: `lore.NewLayout`, `index.Reindex`, `index.Search`, `index.RecordsForAnchor`, `index.StoredRecord`, `lore` record constructors (Task 1–2 + Plan 1).
- Produces: five tools on the existing server — `lore_search`, `lore_for_symbol`, `lore_backlog`, `lore_show`, `lore_add`. (`lore_promote` deferred: a destructive file move belongs behind explicit human CLI use for v1 — note this deviation from the spec's tool list in the commit message; the spec's tool family is otherwise complete.)
- Shared plumbing in lore_tools.go: `loreOpen(repo string) (lore.Layout, *index.Store, error)` wrapping NewLayout + Reindex with the db at `<repo>/.codeindex/lore.db`; `formatRecords([]index.StoredRecord) string` emitting the CLI's line format (`<id>  [<layer>/<status>]  <title>`, stale records suffixed `  STALE`).

- [ ] **Step 1: Write the failing test**

```go
package mcpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeindex/internal/lore"
)

func loreFixtureRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("CODEINDEX_HOME", t.TempDir())
	root := t.TempDir()
	dir := filepath.Join(root, ".lore", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := lore.Record{
		ID: lore.NewID(lore.TypeDecision), Type: lore.TypeDecision,
		Title: "Use Go for the engine", Status: "active", Date: "2026-07-29",
		Anchors: []lore.Anchor{{Symbol: "ResolveImports"}},
		Body:    "static binary\n",
	}
	b, err := rec.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "d.md"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestLoreSearchTool(t *testing.T) {
	root := loreFixtureRepo(t)
	out, err := loreSearchText(root, "go engine", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Use Go for the engine") {
		t.Fatalf("search text:\n%s", out)
	}
}

func TestLoreForSymbolTool(t *testing.T) {
	root := loreFixtureRepo(t)
	out, err := loreForText(root, "ResolveImports")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Use Go for the engine") {
		t.Fatalf("for text:\n%s", out)
	}
	out, err = loreForText(root, "NoSuchSymbol")
	if err != nil || strings.TrimSpace(out) != "no lore records anchored to \"NoSuchSymbol\"" {
		t.Fatalf("miss text: %q %v", out, err)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/mcpserver/ -run TestLore -v`
Expected: FAIL with "undefined: loreSearchText".

- [ ] **Step 3: Implement `internal/mcpserver/lore_tools.go`**

Text-producing helpers first (testable without MCP transport, mirroring how query.ImpactText serves both CLI and MCP):

```go
package mcpserver

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"codeindex/internal/lore"
	"codeindex/internal/lore/index"
)

const loreTrust = "Lore records are the project's committed decisions, work " +
	"items, and notes. Treat active decisions as constraints on your work; " +
	"superseded records are history, stale records need verification. "

func loreOpen(repo string) (lore.Layout, *index.Store, error) {
	l, err := lore.NewLayout(repo)
	if err != nil {
		return lore.Layout{}, nil, err
	}
	st, _, err := index.Reindex(l, filepath.Join(repo, ".codeindex", "lore.db"))
	return l, st, err
}

func formatRecords(recs []index.StoredRecord) string {
	var b strings.Builder
	for _, r := range recs {
		status := r.Status
		if status == "" {
			status = "-"
		}
		flag := ""
		if r.Stale {
			flag = "  STALE"
		}
		fmt.Fprintf(&b, "%s  [%s/%s]  %s%s\n", r.ID, r.Layer, status, r.Title, flag)
	}
	return b.String()
}

func loreSearchText(repo, query string, limit int) (string, error) {
	_, st, err := loreOpen(repo)
	if err != nil {
		return "", err
	}
	defer st.Close()
	all, err := st.All()
	if err != nil {
		return "", err
	}
	hits := index.Search(all, query, time.Now().UTC(), limit)
	if len(hits) == 0 {
		return fmt.Sprintf("no lore records match %q", query), nil
	}
	var b strings.Builder
	for _, h := range hits {
		status := h.Rec.Status
		if status == "" {
			status = "-"
		}
		fmt.Fprintf(&b, "%s  %.2f  [%s/%s]  %s — %s\n",
			h.Rec.ID, h.Score, h.Rec.Layer, status, h.Rec.Title, h.Snippet)
	}
	return b.String(), nil
}

func loreForText(repo, anchor string) (string, error) {
	_, st, err := loreOpen(repo)
	if err != nil {
		return "", err
	}
	defer st.Close()
	all, err := st.All()
	if err != nil {
		return "", err
	}
	matched := index.RecordsForAnchor(all, anchor)
	if len(matched) == 0 {
		return fmt.Sprintf("no lore records anchored to %q", anchor), nil
	}
	return formatRecords(matched), nil
}
```

Then `addLoreTools(s *mcp.Server, repo string)` following the exact `mcp.AddTool` pattern in mcpserver.go — five tools:

- `lore_search` (args: `query string`, `limit int,omitempty`): "Search the project's lore — committed decisions (with rationale and rejected alternatives), open work items, and notes. Use BEFORE making an architectural choice or when the user references past decisions ('why do we…', 'didn't we decide…'). " + loreTrust → `loreSearchText`.
- `lore_for_symbol` (args: `anchor string` — "a symbol name or repo-relative path"): "Decisions, notes, and open work items anchored to a specific symbol or path. Use BEFORE modifying code to learn what was decided about it and what work is already planned there. " + loreTrust → `loreForText`.
- `lore_backlog` (args: `anchor string,omitempty`): open items via the same open-filter + sort logic as the CLI (reimplement the small filter inline: Type item, status open; sort priority→date; blocked detection via openIDs map — ~25 lines, acceptable duplication for v1, noted for Plan 3 consolidation) → text lines `<id>  <priority>  <BLOCKED|ready>  <title>`.
- `lore_show` (args: `id string`): full record — meta line + `r.Record.Marshal()` bytes as text.
- `lore_add` (args: `type string` (decision|item|note), `title string`, `body string`, `anchors []string,omitempty` ("path:P or symbol:S"), `refs []string,omitempty` ("kind:value"), `private bool,omitempty`): builds a `lore.Record` (NewID, DefaultStatus, today's date), validates anchors/refs with the same `strings.Cut` rules as the CLI, writes via the same date-slug filename convention into repo or overlay layer, returns `created <id> <path>`. Description: "Record a decision (with rationale AND rejected alternatives in the body), a work item, or a note. Use when an architectural choice is made, a non-obvious root cause is found, or the user says 'remember this' / 'we decided'. Write it while you have full context."

Each handler returns `text(out)` (existing helper) and wraps errors `fmt.Errorf("lore_x: %w", err)`.

Finally in `mcpserver.go` `New()`, before `return s`: `addLoreTools(s, repo)`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/mcpserver/ ./internal/lore/... && go vet ./internal/mcpserver/ && go build ./cmd/codeindex`
Expected: PASS; existing mcpserver tests still green.

- [ ] **Step 5: Commit**

```bash
git add internal/mcpserver/lore_tools.go internal/mcpserver/lore_tools_test.go internal/mcpserver/mcpserver.go
git commit -m "lore: MCP tool family — search/for_symbol/backlog/show/add (promote stays CLI-only for v1)"
```

---

### Task 4: `related_lore` on impact and callers

**Files:**
- Modify: `internal/mcpserver/mcpserver.go` (impact + callers handlers only)
- Create: helper in `internal/mcpserver/lore_tools.go`
- Test: append to `internal/mcpserver/lore_tools_test.go`

**Interfaces:**
- Produces: `func relatedLoreBlock(repo, symbol string) string` — returns `""` when no records anchor to the symbol (or on ANY error: lore must never break code navigation), else `"\n\nRelated lore (decisions/items anchored to this symbol):\n" + formatRecords(...)` capped at 5 records, active/open records first.
- The impact and callers handlers append `relatedLoreBlock(repo, in.Symbol)` to their output text before wrapping in `text(...)`. Callees/dependents/find/grep are NOT touched (blast-radius entry points only — the moments an agent is about to change the code).

- [ ] **Step 1: Write the failing test** (append)

```go
func TestRelatedLoreBlock(t *testing.T) {
	root := loreFixtureRepo(t) // has a decision anchored to ResolveImports
	blk := relatedLoreBlock(root, "ResolveImports")
	if !strings.Contains(blk, "Related lore") || !strings.Contains(blk, "Use Go for the engine") {
		t.Fatalf("block:\n%s", blk)
	}
	if blk := relatedLoreBlock(root, "NothingHere"); blk != "" {
		t.Fatalf("want empty block for unanchored symbol, got %q", blk)
	}
	if blk := relatedLoreBlock(t.TempDir(), "X"); blk != "" {
		t.Fatalf("want empty block for repo without lore, got %q", blk)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/mcpserver/ -run TestRelatedLore -v`
Expected: FAIL with "undefined: relatedLoreBlock".

- [ ] **Step 3: Implement**

```go
// relatedLoreBlock returns lore anchored to symbol, or "" — lore must never
// break code navigation, so every error path collapses to the empty string.
func relatedLoreBlock(repo, symbol string) string {
	_, st, err := loreOpen(repo)
	if err != nil {
		return ""
	}
	defer st.Close()
	all, err := st.All()
	if err != nil {
		return ""
	}
	matched := index.RecordsForAnchor(all, symbol)
	if len(matched) == 0 {
		return ""
	}
	// Active/open first, then the rest; cap at 5.
	var head, tail []index.StoredRecord
	for _, r := range matched {
		if r.Status == "active" || r.Status == "open" {
			head = append(head, r)
		} else {
			tail = append(tail, r)
		}
	}
	ordered := append(head, tail...)
	if len(ordered) > 5 {
		ordered = ordered[:5]
	}
	return "\n\nRelated lore (decisions/items anchored to this symbol):\n" +
		formatRecords(ordered)
}
```

In the `impact` handler: `out, err := query.ImpactText(...)` → on success `out += relatedLoreBlock(repo, in.Symbol)`. Same one-line addition in `callers`. Extend both tool descriptions with: "Output may include a Related lore section — decisions and open work items attached to this symbol; treat active decisions as constraints."

- [ ] **Step 4: Run tests**

Run: `go test ./internal/mcpserver/ && go vet ./internal/mcpserver/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mcpserver/
git commit -m "lore: related_lore blocks on impact and callers responses"
```

---

### Task 5: Claude Code plugin — lore note, capture hook, slash commands

**Files:**
- Create: `plugin/hooks/lore_context.py`, `plugin/hooks/lore_capture.py`, `plugin/commands/decide.md`, `plugin/commands/lore.md`
- Modify: `plugin/hooks/hooks.json` (register both hooks), `plugin/README.md` (new "Lore" section)

**Interfaces (behavioral contracts, mirroring the existing hooks' measured discipline):**
- `lore_context.py` (UserPromptSubmit, timeout 5): emits a ≤80-token availability note ONLY when `<cwd>/.lore/` exists AND the codeindex binary resolves (reuse the resolution logic from prompt_context.py: `CODEINDEX_BIN` env or `shutil.which`). Note text exactly:
  `"This repo keeps lore: committed decisions, work items, and notes (.lore/). Before architectural choices or when past decisions are referenced, use the lore_search / lore_for_symbol MCP tools (or `codeindex lore <root> search '<q>'`). When a decision is made or a non-obvious root cause found, record it with lore_add — include rejected alternatives. Active decisions are constraints."`
  Silent (no output, exit 0) otherwise or on any error. Respects the existing kill switches: `.codeindex/hook-disabled` file and `CODEINDEX_HOOK_DISABLE=1`.
- `lore_capture.py` (Stop, timeout 10): reads the Stop payload from stdin, and only when `<cwd>/.lore/` exists and the binary resolves, pipes the payload verbatim to `codeindex lore <cwd> capture --stdin` via subprocess. Always exits 0; never prints to stdout (Stop-hook output would block the agent's completion); same kill switches.
- `decide.md` slash command (`/codeindex:decide <title>`): frontmatter `description: Record a decision in .lore/ with rationale and rejected alternatives`, `argument-hint: <decision title>`, `allowed-tools: Bash(codeindex *)`. Body instructs: gather rationale + alternatives from the conversation, then run `codeindex lore "$CLAUDE_PROJECT_DIR" add decision --title "$1" --body -` with a heredoc body containing `## Alternatives considered`, anchoring with `--anchor symbol:X`/`--anchor path:Y` for the code discussed, then show the created record.
- `lore.md` slash command (`/codeindex:lore <query>`): runs `!`codeindex lore "$CLAUDE_PROJECT_DIR" search "$1" --limit 8`` and `!`codeindex lore "$CLAUDE_PROJECT_DIR" backlog`` and asks the model to summarize what's relevant to the current conversation.
- hooks.json: append the UserPromptSubmit entry (no matcher) and a new `Stop` array entry, preserving the existing two hooks untouched.

- [ ] **Step 1: Write `lore_context.py`** (model on prompt_context.py: same JSON-stdin parse, same binary resolution, same silent-failure contract; the `.lore` existence check replaces the language-manifest check)

- [ ] **Step 2: Write `lore_capture.py`**

```python
#!/usr/bin/env python3
"""Stop hook: pipe the session payload to `codeindex lore capture --stdin`.

Ambient channel of the lore engine: metadata-only session notes into the
private overlay (decays in ranking; promotable). Silent and exit-0 on every
failure path — a capture must never block the agent's stop.
"""
import json, os, shutil, subprocess, sys

def resolve_bin():
    b = os.environ.get("CODEINDEX_BIN")
    if b and os.path.exists(b):
        return b
    return shutil.which("codeindex")

def main():
    try:
        raw = sys.stdin.buffer.read()
        payload = json.loads(raw or b"{}")
        cwd = payload.get("cwd") or os.getcwd()
        if os.environ.get("CODEINDEX_HOOK_DISABLE") == "1":
            return
        if os.path.exists(os.path.join(cwd, ".codeindex", "hook-disabled")):
            return
        if not os.path.isdir(os.path.join(cwd, ".lore")):
            return
        binpath = resolve_bin()
        if not binpath:
            return
        subprocess.run([binpath, "lore", cwd, "capture", "--stdin"],
                       input=raw, capture_output=True, timeout=8)
    except Exception:
        pass

if __name__ == "__main__":
    main()
```

- [ ] **Step 3: Write both command files, update hooks.json and plugin/README.md** (README gains a "Lore" table section documenting the two hooks, two commands, and the kill switches; note explicitly that the capture hook only activates in repos that have run `lore init`)

- [ ] **Step 4: Verify**

Run: `python3 -m py_compile plugin/hooks/lore_context.py plugin/hooks/lore_capture.py && python3 -c "import json; json.load(open('plugin/hooks/hooks.json'))"`
Expected: both compile; hooks.json is valid JSON with 3 event keys.
Manual smoke: `echo '{"cwd":"'$PWD'","session_id":"smoketest1","last_assistant_message":"hi"}' | python3 plugin/hooks/lore_capture.py` inside the codeindex repo (which has .lore/) — then `ls "$CODEINDEX_HOME"/lore/*/sessions/` or `~/.codeindex/lore/*/sessions/` shows the day file; `codeindex lore . search smoketest` finds nothing (metadata note has no such token) but `codeindex lore . search "session"` ranks it.

- [ ] **Step 5: Commit**

```bash
git add plugin/
git commit -m "lore: Claude Code plugin — availability note, Stop-hook capture, /decide and /lore commands"
```

---

### Task 6: `lore init --host` scaffolding for Cursor and Codex + docs

**Files:**
- Modify: `cmd/codeindex/lore.go` (loreInit gains `--host <claude|cursor|codex|all>`), `README.md` (Lore section gains host-setup subsection)
- Create (at runtime, by init): `.cursor/rules/lore.mdc`, managed block in `AGENTS.md`
- Test: append to `cmd/codeindex/lore_test.go`

**Interfaces:**
- `lore init` (no flag): unchanged behavior (scaffold .lore/ only) — all existing tests stay green.
- `lore init --host cursor`: additionally writes `.cursor/rules/lore.mdc` (create dirs as needed) with frontmatter `description: Project lore — decisions, work items, notes` + `alwaysApply: true` and a body carrying the SAME behavioral contract as the Claude prompt note (search before architectural choices; record decisions with alternatives via `codeindex lore . add decision ...` or the MCP tools; active decisions are constraints), plus an MCP registration hint comment pointing at `codeindex mcp <repo-root>`.
- `lore init --host codex`: appends (or replaces, marker-delimited) a managed block to `AGENTS.md`:
  `<!-- codeindex-lore:start (managed by codeindex lore init — do not hand-edit) -->` … same contract text … `<!-- codeindex-lore:end -->`. Idempotent: existing block is replaced in place, never duplicated.
- `lore init --host claude`: prints pointer to the plugin (`plugin/README.md`) — the plugin ships the hooks; init does not duplicate them.
- `lore init --host all`: cursor + codex + the claude pointer.
- Unknown host → error listing valid values.

- [ ] **Step 1: Write the failing tests** (append)

```go
func TestLoreInitHostCursorAndCodex(t *testing.T) {
	root := loreTestRepo(t)
	runLoreOK(t, root, "init", "--host", "cursor")
	b, err := os.ReadFile(filepath.Join(root, ".cursor", "rules", "lore.mdc"))
	if err != nil || !strings.Contains(string(b), "alwaysApply: true") ||
		!strings.Contains(string(b), "codeindex lore") {
		t.Fatalf("cursor rule: %v\n%s", err, b)
	}
	runLoreOK(t, root, "init", "--host", "codex")
	runLoreOK(t, root, "init", "--host", "codex") // idempotent
	ab, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(ab), "codeindex-lore:start") != 1 {
		t.Fatalf("managed block duplicated:\n%s", ab)
	}
}

func TestLoreInitHostUnknown(t *testing.T) {
	root := loreTestRepo(t)
	var buf bytes.Buffer
	if err := runLore(root, []string{"init", "--host", "vim"}, &buf); err == nil {
		t.Fatal("want error for unknown host")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./cmd/codeindex/ -run TestLoreInitHost -v`
Expected: FAIL (flag not handled; files not written).

- [ ] **Step 3: Implement** — `loreInit` parses `stringFlag(args, "--host")`; empty → current behavior; otherwise scaffold .lore/ first (reuse existing logic, tolerate already-initialized), then per-host writers `initCursor(root) error` / `initCodex(root) error` (marker-replace via strings.Index on the start/end markers) / claude pointer print. The shared contract text is one Go const `loreHostContract` used by both writers so the three hosts never drift.

- [ ] **Step 4: Run all tests**

Run: `go test ./cmd/codeindex/ ./internal/lore/... ./internal/mcpserver/ && go vet ./... && go build ./cmd/codeindex`
Expected: all green.

- [ ] **Step 5: Update README.md** — under the Lore section add "Host setup": one paragraph + the three `lore init --host` invocations, MCP registration snippets for Cursor (`.cursor/mcp.json` with `"command": "codeindex", "args": ["mcp", "."]`) and Codex (`config.toml` `[mcp_servers.codeindex]` entry), and a pointer to the Claude Code plugin.

- [ ] **Step 6: Full-suite + dogfood smoke, then commit**

Run: `go test ./... && go build -o /tmp/codeindex-p2 ./cmd/codeindex`
Dogfood: in the codeindex repo itself — `/tmp/codeindex-p2 lore . init --host codex` (verify AGENTS.md block appears once), then start `/tmp/codeindex-p2 mcp .` briefly via the MCP inspector if available or verify `lore_search` appears in the tool list by grepping the binary's startup (acceptable: `go test ./internal/mcpserver/` already covers tool registration).

```bash
git add cmd/codeindex/ README.md
git commit -m "lore: init --host scaffolding for cursor/codex + host setup docs — plan 2 complete"
```

---

## Self-Review Notes

- Spec coverage (Plan 2 scope): MCP tool family ✓ (T3, with lore_promote deliberately deferred and the deviation noted), related_lore ✓ (T4), capture ✓ (T2), Claude Code packaging ✓ (T5), Cursor/Codex packaging + per-host init ✓ (T6). AnchorMatches export (T1) is enabling refactor.
- Deviations from spec, both justified: (1) no SKILL.md — the repo's own A/B data (plugin/README.md) shows skills collapse adoption; the deliberate channel ships as slash commands + always-visible notes + MCP descriptions instead; (2) lore_promote not exposed over MCP — destructive file move stays human/CLI for v1.
- Type consistency: loreOpen/formatRecords shared across T3–T4; loreHostContract shared across T5–T6 surfaces via the plan's contract text.
- Known risks: Stop-hook payload field names vary by host version — capture is deliberately schema-tolerant (T2 freeform fallback); mcpserver tests avoid MCP transport by testing text helpers directly, matching existing test style.
