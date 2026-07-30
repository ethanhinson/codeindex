# Lore Knowledge-Graph Edges Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a free-form record→record `related:` edge to lore, with backlinks and a full-trace graph walk, and surface attached knowledge in the CLI `impact` command (not just MCP), plus a graph-health report in `lore doctor`.

**Architecture:** `related:` is authored in YAML frontmatter (like `blocked_by`/`tags`), indexed into a new `lore_links` table in `lore.db` (never `graph.db` — honors dec-01KYR17XECDN2T35W7ERZ932Y8). A shared BFS `Trace` primitive in `internal/lore/index` walks the record graph cycle-safely with configurable depth; one shared `RelatedLoreBlock` formatter renders the "Related lore" block for both the CLI `impact` path and the MCP tools.

**Tech Stack:** Go, SQLite (mattn/go-sqlite3, no FTS5), gopkg.in/yaml.v3.

## Global Constraints

- **No graph.db coupling.** Record→record edges live in `.codeindex/lore.db` only; the code→knowledge bridge in `impact` is a query-time join across the anchor, never a schema link.
- **Lore must never break code navigation.** Every lore error path in an `impact`/`callers`/`callees` flow collapses to empty output — the blast-radius result must still print.
- Records are Markdown + YAML frontmatter; `.lore/` files are the source of truth, `lore.db` is derived and rebuildable.
- Go module path is `codeindex`; internal packages import as `codeindex/internal/...`.
- Run tests with `go test ./...` from repo root `/Users/ethanhinson/codeindex`.

---

### Task 1: `related` field on the record model

**Files:**
- Modify: `internal/lore/record.go`
- Test: `internal/lore/record_test.go`

**Interfaces:**
- Produces: `lore.Record.Related []string` (list of id-or-slug strings); the YAML key is `related` (flow list, omitempty).

- [ ] **Step 1: Write the failing test**

Add to `internal/lore/record_test.go`:

```go
func TestRelatedRoundTrip(t *testing.T) {
	src := []byte("---\n" +
		"id: itm-X\n" +
		"title: A\n" +
		"date: \"2026-07-30\"\n" +
		"related: [dec-Y, some-slug]\n" +
		"---\n\nbody\n")
	r, err := Parse(src, TypeItem)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Related) != 2 || r.Related[0] != "dec-Y" || r.Related[1] != "some-slug" {
		t.Fatalf("Related = %v", r.Related)
	}
	out, err := r.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Parse(out, TypeItem)
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Related) != 2 || r2.Related[0] != "dec-Y" {
		t.Fatalf("round-trip Related = %v", r2.Related)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/lore/ -run TestRelatedRoundTrip -v`
Expected: FAIL (compile error: `r.Related` undefined).

- [ ] **Step 3: Implement the field**

In `internal/lore/record.go`:

- Add to the `Record` struct (after `BlockedBy []string`):
```go
	Related      []string
```
- Add to the `wire` struct (after the `BlockedBy` line):
```go
	Related      []string            `yaml:"related,omitempty,flow"`
```
- Add `"related"` to the `knownKeys` slice (after `"blocked_by"`).
- In `Parse`, add `Related: w.Related,` to the `Record{...}` literal (alongside `BlockedBy: w.BlockedBy`).
- In `Marshal`, add `Related: r.Related,` to the `wire{...}` literal (alongside `BlockedBy: r.BlockedBy`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/lore/ -run 'TestRelatedRoundTrip|TestKnownKeysCoversWireFields' -v`
Expected: PASS (both — `TestKnownKeysCoversWireFields` confirms `related` is covered).

- [ ] **Step 5: Commit**

```bash
git add internal/lore/record.go internal/lore/record_test.go
git commit -m "lore: add related field to record model"
```

---

### Task 2: `lore_links` table + load into StoredRecord

**Files:**
- Modify: `internal/lore/index/store.go`
- Test: `internal/lore/index/store_test.go`

**Interfaces:**
- Consumes: `lore.Record.Related` (Task 1).
- Produces: `lore_links(record_id, related_id)` rows; `StoredRecord.Related` populated by `loadChildren` (the field is the embedded `lore.Record.Related`). `schemaVersion` becomes `3`.

- [ ] **Step 1: Write the failing test**

Add to `internal/lore/index/store_test.go`:

```go
func TestLoreLinksUpsertAndLoad(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "lore.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rec := lore.Record{ID: "itm-A", Type: lore.TypeItem, Title: "A",
		Related: []string{"dec-B", "some-slug"}}
	if err := st.Upsert(rec, "repo", "items/a.md"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := st.Get("itm-A")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if len(got.Related) != 2 || got.Related[0] != "dec-B" || got.Related[1] != "some-slug" {
		t.Fatalf("Related = %v", got.Related)
	}
	// Re-upsert with fewer links replaces, not appends.
	rec.Related = []string{"dec-B"}
	if err := st.Upsert(rec, "repo", "items/a.md"); err != nil {
		t.Fatal(err)
	}
	got, _, _ = st.Get("itm-A")
	if len(got.Related) != 1 {
		t.Fatalf("after re-upsert Related = %v", got.Related)
	}
}
```

(If `store_test.go` lacks the imports `path/filepath` and `codeindex/internal/lore`, add them.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/lore/index/ -run TestLoreLinksUpsertAndLoad -v`
Expected: FAIL (no `lore_links` table / `got.Related` empty).

- [ ] **Step 3: Implement the schema + read/write**

In `internal/lore/index/store.go`:

- Bump the version and update its comment:
```go
// schemaVersion 3 adds lore_links (record→record related edges) in the
// knowledge-graph-edges work.
const schemaVersion = 3
```
- Add to the `schema` const string (after the `lore_tags` index line):
```go
CREATE TABLE IF NOT EXISTS lore_links (record_id TEXT NOT NULL, related_id TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS idx_lore_links_rec ON lore_links(record_id);
CREATE INDEX IF NOT EXISTS idx_lore_links_rel ON lore_links(related_id);
```
- In `Open`, add `"lore_links"` to the wipe-on-mismatch table slice (the list containing `"lore_files", "lore_records", ...`).
- In `Upsert`, add `"lore_links"` to the delete-children slice (`[]string{"lore_anchors", "lore_refs", "lore_blocked", "lore_tags"}` → append `"lore_links"`), then after the `lore_blocked` insert loop add:
```go
	for _, rel := range r.Related {
		if _, err := tx.Exec(`INSERT INTO lore_links(record_id,related_id) VALUES(?,?)`,
			r.ID, rel); err != nil {
			return err
		}
	}
```
- In `DeleteByFile`, add `"lore_links"` to the per-id delete slice (`[]string{"lore_records", "lore_anchors", "lore_refs", "lore_blocked", "lore_tags"}`).
- In `loadChildren`, after the `lore_tags` load block (before the final `return nil`), add:
```go
	rows, err = s.db.Query(`SELECT related_id FROM lore_links WHERE record_id=?`, r.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var rel string
		if err := rows.Scan(&rel); err != nil {
			rows.Close()
			return err
		}
		r.Related = append(r.Related, rel)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/lore/index/ -run TestLoreLinksUpsertAndLoad -v && go test ./internal/lore/index/`
Expected: PASS (targeted test + whole package, confirming the schema bump didn't break existing store tests).

- [ ] **Step 5: Commit**

```bash
git add internal/lore/index/store.go internal/lore/index/store_test.go
git commit -m "lore/index: persist related edges in lore_links (schema v3)"
```

---

### Task 3: resolution + backlinks + `Trace` graph walk

**Files:**
- Create: `internal/lore/index/trace.go`
- Test: `internal/lore/index/trace_test.go`

**Interfaces:**
- Consumes: `StoredRecord.Related`, `.Supersedes`, `.SupersededBy`, `.BlockedBy`, `.Title` (Tasks 1–2); `lore.Slug` from `codeindex/internal/lore`.
- Produces:
  - `func ResolveID(recs []StoredRecord, value string) (string, bool)` — value is an id or slug; returns canonical id. Exact id match wins; else exact match against `lore.Slug(title)`; ambiguous slug → `("", false)`.
  - `type Reached struct { ID string; Distance int; ViaEdge string; ViaParent string }`
  - `type TraceOpts struct { Depth int; Cap int }` — `Depth < 0` means unbounded (full trace); `Cap <= 0` defaults to 200.
  - `func Trace(recs []StoredRecord, startID string, opts TraceOpts) []Reached` — BFS, cycle-safe, excludes the start node from results, shortest-distance-wins.
  - `func Backlinks(recs []StoredRecord, id string) []StoredRecord` — direct (depth-1) inbound records.

- [ ] **Step 1: Write the failing tests**

Create `internal/lore/index/trace_test.go`:

```go
package index

import (
	"testing"

	"codeindex/internal/lore"
)

func recWith(id, title string, related, blockedBy []string, supersedes string) StoredRecord {
	return StoredRecord{Record: lore.Record{
		ID: id, Title: title, Related: related, BlockedBy: blockedBy, Supersedes: supersedes,
	}}
}

func TestResolveID(t *testing.T) {
	recs := []StoredRecord{recWith("dec-1", "No Graph Coupling", nil, nil, "")}
	if id, ok := ResolveID(recs, "dec-1"); !ok || id != "dec-1" {
		t.Fatalf("by id: %q %v", id, ok)
	}
	if id, ok := ResolveID(recs, lore.Slug("No Graph Coupling")); !ok || id != "dec-1" {
		t.Fatalf("by slug: %q %v", id, ok)
	}
	if _, ok := ResolveID(recs, "nope"); ok {
		t.Fatalf("missing should not resolve")
	}
}

func TestTraceFullAndDepth(t *testing.T) {
	// A -> B -> C, and a cycle A <-> B via B.related back to A.
	recs := []StoredRecord{
		recWith("A", "a", []string{"B"}, nil, ""),
		recWith("B", "b", []string{"C", "A"}, nil, ""),
		recWith("C", "c", nil, nil, ""),
	}
	full := Trace(recs, "A", TraceOpts{Depth: -1})
	dist := map[string]int{}
	for _, r := range full {
		dist[r.ID] = r.Distance
	}
	if dist["B"] != 1 || dist["C"] != 2 {
		t.Fatalf("distances = %v", dist)
	}
	if _, ok := dist["A"]; ok {
		t.Fatalf("start node must be excluded")
	}
	// Depth 1 reaches only B.
	d1 := Trace(recs, "A", TraceOpts{Depth: 1})
	if len(d1) != 1 || d1[0].ID != "B" {
		t.Fatalf("depth-1 = %+v", d1)
	}
}

func TestTraceCap(t *testing.T) {
	var recs []StoredRecord
	// chain of 10 nodes n0->n1->...->n9
	for i := 0; i < 10; i++ {
		var rel []string
		if i < 9 {
			rel = []string{itoa(i + 1)}
		}
		recs = append(recs, recWith(itoa(i), itoa(i), rel, nil, ""))
	}
	got := Trace(recs, "0", TraceOpts{Depth: -1, Cap: 3})
	if len(got) != 3 {
		t.Fatalf("cap should bound reached to 3, got %d", len(got))
	}
}

func TestBacklinks(t *testing.T) {
	recs := []StoredRecord{
		recWith("A", "a", []string{"B"}, nil, ""),
		recWith("C", "c", nil, []string{"B"}, ""),      // blocked_by B
		recWith("D", "d", nil, nil, "B"),                // supersedes B
		recWith("B", "b", nil, nil, ""),
	}
	bl := Backlinks(recs, "B")
	ids := map[string]bool{}
	for _, r := range bl {
		ids[r.ID] = true
	}
	if !ids["A"] || !ids["C"] || !ids["D"] {
		t.Fatalf("backlinks = %v", ids)
	}
}

func itoa(i int) string { return string(rune('n')) + string(rune('0'+i)) }
```

Note: `itoa` above yields distinct 2-char ids (`n0`..`n9`); the chain test uses them consistently via `recWith(itoa(i), ...)` and `rel = []string{itoa(i+1)}` — the start id passed to `Trace` must match, so change the `Trace(recs, "0", ...)` call to `Trace(recs, itoa(0), ...)`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/lore/index/ -run 'TestResolveID|TestTrace|TestBacklinks' -v`
Expected: FAIL (compile error: `ResolveID`/`Trace`/`Backlinks` undefined).

- [ ] **Step 3: Implement `trace.go`**

Create `internal/lore/index/trace.go`:

```go
package index

import "codeindex/internal/lore"

// Reached is one record found by Trace: how far from the start, along which
// edge, and from which parent record.
type Reached struct {
	ID        string
	Distance  int
	ViaEdge   string // "related" | "supersedes" | "blocked_by"
	ViaParent string
}

// TraceOpts bounds a walk. Depth < 0 is unbounded (full trace). Cap <= 0
// defaults to 200 total reached records.
type TraceOpts struct {
	Depth int
	Cap   int
}

// ResolveID maps an id-or-slug to a canonical record id. Exact id wins; else an
// exact, unambiguous match against lore.Slug(title). Returns ("", false) when
// missing or ambiguous.
func ResolveID(recs []StoredRecord, value string) (string, bool) {
	for _, r := range recs {
		if r.ID == value {
			return r.ID, true
		}
	}
	var hit string
	n := 0
	for _, r := range recs {
		if lore.Slug(r.Title) == value {
			hit = r.ID
			n++
		}
	}
	if n == 1 {
		return hit, true
	}
	return "", false
}

// neighbors returns the undirected record-graph neighbors of rec, each tagged
// with the edge type it was reached by. related is treated as bidirectional;
// supersedes/superseded_by and blocked_by (and their reverses) are included.
func neighbors(recs []StoredRecord, byID map[string]StoredRecord, rec StoredRecord) []Reached {
	var out []Reached
	add := func(target, edge string) {
		if id, ok := ResolveID(recs, target); ok {
			out = append(out, Reached{ID: id, ViaEdge: edge, ViaParent: rec.ID})
		}
	}
	for _, rel := range rec.Related {
		add(rel, "related")
	}
	if rec.Supersedes != "" {
		add(rec.Supersedes, "supersedes")
	}
	if rec.SupersededBy != "" {
		add(rec.SupersededBy, "supersedes")
	}
	for _, b := range rec.BlockedBy {
		add(b, "blocked_by")
	}
	// Reverse edges: any record pointing at rec.
	for _, other := range recs {
		if other.ID == rec.ID {
			continue
		}
		for _, rel := range other.Related {
			if id, ok := ResolveID(recs, rel); ok && id == rec.ID {
				out = append(out, Reached{ID: other.ID, ViaEdge: "related", ViaParent: rec.ID})
			}
		}
		if other.Supersedes == rec.ID || other.SupersededBy == rec.ID {
			out = append(out, Reached{ID: other.ID, ViaEdge: "supersedes", ViaParent: rec.ID})
		}
		for _, b := range other.BlockedBy {
			if b == rec.ID {
				out = append(out, Reached{ID: other.ID, ViaEdge: "blocked_by", ViaParent: rec.ID})
			}
		}
	}
	return out
}

// Trace walks the record graph breadth-first from startID, cycle-safe. The
// start node is excluded from the result; shortest distance wins.
func Trace(recs []StoredRecord, startID string, opts TraceOpts) []Reached {
	cap := opts.Cap
	if cap <= 0 {
		cap = 200
	}
	byID := map[string]StoredRecord{}
	for _, r := range recs {
		byID[r.ID] = r
	}
	if _, ok := byID[startID]; !ok {
		return nil
	}
	seen := map[string]bool{startID: true}
	var out []Reached
	frontier := []Reached{{ID: startID, Distance: 0}}
	for len(frontier) > 0 && len(out) < cap {
		var next []Reached
		for _, cur := range frontier {
			if opts.Depth >= 0 && cur.Distance >= opts.Depth {
				continue
			}
			for _, nb := range neighbors(recs, byID, byID[cur.ID]) {
				if seen[nb.ID] {
					continue
				}
				seen[nb.ID] = true
				reached := Reached{ID: nb.ID, Distance: cur.Distance + 1,
					ViaEdge: nb.ViaEdge, ViaParent: cur.ID}
				out = append(out, reached)
				next = append(next, reached)
				if len(out) >= cap {
					return out
				}
			}
		}
		frontier = next
	}
	return out
}

// Backlinks returns records that directly reference id (depth-1 inbound):
// via related, supersedes/superseded_by, or blocked_by.
func Backlinks(recs []StoredRecord, id string) []StoredRecord {
	var out []StoredRecord
	for _, r := range recs {
		hit := r.Supersedes == id || r.SupersededBy == id
		for _, rel := range r.Related {
			if rid, ok := ResolveID(recs, rel); ok && rid == id {
				hit = true
			}
		}
		for _, b := range r.BlockedBy {
			if b == id {
				hit = true
			}
		}
		if hit {
			out = append(out, r)
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Fix the `Trace(recs, "0", ...)` call in the cap test to `Trace(recs, itoa(0), ...)` per the Step 1 note, then:
Run: `go test ./internal/lore/index/ -run 'TestResolveID|TestTrace|TestBacklinks' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/lore/index/trace.go internal/lore/index/trace_test.go
git commit -m "lore/index: add ResolveID, Trace graph walk, and Backlinks"
```

---

### Task 4: shared `RelatedLoreBlock` formatter (CLI + MCP)

**Files:**
- Create: `internal/lore/index/relatedblock.go`
- Test: `internal/lore/index/relatedblock_test.go`
- Modify: `internal/mcpserver/lore_tools.go` (make `relatedLoreBlock` a thin wrapper)

**Interfaces:**
- Consumes: `RecordsForAnchor` (match.go), `Trace` (Task 3).
- Produces: `func RelatedLoreBlock(recs []StoredRecord, symbol string, depth int) string` — returns the "Related lore" text block (with leading blank lines and header), or `""` when nothing is anchored. `depth` is passed straight to `Trace` (`< 0` = full trace). Ordering: distance ascending, then active/open before others; total entries capped at 5 for the impact block; a "(+N more)" line if truncated. Each entry annotated with its distance when > 0.

- [ ] **Step 1: Write the failing test**

Create `internal/lore/index/relatedblock_test.go`:

```go
package index

import (
	"strings"
	"testing"

	"codeindex/internal/lore"
)

func TestRelatedLoreBlock(t *testing.T) {
	recs := []StoredRecord{
		{Record: lore.Record{ID: "dec-1", Type: lore.TypeDecision, Title: "Anchored",
			Status: "active", Anchors: []lore.Anchor{{Symbol: "Foo"}}, Related: []string{"note-2"}}},
		{Record: lore.Record{ID: "note-2", Type: lore.TypeNote, Title: "Linked note"}},
	}
	block := RelatedLoreBlock(recs, "Foo", -1)
	if !strings.Contains(block, "dec-1") {
		t.Fatalf("expected anchored record in block:\n%s", block)
	}
	if !strings.Contains(block, "note-2") {
		t.Fatalf("expected one-hop related record via full trace:\n%s", block)
	}
	if RelatedLoreBlock(recs, "Unanchored", -1) != "" {
		t.Fatalf("no anchor match must yield empty block")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/lore/index/ -run TestRelatedLoreBlock -v`
Expected: FAIL (`RelatedLoreBlock` undefined).

- [ ] **Step 3: Implement `relatedblock.go`**

Create `internal/lore/index/relatedblock.go`:

```go
package index

import (
	"fmt"
	"sort"
	"strings"
)

const relatedBlockCap = 5

// RelatedLoreBlock renders the "Related lore" block for a code query: records
// anchored to symbol (distance 0) plus their graph neighbors out to depth
// (depth < 0 = full trace). Returns "" when nothing is anchored. Ordering is
// distance ascending, then active/open first. Capped at relatedBlockCap entries.
func RelatedLoreBlock(recs []StoredRecord, symbol string, depth int) string {
	roots := RecordsForAnchor(recs, symbol)
	if len(roots) == 0 {
		return ""
	}
	byID := map[string]StoredRecord{}
	for _, r := range recs {
		byID[r.ID] = r
	}
	dist := map[string]int{}
	for _, r := range roots {
		dist[r.ID] = 0
	}
	for _, r := range roots {
		for _, reached := range Trace(recs, r.ID, TraceOpts{Depth: depth}) {
			if d, ok := dist[reached.ID]; !ok || reached.Distance < d {
				dist[reached.ID] = reached.Distance
			}
		}
	}
	type entry struct {
		r StoredRecord
		d int
	}
	var entries []entry
	for id, d := range dist {
		if r, ok := byID[id]; ok {
			entries = append(entries, entry{r, d})
		}
	}
	rank := func(status string) int {
		if status == "active" || status == "open" {
			return 0
		}
		return 1
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].d != entries[j].d {
			return entries[i].d < entries[j].d
		}
		return rank(entries[i].r.Status) < rank(entries[j].r.Status)
	})
	total := len(entries)
	if len(entries) > relatedBlockCap {
		entries = entries[:relatedBlockCap]
	}
	var b strings.Builder
	b.WriteString("\n\nRelated lore (decisions/items/notes for this symbol and its links):\n")
	for _, e := range entries {
		status := e.r.Status
		if status == "" {
			status = "-"
		}
		flag := ""
		if e.r.Stale {
			flag = "  STALE"
		}
		hop := ""
		if e.d > 0 {
			hop = fmt.Sprintf("  (+%d)", e.d)
		}
		fmt.Fprintf(&b, "%s  [%s/%s]  %s%s%s\n", e.r.ID, e.r.Layer, status, e.r.Title, hop, flag)
	}
	if total > relatedBlockCap {
		fmt.Fprintf(&b, "(+%d more)\n", total-relatedBlockCap)
	}
	return b.String()
}
```

- [ ] **Step 4: Refactor the MCP wrapper**

In `internal/mcpserver/lore_tools.go`, replace the body of `relatedLoreBlock` (keep the fail-open contract) so it delegates to the shared formatter. Full trace is the MCP default (`depth = -1`):

```go
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
	return index.RelatedLoreBlock(all, symbol, -1)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/lore/index/ -run TestRelatedLoreBlock -v && go test ./internal/mcpserver/`
Expected: PASS. The existing `TestRelatedLoreBlock` in `lore_tools_test.go` may assert the old header text — if it fails, update its expected substring to match the new header/format (it is testing the same behavior through the wrapper).

- [ ] **Step 6: Commit**

```bash
git add internal/lore/index/relatedblock.go internal/lore/index/relatedblock_test.go internal/mcpserver/lore_tools.go internal/mcpserver/lore_tools_test.go
git commit -m "lore/index: shared RelatedLoreBlock formatter; MCP delegates to it"
```

---

### Task 5: CLI `impact` surfaces related lore (`--related-depth`)

**Files:**
- Modify: `cmd/codeindex/main.go` (the `impact` case + `runImpact`)
- Test: `cmd/codeindex/lore_test.go` (add a CLI-level test) — or `main_test.go` if that is where CLI tests live; use whichever exists.

**Interfaces:**
- Consumes: `index.RelatedLoreBlock` (Task 4), `loreReindex` (lore.go), `query.ImpactText`.
- Produces: `runImpact(root, name string, limit, relatedDepth int)` — appends the related-lore block, fail-open. CLI flag `--related-depth N|all`; default `2`.

- [ ] **Step 1: Write the failing test**

Determine the CLI test file: `ls cmd/codeindex/*_test.go`. Add this test to whichever test file exercises lore commands (it needs the existing helpers there; if a `withTempRepo`-style helper exists, reuse it — otherwise build a temp repo inline as other tests in that file do):

```go
func TestRunImpactAppendsRelatedLore(t *testing.T) {
	// Fail-open guarantee: impact must not error when lore is absent.
	root := t.TempDir()
	// A minimal repo with no .lore/ and no graph — runImpact should still
	// return without surfacing a lore error (block is simply empty).
	err := runImpact(root, "NoSuchSymbol", 50, 2)
	if err == nil {
		return // acceptable: no graph yet may error on ImpactText; both paths tested below
	}
	// If ImpactText errors due to missing graph, that is unrelated to lore;
	// assert the error is not a lore error.
	if strings.Contains(err.Error(), "lore") {
		t.Fatalf("lore must not break impact: %v", err)
	}
}
```

(This test documents the fail-open contract; the substantive related-lore rendering is covered by Task 4's unit test on the shared formatter.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/codeindex/ -run TestRunImpactAppendsRelatedLore -v`
Expected: FAIL (compile error: `runImpact` takes 3 args, test passes 4).

- [ ] **Step 3: Implement**

In `cmd/codeindex/main.go`:

- Change the `impact` case to parse `--related-depth` and pass it through:
```go
	case "impact":
		if len(os.Args) < 4 {
			fatal(fmt.Errorf("usage: codeindex impact <repo-root> <symbol> [--limit N] [--related-depth N|all]"))
		}
		limit := 50
		relatedDepth := 2
		for i := 4; i < len(os.Args)-1; i++ {
			switch os.Args[i] {
			case "--limit":
				fmt.Sscanf(os.Args[i+1], "%d", &limit)
			case "--related-depth":
				if os.Args[i+1] == "all" {
					relatedDepth = -1
				} else {
					fmt.Sscanf(os.Args[i+1], "%d", &relatedDepth)
				}
			}
		}
		if err := runImpact(root, os.Args[3], limit, relatedDepth); err != nil {
			fatal(err)
		}
```

- Rewrite `runImpact` to append the block, fail-open:
```go
// runImpact prints the counts-first blast-radius summary, then any related
// lore. Lore must never break navigation: a lore failure drops the block.
func runImpact(root, name string, limit, relatedDepth int) error {
	out, err := query.ImpactText(root, name, limit)
	if err != nil {
		return err
	}
	fmt.Print(out)
	fmt.Print(relatedLoreForImpact(root, name, relatedDepth))
	return nil
}

// relatedLoreForImpact returns the related-lore block or "" on any error.
func relatedLoreForImpact(root, symbol string, depth int) string {
	_, st, _, err := loreReindex(root)
	if err != nil {
		return ""
	}
	defer st.Close()
	all, err := st.All()
	if err != nil {
		return ""
	}
	return index.RelatedLoreBlock(all, symbol, depth)
}
```

- Ensure `cmd/codeindex/main.go` imports `codeindex/internal/lore/index` (it may already via other code; add to the import block if not).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/codeindex/ -run TestRunImpactAppendsRelatedLore -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 5: Manual dogfood check**

Run: `go run ./cmd/codeindex impact . StaleRecords --related-depth all`
Expected: the blast-radius summary followed by a "Related lore" block that includes the `no graph.db coupling` decision (anchored to `internal/lore/index/`) and, via trace, the records it links to.

- [ ] **Step 6: Commit**

```bash
git add cmd/codeindex/main.go cmd/codeindex/*_test.go
git commit -m "cli: impact surfaces related lore with --related-depth (fail-open)"
```

---

### Task 6: `lore related` subcommand + backlinks in `lore show`

**Files:**
- Modify: `cmd/codeindex/lore.go` (dispatch, usage, new `loreRelated`, `loreShow`)
- Test: `cmd/codeindex/lore_test.go`

**Interfaces:**
- Consumes: `index.Trace`, `index.Backlinks`, `index.ResolveID` (Task 3); `loreReindex`, `stringFlag` (lore.go).
- Produces: `codeindex lore <repo> related <id> [--depth N|all]` (default full trace); `lore show` gains a "Referenced by:" section.

- [ ] **Step 1: Write the failing test**

Add to `cmd/codeindex/lore_test.go` (reuse the file's existing temp-repo + `lore add` helpers; if the pattern is to shell through `runLore`, follow it):

```go
func TestLoreRelatedAndBacklinks(t *testing.T) {
	root := t.TempDir()
	if err := loreInitScaffold(root, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	// dec-parent linked-from itm-child via related.
	mustAdd := func(args ...string) {
		if err := loreAdd(root, args, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	mustAdd("decision", "--title", "Parent Decision")
	// find its id
	_, st, _, err := loreReindex(root)
	if err != nil {
		t.Fatal(err)
	}
	all, _ := st.All()
	st.Close()
	var parentID string
	for _, r := range all {
		if r.Title == "Parent Decision" {
			parentID = r.ID
		}
	}
	mustAdd("note", "--title", "Child Note", "--related", parentID)

	var out strings.Builder
	if err := loreRelated(root, []string{parentID, "--depth", "all"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Child Note") {
		t.Fatalf("related/backlinks should reach Child Note:\n%s", out.String())
	}
}
```

Note: this test requires `lore add` to accept `--related` (multiFlag). That wiring is Step 3.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/codeindex/ -run TestLoreRelatedAndBacklinks -v`
Expected: FAIL (`loreRelated` undefined; `--related` not parsed).

- [ ] **Step 3: Implement**

In `cmd/codeindex/lore.go`:

- Wire `--related` into `lore add`. In the record-building function (around line 393 where `Tags`/`BlockedBy` are set), add:
```go
		Related: multiFlag(args, "--related"),
```
to the `lore.Record{...}` literal.

- Add the subcommand to `loreUsage`:
```go
const loreUsage = "usage: codeindex lore <repo-root> " +
	"<add|show|related|search|for|backlog|promote|supersede|doctor|init|capture|event|sync|push> ..."
```
- Add a dispatch case in `runLore` (after `case "show":`):
```go
	case "related":
		return loreRelated(root, args[1:], out)
```
- Implement `loreRelated`:
```go
// loreRelated prints out-links and back-links for a record, tracing the record
// graph to the requested depth (default: full trace).
func loreRelated(root string, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: codeindex lore <repo> related <id> [--depth N|all]")
	}
	depth := -1 // full trace by default
	if v := stringFlag(args, "--depth"); v != "" && v != "all" {
		fmt.Sscanf(v, "%d", &depth)
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
	id, ok := index.ResolveID(all, args[0])
	if !ok {
		return fmt.Errorf("no record %q", args[0])
	}
	byID := map[string]index.StoredRecord{}
	for _, r := range all {
		byID[r.ID] = r
	}
	fmt.Fprintf(out, "%s\n", id)
	for _, reached := range index.Trace(all, id, index.TraceOpts{Depth: depth}) {
		r := byID[reached.ID]
		fmt.Fprintf(out, "  d%d  %-11s  %s/%s  %s\n", reached.Distance, reached.ViaEdge,
			r.Type, orDash(r.Status), r.Title)
	}
	return nil
}
```

- Add a "Referenced by:" section to `loreShow`, before the final `return nil` (after the body is written):
```go
	bl := index.Backlinks(all, r.ID)
	if len(bl) > 0 {
		fmt.Fprintln(out, "\nReferenced by:")
		for _, b := range bl {
			fmt.Fprintf(out, "  %s  %s/%s  %s\n", b.ID, b.Type, orDash(b.Status), b.Title)
		}
	}
```
This requires `loreShow` to have the full record set. `loreShow` currently only calls `st.Get`. Add after `defer st.Close()`:
```go
	all, err := st.All()
	if err != nil {
		return err
	}
```
(Place it before the `st.Get` call; reuse `all` for backlinks.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/codeindex/ -run 'TestLoreRelatedAndBacklinks' -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 5: Commit**

```bash
git add cmd/codeindex/lore.go cmd/codeindex/lore_test.go
git commit -m "cli: lore related subcommand + Referenced-by in lore show; add --related to add"
```

---

### Task 7: graph-health in `lore doctor`

**Files:**
- Modify: `cmd/codeindex/lore.go` (`loreDoctor`)
- Test: `cmd/codeindex/lore_test.go`

**Interfaces:**
- Consumes: `index.ResolveID` (Task 3); the `all []index.StoredRecord` and `byID` already built in `loreDoctor`.
- Produces: doctor reports `dangling-link`, `orphan`, and a `graph:` density summary line.

- [ ] **Step 1: Write the failing test**

Add to `cmd/codeindex/lore_test.go`:

```go
func TestLoreDoctorGraphHealth(t *testing.T) {
	root := t.TempDir()
	if err := loreInitScaffold(root, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	// A note with a dangling related link and no anchors => dangling + orphan-ish.
	if err := loreAdd(root, []string{"note", "--title", "Floating", "--related", "itm-DOESNOTEXIST"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := loreDoctor(root, nil, &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "dangling-link") {
		t.Fatalf("expected dangling-link finding:\n%s", s)
	}
	if !strings.Contains(s, "graph:") {
		t.Fatalf("expected graph density summary:\n%s", s)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/codeindex/ -run TestLoreDoctorGraphHealth -v`
Expected: FAIL (no `dangling-link` / `graph:` output).

- [ ] **Step 3: Implement**

In `cmd/codeindex/lore.go`, inside `loreDoctor`, within the `for _, r := range all` loop (alongside the existing `blocked_by` dangling check), add a dangling-`related` check:
```go
		for _, rel := range r.Related {
			if _, ok := index.ResolveID(all, rel); !ok {
				fmt.Fprintf(out, "dangling-link  %s  related %s\n", r.ID, rel)
				findings++
			}
		}
```

Then, after the loop (before the `if findings == 0` block), add orphan detection and the density summary. Note the loop variable is named `outDeg` to avoid shadowing the `out io.Writer` parameter:
```go
	edges := 0
	orphans := 0
	for _, r := range all {
		outDeg := len(r.Related) + len(r.BlockedBy)
		if r.Supersedes != "" {
			outDeg++
		}
		edges += outDeg
		connected := outDeg > 0 || r.SupersededBy != "" || len(index.Backlinks(all, r.ID)) > 0
		if !connected && len(r.Anchors) == 0 {
			fmt.Fprintf(out, "orphan  %s  %s\n", r.ID, r.Title)
			orphans++
			findings++
		}
	}
	density := 0.0
	if len(all) > 0 {
		density = float64(edges) / float64(len(all))
	}
	fmt.Fprintf(out, "graph: %d records, %d edges, %.2f edges/record, %d orphans\n",
		len(all), edges, density, orphans)
```

(The `graph:` summary line is informational and always printed; it is not counted as a finding. Orphans and dangling links ARE findings.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/codeindex/ -run TestLoreDoctorGraphHealth -v`
Expected: PASS.

- [ ] **Step 5: Full suite + dogfood**

Run: `go test ./... && go run ./cmd/codeindex lore . doctor`
Expected: all green; doctor prints the new `graph:` line for the real `.lore/` graph (baseline measurement for the friction log).

- [ ] **Step 6: Commit**

```bash
git add cmd/codeindex/lore.go cmd/codeindex/lore_test.go
git commit -m "cli: lore doctor reports orphans, dangling links, and graph density"
```

---

## Post-implementation

- [ ] Run `go test ./...` and `go vet ./...` — all clean.
- [ ] Capture the `lore doctor` `graph:` baseline as the first friction-log data point (an ordinary note, `related:`-linked to this plan's decision `dec-01KYTG2C8BPFS0GV787Y8AA4QM`), then link the existing backlog records to measure density improvement.
- [ ] Update the `consolidate backlog filter/sort` item (itm-01KYSZT2F9K5CZYYEXZKFFT2Y7) noting the related-lore-block duplication is now retired.
