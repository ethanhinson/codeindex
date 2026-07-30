# Lore Graph UI — First Slice (Backend): `serve` + Graph Read API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `codeindex serve` HTTP command that exposes a JSON `graph(focus)` neighborhood API joining the code call graph with lore records, plus an embedded static placeholder for the SPA to land in.

**Architecture:** A new `internal/readmodel` package converts the existing `graph.Store` (call graph) and `lore/index.Store` (records) results into JSON-serializable graph nodes/edges — one read model shared by the future web API and CLI. A new `internal/webserver` package serves that read model over `net/http` and hosts embedded SPA assets. `cmd/codeindex` gains a `serve` subcommand following the repo's existing manual-dispatch pattern.

**Tech Stack:** Go (stdlib `net/http`, `encoding/json`, `embed`), existing `internal/graph`, `internal/query`, `internal/lore/index` packages. No new module dependencies.

## Global Constraints

- Go version floor: **1.26.5** (from `go.mod`). Module path: **`codeindex`**.
- **No new `go.mod` dependencies.** Use only stdlib (`net/http`, `encoding/json`, `embed`, `io/fs`) plus existing internal packages.
- Result structs from `internal/graph` and `internal/lore/index` have **no JSON tags** — define dedicated wire structs with `json:` tags in `internal/readmodel`; never marshal the raw graph/lore structs.
- **Read-only.** No handler mutates `.lore/` files or the databases beyond the existing lazy index freshening (`query.Fresh`, `index.Reindex`).
- **Bind to loopback only** (`127.0.0.1`) — local dev tool, no remote hosting.
- Command dispatch follows the existing pattern in `cmd/codeindex/main.go`: `os.Args[1]`=command, `os.Args[2]`=repo root, manual flag parsing, handlers return `error`, `fatal(err)` on non-nil.
- **`related` edge assumption:** the lore record→record `related` edge is NOT on `main` (it lives on PR #1, `feature/lore-knowledge-graph-edges`). Edge kinds in this slice are `calls`, `anchors`, `blocked_by`. The `EdgeKind` enum is open; adding `related` when PR #1 merges is a one-line addition in `RecordNeighborhood` (noted at end).
- Graph output MUST be **deterministic** (sorted nodes/edges) so tests are stable.

## Key identifiers (used across tasks)

Wire types (Task 1), consumed by every later task:

```go
type NodeKind string
const (
	NodeSymbol   NodeKind = "symbol"
	NodeDecision NodeKind = "decision"
	NodeItem     NodeKind = "item"
	NodeNote     NodeKind = "note"
	NodePath     NodeKind = "path"
)

type EdgeKind string
const (
	EdgeCalls     EdgeKind = "calls"
	EdgeAnchors   EdgeKind = "anchors"
	EdgeBlockedBy EdgeKind = "blocked_by"
)

type Node struct {
	ID        string   `json:"id"`
	Kind      NodeKind `json:"kind"`
	Label     string   `json:"label"`
	File      string   `json:"file,omitempty"`
	Line      int      `json:"line,omitempty"`
	Signature string   `json:"signature,omitempty"`
	Status    string   `json:"status,omitempty"`
	Priority  string   `json:"priority,omitempty"`
}

type Edge struct {
	Source string   `json:"source"`
	Target string   `json:"target"`
	Kind   EdgeKind `json:"kind"`
	Conf   string   `json:"conf,omitempty"`
}

type Graph struct {
	Focus string `json:"focus"`
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}
```

Node ID scheme (stable, so symbol nodes coincide whether reached from code or from a lore anchor):
- symbol node: `"sym:" + QName` (QName = `Parent.Name`, or `Name` when top-level)
- lore node: the record ID verbatim (`dec-…`, `itm-…`, `note-…`)
- path node: `"path:" + path`

Reused existing signatures (verified against the worktree, off `main`):
- `func (s *graph.Store) Definitions(name, parent string) ([]graph.Symbol, error)`
- `func (s *graph.Store) Callers(name, parent string) ([]graph.Caller, error)` — `Caller{File,Name,Parent,Signature,Line int,Conf graph.Confidence}`, method `QName() string`
- `func (s *graph.Store) Callees(name, parent string) ([]graph.Callee, error)` — `Callee{Name,DefParent,CallLine,Conf,DefFile,DefLine int}`, method `QName() string`
- `graph.Symbol{File,Name,Parent,Kind,Signature,StartLine,EndLine int}`, method `QName() string`
- `func graph.Open(path string) (*graph.Store, error)`
- `func query.Fresh(root string) (query.FreshInfo, error)`
- `func query.SplitAnchor(anchor string) (name, parent string)`
- `func engine.Build(root, dbPath string) (engine.Stats, error)` (tests only)
- `func lore.NewLayout(root string) (lore.Layout, error)`
- `func loreindex.Reindex(l lore.Layout, dbPath string) (*loreindex.Store, loreindex.Report, error)`
- `func (s *loreindex.Store) All() ([]loreindex.StoredRecord, error)`
- `func loreindex.RecordsForAnchor(recs []loreindex.StoredRecord, q string) []loreindex.StoredRecord`
- `loreindex.StoredRecord` embeds `lore.Record` (fields `ID, Type, Title, Status, Priority, BlockedBy []string, Anchors []lore.Anchor, …`); `lore.Anchor{Path, Symbol string}`

## File Structure

- Create `internal/readmodel/model.go` — wire types (above) + id helpers (`symID`, `qname`) + `sortGraph`.
- Create `internal/readmodel/graph.go` — `SymbolNeighborhood`, `AttachAnchoredLore`, `RecordNeighborhood`, `Neighborhood` dispatcher, store-open helpers.
- Create `internal/readmodel/graph_test.go` — tests (with a local `writeTree` fixture helper).
- Create `internal/webserver/server.go` — `New(root, version) http.Handler`, `Run(root, addr, version) error`, `writeJSON`.
- Create `internal/webserver/static.go` — `//go:embed dist` + static file handler.
- Create `internal/webserver/dist/index.html` — placeholder SPA shell (real SPA replaces it in the next plan).
- Create `internal/webserver/server_test.go` — `httptest` coverage.
- Create `cmd/codeindex/serve.go` — `runServe(root, addr string) error`.
- Modify `cmd/codeindex/main.go` — add `case "serve"` + usage string.

---

### Task 1: readmodel wire types + `SymbolNeighborhood`

**Files:**
- Create: `internal/readmodel/model.go`
- Create: `internal/readmodel/graph.go`
- Test: `internal/readmodel/graph_test.go`

**Interfaces:**
- Consumes: `graph.Store` (`Definitions`, `Callers`, `Callees`), `graph.Confidence`.
- Produces: types `Node/Edge/Graph/NodeKind/EdgeKind` (see Key identifiers); `func SymbolNeighborhood(st *graph.Store, name, parent string) (Graph, error)`; helpers `symID(qname string) string`, `qname(name, parent string) string`, `sortGraph(g *Graph)`.

- [ ] **Step 1: Write the failing test**

```go
// internal/readmodel/graph_test.go
package readmodel

import (
	"os"
	"path/filepath"
	"testing"

	"codeindex/internal/engine"
	"codeindex/internal/graph"
)

// writeTree writes files under a fresh temp dir and returns the dir.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func buildStore(t *testing.T, files map[string]string) *graph.Store {
	t.Helper()
	dir := writeTree(t, files)
	db := filepath.Join(dir, "g.db")
	if _, err := engine.Build(dir, db); err != nil {
		t.Fatal(err)
	}
	st, err := graph.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestSymbolNeighborhood(t *testing.T) {
	st := buildStore(t, map[string]string{
		"a.go": "package p\nfunc Helper(x int) int { return x + 1 }\nfunc A() int { return Helper(1) }\n",
		"b.go": "package p\nfunc B() int { return A() }\n",
	})
	g, err := SymbolNeighborhood(st, "A", "")
	if err != nil {
		t.Fatal(err)
	}
	if g.Focus != "sym:A" {
		t.Fatalf("focus = %q, want sym:A", g.Focus)
	}
	// Expect nodes: A (focus), Helper (callee), B (caller).
	ids := map[string]bool{}
	for _, n := range g.Nodes {
		ids[n.ID] = true
	}
	for _, want := range []string{"sym:A", "sym:Helper", "sym:B"} {
		if !ids[want] {
			t.Errorf("missing node %q; got %v", want, ids)
		}
	}
	// Expect edges: B->A (calls), A->Helper (calls).
	var hasCallerEdge, hasCalleeEdge bool
	for _, e := range g.Edges {
		if e.Source == "sym:B" && e.Target == "sym:A" && e.Kind == EdgeCalls {
			hasCallerEdge = true
		}
		if e.Source == "sym:A" && e.Target == "sym:Helper" && e.Kind == EdgeCalls {
			hasCalleeEdge = true
		}
	}
	if !hasCallerEdge || !hasCalleeEdge {
		t.Errorf("edges wrong: %+v", g.Edges)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/readmodel/ -run TestSymbolNeighborhood -v`
Expected: FAIL — `undefined: SymbolNeighborhood` (package doesn't compile yet).

- [ ] **Step 3: Write `model.go`**

```go
// internal/readmodel/model.go
// Package readmodel converts the codeindex call graph and lore records into a
// single JSON-serializable node/edge graph shared by the web API and CLI.
package readmodel

import "sort"

type NodeKind string

const (
	NodeSymbol   NodeKind = "symbol"
	NodeDecision NodeKind = "decision"
	NodeItem     NodeKind = "item"
	NodeNote     NodeKind = "note"
	NodePath     NodeKind = "path"
)

type EdgeKind string

const (
	EdgeCalls     EdgeKind = "calls"
	EdgeAnchors   EdgeKind = "anchors"
	EdgeBlockedBy EdgeKind = "blocked_by"
)

type Node struct {
	ID        string   `json:"id"`
	Kind      NodeKind `json:"kind"`
	Label     string   `json:"label"`
	File      string   `json:"file,omitempty"`
	Line      int      `json:"line,omitempty"`
	Signature string   `json:"signature,omitempty"`
	Status    string   `json:"status,omitempty"`
	Priority  string   `json:"priority,omitempty"`
}

type Edge struct {
	Source string   `json:"source"`
	Target string   `json:"target"`
	Kind   EdgeKind `json:"kind"`
	Conf   string   `json:"conf,omitempty"`
}

type Graph struct {
	Focus string `json:"focus"`
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

func symID(qn string) string { return "sym:" + qn }

func qname(name, parent string) string {
	if parent != "" {
		return parent + "." + name
	}
	return name
}

// sortGraph orders nodes by ID and edges by (source,target,kind) for
// deterministic output.
func sortGraph(g *Graph) {
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	sort.Slice(g.Edges, func(i, j int) bool {
		a, b := g.Edges[i], g.Edges[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		return a.Kind < b.Kind
	})
}
```

- [ ] **Step 4: Write `SymbolNeighborhood` in `graph.go`**

```go
// internal/readmodel/graph.go
package readmodel

import "codeindex/internal/graph"

// SymbolNeighborhood returns the focus symbol plus its direct callers and
// callees as a node/edge graph.
func SymbolNeighborhood(st *graph.Store, name, parent string) (Graph, error) {
	focusQ := qname(name, parent)
	focusID := symID(focusQ)
	nodes := map[string]Node{focusID: {ID: focusID, Kind: NodeSymbol, Label: focusQ}}

	defs, err := st.Definitions(name, parent)
	if err != nil {
		return Graph{}, err
	}
	if len(defs) > 0 {
		n := nodes[focusID]
		n.File, n.Line, n.Signature = defs[0].File, defs[0].StartLine, defs[0].Signature
		nodes[focusID] = n
	}

	callers, err := st.Callers(name, parent)
	if err != nil {
		return Graph{}, err
	}
	var edges []Edge
	for _, c := range callers {
		id := symID(c.QName())
		if _, ok := nodes[id]; !ok {
			nodes[id] = Node{ID: id, Kind: NodeSymbol, Label: c.QName(), File: c.File, Line: c.Line, Signature: c.Signature}
		}
		edges = append(edges, Edge{Source: id, Target: focusID, Kind: EdgeCalls, Conf: string(c.Conf)})
	}

	callees, err := st.Callees(name, parent)
	if err != nil {
		return Graph{}, err
	}
	for _, c := range callees {
		id := symID(c.QName())
		if _, ok := nodes[id]; !ok {
			nodes[id] = Node{ID: id, Kind: NodeSymbol, Label: c.QName(), File: c.DefFile, Line: c.DefLine}
		}
		edges = append(edges, Edge{Source: focusID, Target: id, Kind: EdgeCalls, Conf: string(c.Conf)})
	}

	g := Graph{Focus: focusID, Edges: edges}
	for _, n := range nodes {
		g.Nodes = append(g.Nodes, n)
	}
	sortGraph(&g)
	return g, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/readmodel/ -run TestSymbolNeighborhood -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/readmodel/model.go internal/readmodel/graph.go internal/readmodel/graph_test.go
git commit -m "readmodel: symbol neighborhood graph over call index"
```

---

### Task 2: Attach anchored lore to a symbol graph

**Files:**
- Modify: `internal/readmodel/graph.go`
- Test: `internal/readmodel/graph_test.go`

**Interfaces:**
- Consumes: `loreindex.StoredRecord`, `loreindex.RecordsForAnchor`.
- Produces: `func AttachAnchoredLore(g *Graph, recs []loreindex.StoredRecord)`; helper `func loreNode(r loreindex.StoredRecord) Node`.

- [ ] **Step 1: Write the failing test**

```go
// append to internal/readmodel/graph_test.go
import (
	"codeindex/internal/lore"
	loreindex "codeindex/internal/lore/index"
)

func openLoreStore(t *testing.T) *loreindex.Store {
	t.Helper()
	s, err := loreindex.Open(filepath.Join(t.TempDir(), "lore.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAttachAnchoredLore(t *testing.T) {
	ls := openLoreStore(t)
	rec := lore.Record{
		ID: "dec-A", Type: lore.TypeDecision, Title: "Keep Helper pure",
		Status: "active", Date: "2026-07-29",
		Anchors: []lore.Anchor{{Symbol: "Helper"}},
	}
	if err := ls.Upsert(rec, "repo", "/repo/.lore/decisions/a.md"); err != nil {
		t.Fatal(err)
	}
	recs, err := ls.All()
	if err != nil {
		t.Fatal(err)
	}

	g := Graph{
		Focus: "sym:Helper",
		Nodes: []Node{{ID: "sym:Helper", Kind: NodeSymbol, Label: "Helper"}},
	}
	AttachAnchoredLore(&g, recs)

	var hasNode, hasEdge bool
	for _, n := range g.Nodes {
		if n.ID == "dec-A" && n.Kind == NodeDecision && n.Label == "Keep Helper pure" {
			hasNode = true
		}
	}
	for _, e := range g.Edges {
		if e.Source == "dec-A" && e.Target == "sym:Helper" && e.Kind == EdgeAnchors {
			hasEdge = true
		}
	}
	if !hasNode || !hasEdge {
		t.Fatalf("anchored lore not attached: nodes=%+v edges=%+v", g.Nodes, g.Edges)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/readmodel/ -run TestAttachAnchoredLore -v`
Expected: FAIL — `undefined: AttachAnchoredLore`.

- [ ] **Step 3: Implement `AttachAnchoredLore` and `loreNode`**

```go
// append to internal/readmodel/graph.go
import loreindex "codeindex/internal/lore/index"

func loreNode(r loreindex.StoredRecord) Node {
	return Node{
		ID:       r.ID,
		Kind:     NodeKind(r.Type),
		Label:    r.Title,
		Status:   r.Status,
		Priority: r.Priority,
	}
}

// AttachAnchoredLore adds, for every symbol node already in g, the lore records
// anchored to that symbol (as lore nodes) and an anchors edge lore->symbol.
func AttachAnchoredLore(g *Graph, recs []loreindex.StoredRecord) {
	present := map[string]bool{}
	var symbols []Node
	for _, n := range g.Nodes {
		present[n.ID] = true
		if n.Kind == NodeSymbol {
			symbols = append(symbols, n)
		}
	}
	for _, sym := range symbols {
		for _, r := range loreindex.RecordsForAnchor(recs, sym.Label) {
			if !present[r.ID] {
				g.Nodes = append(g.Nodes, loreNode(r))
				present[r.ID] = true
			}
			g.Edges = append(g.Edges, Edge{Source: r.ID, Target: sym.ID, Kind: EdgeAnchors})
		}
	}
	sortGraph(g)
}
```

Note: the existing `graph.go` import block must merge the two import lines
(`"codeindex/internal/graph"` and `loreindex "codeindex/internal/lore/index"`)
into one grouped `import (...)` block.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/readmodel/ -run TestAttachAnchoredLore -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/readmodel/graph.go internal/readmodel/graph_test.go
git commit -m "readmodel: attach anchored lore records to symbol nodes"
```

---

### Task 3: `RecordNeighborhood` (lore-focused graph)

**Files:**
- Modify: `internal/readmodel/graph.go`
- Test: `internal/readmodel/graph_test.go`

**Interfaces:**
- Consumes: `loreindex.StoredRecord`, `graph.Store.Definitions`, `query.SplitAnchor`, `lore.Anchor`.
- Produces: `func RecordNeighborhood(rec loreindex.StoredRecord, all []loreindex.StoredRecord, st *graph.Store) (Graph, error)`.

- [ ] **Step 1: Write the failing test**

```go
// append to internal/readmodel/graph_test.go
func TestRecordNeighborhood(t *testing.T) {
	st := buildStore(t, map[string]string{
		"a.go": "package p\nfunc Helper(x int) int { return x + 1 }\n",
	})
	blocker := loreindex.StoredRecord{Record: lore.Record{
		ID: "itm-BLOCK", Type: lore.TypeItem, Title: "prereq", Status: "open",
	}}
	focus := loreindex.StoredRecord{Record: lore.Record{
		ID: "itm-FOCUS", Type: lore.TypeItem, Title: "do the thing", Status: "open",
		Anchors:   []lore.Anchor{{Symbol: "Helper"}, {Path: "a.go"}},
		BlockedBy: []string{"itm-BLOCK"},
	}}
	all := []loreindex.StoredRecord{focus, blocker}

	g, err := RecordNeighborhood(focus, all, st)
	if err != nil {
		t.Fatal(err)
	}
	if g.Focus != "itm-FOCUS" {
		t.Fatalf("focus = %q", g.Focus)
	}
	ids := map[string]NodeKind{}
	for _, n := range g.Nodes {
		ids[n.ID] = n.Kind
	}
	if ids["sym:Helper"] != NodeSymbol {
		t.Errorf("missing anchored symbol node; got %v", ids)
	}
	if ids["path:a.go"] != NodePath {
		t.Errorf("missing anchored path node; got %v", ids)
	}
	if ids["itm-BLOCK"] != NodeItem {
		t.Errorf("missing blocker node; got %v", ids)
	}
	var anchorEdge, blockEdge bool
	for _, e := range g.Edges {
		if e.Source == "itm-FOCUS" && e.Target == "sym:Helper" && e.Kind == EdgeAnchors {
			anchorEdge = true
		}
		if e.Source == "itm-FOCUS" && e.Target == "itm-BLOCK" && e.Kind == EdgeBlockedBy {
			blockEdge = true
		}
	}
	if !anchorEdge || !blockEdge {
		t.Errorf("edges wrong: %+v", g.Edges)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/readmodel/ -run TestRecordNeighborhood -v`
Expected: FAIL — `undefined: RecordNeighborhood`.

- [ ] **Step 3: Implement `RecordNeighborhood`**

```go
// append to internal/readmodel/graph.go — add "codeindex/internal/query"
// to the grouped import block.

// RecordNeighborhood returns a lore record as focus with its anchored symbols
// and paths and its blocked_by items.
func RecordNeighborhood(rec loreindex.StoredRecord, all []loreindex.StoredRecord, st *graph.Store) (Graph, error) {
	g := Graph{Focus: rec.ID, Nodes: []Node{loreNode(rec)}}

	for _, a := range rec.Anchors {
		switch {
		case a.Symbol != "":
			name, parent := query.SplitAnchor(a.Symbol)
			defs, err := st.Definitions(name, parent)
			if err != nil {
				return Graph{}, err
			}
			label := a.Symbol
			node := Node{Kind: NodeSymbol, Label: label}
			if len(defs) > 0 {
				label = defs[0].QName()
				node.Label, node.File, node.Line, node.Signature = label, defs[0].File, defs[0].StartLine, defs[0].Signature
			}
			node.ID = symID(label)
			g.Nodes = append(g.Nodes, node)
			g.Edges = append(g.Edges, Edge{Source: rec.ID, Target: node.ID, Kind: EdgeAnchors})
		case a.Path != "":
			id := "path:" + a.Path
			g.Nodes = append(g.Nodes, Node{ID: id, Kind: NodePath, Label: a.Path, File: a.Path})
			g.Edges = append(g.Edges, Edge{Source: rec.ID, Target: id, Kind: EdgeAnchors})
		}
	}

	byID := map[string]loreindex.StoredRecord{}
	for _, r := range all {
		byID[r.ID] = r
	}
	for _, bid := range rec.BlockedBy {
		if br, ok := byID[bid]; ok {
			g.Nodes = append(g.Nodes, loreNode(br))
		} else {
			g.Nodes = append(g.Nodes, Node{ID: bid, Kind: NodeItem, Label: bid})
		}
		g.Edges = append(g.Edges, Edge{Source: rec.ID, Target: bid, Kind: EdgeBlockedBy})
	}

	sortGraph(&g)
	return g, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/readmodel/ -run TestRecordNeighborhood -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/readmodel/graph.go internal/readmodel/graph_test.go
git commit -m "readmodel: lore record neighborhood (anchors + blocked_by)"
```

---

### Task 4: `Neighborhood` dispatcher + store-open helpers

**Files:**
- Modify: `internal/readmodel/graph.go`
- Test: `internal/readmodel/graph_test.go`

**Interfaces:**
- Consumes: `query.Fresh`, `graph.Open`, `lore.NewLayout`, `loreindex.Reindex`, `loreindex.Store.All`.
- Produces: `func Neighborhood(root, focusID string) (Graph, error)`; unexported `openGraph(root string) (*graph.Store, error)`, `openLore(root string) ([]loreindex.StoredRecord, error)`.

- [ ] **Step 1: Write the failing test**

```go
// append to internal/readmodel/graph_test.go
import "codeindex/internal/lore"

// writeRepo creates a temp repo with both code files and .lore records.
func writeRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("CODEINDEX_HOME", t.TempDir())
	root := writeTree(t, map[string]string{
		"a.go": "package p\nfunc Helper(x int) int { return x + 1 }\nfunc A() int { return Helper(1) }\n",
	})
	rec := lore.Record{
		ID: "dec-A", Type: lore.TypeDecision, Title: "Keep Helper pure",
		Status: "active", Date: "2026-07-29",
		Anchors: []lore.Anchor{{Symbol: "Helper"}},
	}
	b, err := rec.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".lore", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.md"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestNeighborhoodSymbolFocusJoinsLore(t *testing.T) {
	root := writeRepo(t)
	g, err := Neighborhood(root, "sym:Helper")
	if err != nil {
		t.Fatal(err)
	}
	var hasLore bool
	for _, e := range g.Edges {
		if e.Source == "dec-A" && e.Target == "sym:Helper" && e.Kind == EdgeAnchors {
			hasLore = true
		}
	}
	if !hasLore {
		t.Fatalf("expected anchored decision joined to symbol; edges=%+v", g.Edges)
	}
}

func TestNeighborhoodRecordFocus(t *testing.T) {
	root := writeRepo(t)
	g, err := Neighborhood(root, "dec-A")
	if err != nil {
		t.Fatal(err)
	}
	if g.Focus != "dec-A" {
		t.Fatalf("focus = %q", g.Focus)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/readmodel/ -run TestNeighborhood -v`
Expected: FAIL — `undefined: Neighborhood`.

- [ ] **Step 3: Implement dispatcher + helpers**

```go
// append to internal/readmodel/graph.go — add "fmt", "path/filepath",
// "strings", "codeindex/internal/lore" to the grouped import block.

func openGraph(root string) (*graph.Store, error) {
	if _, err := query.Fresh(root); err != nil {
		return nil, err
	}
	return graph.Open(filepath.Join(root, ".codeindex", "graph.db"))
}

func openLore(root string) ([]loreindex.StoredRecord, error) {
	l, err := lore.NewLayout(root)
	if err != nil {
		return nil, err
	}
	st, _, err := loreindex.Reindex(l, filepath.Join(root, ".codeindex", "lore.db"))
	if err != nil {
		return nil, err
	}
	defer st.Close()
	return st.All()
}

// Neighborhood resolves focusID to a symbol or lore record and returns its
// 1-hop neighborhood, joining code and lore.
func Neighborhood(root, focusID string) (Graph, error) {
	st, err := openGraph(root)
	if err != nil {
		return Graph{}, err
	}
	defer st.Close()
	recs, err := openLore(root)
	if err != nil {
		return Graph{}, err
	}

	if strings.HasPrefix(focusID, "dec-") || strings.HasPrefix(focusID, "itm-") || strings.HasPrefix(focusID, "note-") {
		for _, r := range recs {
			if r.ID == focusID {
				return RecordNeighborhood(r, recs, st)
			}
		}
		return Graph{}, fmt.Errorf("record not found: %s", focusID)
	}

	anchor := strings.TrimPrefix(focusID, "sym:")
	name, parent := query.SplitAnchor(anchor)
	g, err := SymbolNeighborhood(st, name, parent)
	if err != nil {
		return Graph{}, err
	}
	AttachAnchoredLore(&g, recs)
	return g, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/readmodel/ -v`
Expected: PASS (all readmodel tests).

- [ ] **Step 5: Commit**

```bash
git add internal/readmodel/graph.go internal/readmodel/graph_test.go
git commit -m "readmodel: Neighborhood dispatcher joins code + lore by focus id"
```

---

### Task 5: `internal/webserver` — health + graph JSON endpoints

**Files:**
- Create: `internal/webserver/server.go`
- Test: `internal/webserver/server_test.go`

**Interfaces:**
- Consumes: `readmodel.Neighborhood`.
- Produces: `func New(root, version string) http.Handler`; `func Run(root, addr, version string) error`; unexported `writeJSON(w http.ResponseWriter, code int, v any)`.

- [ ] **Step 1: Write the failing test**

```go
// internal/webserver/server_test.go
package webserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"codeindex/internal/lore"
)

func writeRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("CODEINDEX_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"),
		[]byte("package p\nfunc Helper(x int) int { return x + 1 }\nfunc A() int { return Helper(1) }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := lore.Record{
		ID: "dec-A", Type: lore.TypeDecision, Title: "Keep Helper pure",
		Status: "active", Date: "2026-07-29",
		Anchors: []lore.Anchor{{Symbol: "Helper"}},
	}
	b, err := rec.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".lore", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.md"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestHealthEndpoint(t *testing.T) {
	srv := httptest.NewServer(New(writeRepo(t), "test"))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" || body["version"] != "test" {
		t.Fatalf("body = %+v", body)
	}
}

func TestGraphEndpoint(t *testing.T) {
	srv := httptest.NewServer(New(writeRepo(t), "test"))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/graph?focus=sym:Helper")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var g struct {
		Focus string `json:"focus"`
		Edges []struct {
			Source, Target, Kind string
		} `json:"edges"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
		t.Fatal(err)
	}
	if g.Focus != "sym:Helper" {
		t.Fatalf("focus = %q", g.Focus)
	}
	var joined bool
	for _, e := range g.Edges {
		if e.Source == "dec-A" && e.Target == "sym:Helper" && e.Kind == "anchors" {
			joined = true
		}
	}
	if !joined {
		t.Fatalf("expected code+lore join edge; edges=%+v", g.Edges)
	}
}

func TestGraphEndpointMissingFocus(t *testing.T) {
	srv := httptest.NewServer(New(writeRepo(t), "test"))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/graph")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/webserver/ -v`
Expected: FAIL — `undefined: New` (package doesn't compile).

- [ ] **Step 3: Implement `server.go`**

```go
// internal/webserver/server.go
// Package webserver serves the codeindex read model over HTTP and hosts the
// embedded SPA. Read-only; bind to loopback only.
package webserver

import (
	"encoding/json"
	"log"
	"net/http"

	"codeindex/internal/readmodel"
)

// New returns the HTTP handler for the read-only graph API and static SPA.
func New(root, version string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "ok", "version": version, "root": root,
		})
	})

	mux.HandleFunc("/api/graph", func(w http.ResponseWriter, r *http.Request) {
		focus := r.URL.Query().Get("focus")
		if focus == "" {
			http.Error(w, "missing required query param: focus", http.StatusBadRequest)
			return
		}
		g, err := readmodel.Neighborhood(root, focus)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, g)
	})

	return mux
}

// Run serves on addr until the process is stopped. addr must be loopback.
func Run(root, addr, version string) error {
	log.Printf("codeindex serve: http://%s (root %s)", addr, root)
	return http.ListenAndServe(addr, New(root, version))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/webserver/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/webserver/server.go internal/webserver/server_test.go
git commit -m "webserver: read-only /api/health and /api/graph endpoints"
```

---

### Task 6: Embedded static SPA placeholder + `serve` subcommand

**Files:**
- Create: `internal/webserver/dist/index.html`
- Create: `internal/webserver/static.go`
- Modify: `internal/webserver/server.go` (mount static handler)
- Create: `cmd/codeindex/serve.go`
- Modify: `cmd/codeindex/main.go` (dispatch + usage)
- Test: `internal/webserver/server_test.go` (static route)

**Interfaces:**
- Consumes: `webserver.Run`, `query.Fresh`.
- Produces: `func runServe(root, addr string) error` (cmd); unexported `staticHandler() http.Handler` (webserver).

- [ ] **Step 1: Create the placeholder SPA shell**

```html
<!-- internal/webserver/dist/index.html -->
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <title>codeindex graph</title>
  </head>
  <body>
    <div id="root">codeindex graph UI — SPA not yet built. API is live at <code>/api/graph?focus=…</code>.</div>
  </body>
</html>
```

- [ ] **Step 2: Write the failing test**

```go
// append to internal/webserver/server_test.go
import "io"

func TestStaticIndexServed(t *testing.T) {
	srv := httptest.NewServer(New(writeRepo(t), "test"))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "codeindex graph UI") {
		t.Fatalf("index body unexpected: %q", string(b))
	}
}
```

Add `"strings"` to the `server_test.go` import block.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/webserver/ -run TestStaticIndexServed -v`
Expected: FAIL — `/` returns 404 (no static handler mounted yet).

- [ ] **Step 4: Implement `static.go` and mount it**

```go
// internal/webserver/static.go
package webserver

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist
var distFS embed.FS

// staticHandler serves the embedded SPA assets from dist/.
func staticHandler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // dist is embedded at build time; this cannot fail in a built binary
	}
	return http.FileServer(http.FS(sub))
}
```

In `server.go`, mount the static handler at `/` as the last route in `New`
(the `/api/` routes win by ServeMux longest-prefix matching):

```go
	mux.Handle("/", staticHandler())

	return mux
```

(Insert the `mux.Handle("/", staticHandler())` line immediately before the
existing `return mux`.)

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/webserver/ -v`
Expected: PASS (all webserver tests).

- [ ] **Step 6: Wire the `serve` subcommand**

```go
// cmd/codeindex/serve.go
package main

import (
	"codeindex/internal/query"
	"codeindex/internal/webserver"
)

// runServe freshens the index, then serves the read-only graph API and SPA.
func runServe(root, addr string) error {
	if _, err := query.Fresh(root); err != nil {
		return err
	}
	return webserver.Run(root, addr, version)
}
```

In `cmd/codeindex/main.go`, add a case to the dispatch switch (near the `mcp`
case):

```go
	case "serve":
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
```

Add `serve` to the usage string in `main()` (the `fmt.Fprintln(os.Stderr, "usage: codeindex <…>")` line): insert `serve|` before `mcp`.

- [ ] **Step 7: Verify the whole binary builds and all tests pass**

Run: `go build ./cmd/codeindex && go test ./...`
Expected: build succeeds; all packages PASS.

- [ ] **Step 8: Smoke-test the running server manually**

Run (in one shell):
```bash
go run ./cmd/codeindex serve "$PWD" --addr 127.0.0.1:7676
```
Run (in another shell):
```bash
curl -s 'http://127.0.0.1:7676/api/health'
curl -s 'http://127.0.0.1:7676/api/graph?focus=sym:Neighborhood'
```
Expected: health returns `{"status":"ok",...}`; graph returns a JSON node/edge
neighborhood for `readmodel.Neighborhood` (symbols + any anchored lore). Stop
the server with Ctrl-C.

- [ ] **Step 9: Commit**

```bash
git add internal/webserver/dist/index.html internal/webserver/static.go internal/webserver/server.go cmd/codeindex/serve.go cmd/codeindex/main.go
git commit -m "serve: embed SPA placeholder and add codeindex serve command"
```

---

## After this plan (next plans in the first slice)

1. **Frontend SPA** (Epics C+F+E): React + Vite + Cytoscape.js building into
   `internal/webserver/dist/`, consuming `/api/graph`. Canvas shell, command
   palette, focus/expand/pivot, typed nodes/edges, inspector peek.
2. **`related` edge** (when PR #1 `feature/lore-knowledge-graph-edges` merges to
   `main`): add `EdgeRelated EdgeKind = "related"` and, in `RecordNeighborhood`,
   loop over `rec.Related` emitting `related` edges to those record nodes —
   mirroring the existing `blocked_by` loop.

## Self-Review

- **Spec coverage:** This plan covers Epic A (serve host + read-model layer) and
  the code+lore join core of Epic B (`graph(focus)` neighborhood + anchors join)
  from the design's first vertical slice. Path-tracing, semantic zoom, saved
  views (Epic F) and the SPA (Epics C/E) are explicitly deferred to later plans.
- **Placeholder scan:** No TBD/TODO in steps; every code step has concrete code.
  The one HTML placeholder is intentional and real (a served file), not a plan
  gap.
- **Type consistency:** `Node/Edge/Graph`, `symID`, `qname`, `sortGraph`,
  `loreNode`, `SymbolNeighborhood`, `AttachAnchoredLore`, `RecordNeighborhood`,
  `Neighborhood`, `openGraph`, `openLore`, `New`, `Run`, `writeJSON`,
  `staticHandler`, `runServe` are used identically wherever referenced. Node ID
  scheme (`sym:` / record-id / `path:`) is consistent across symbol and record
  builders so joins line up.
