<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0003 — Decouple the symbol-graph query layer (headless JSON API + CLI)](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0003-decouple-graph-query-layer.md)**
<!-- docket:backlink:end -->

# Decouple the Symbol-Graph Query Layer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn `serve` into a headless, versioned, symbol-only JSON graph API and delete the lore/web overlay it was coupled to (web app, static hosting, readmodel lore branch), plus two carryovers from change 0002 (the remaining `internal/lore/**` tree and the orphaned `internal/tui/tree` package).

**Architecture:** This is a subtraction + decoupling. We delete the React `web/` app, the webserver's embedded static-file handler and `dist/`, and every lore-coupled path in `internal/readmodel`, leaving a symbol-only read model (`SymbolNeighborhood` + a symbol-only `FullGraph`). The HTTP layer re-points `/api/graph` from `?focus=` (lore-joined `Neighborhood`) to `?symbol=&parent=` (`SymbolNeighborhood`), removes the root `/` static route so unknown paths 404, and wraps every graph response in a top-level `schemaVersion`. Then we delete the now-import-free `internal/lore/**` and `internal/tui` trees. Each task keeps `go build ./...` and `go test ./...` green.

**Tech Stack:** Go (module `codeindex`), `net/http` + `net/http/httptest`, `encoding/json`. No new dependencies.

## Global Constraints

- Module path is `codeindex`; internal imports are `codeindex/internal/...`.
- After **every** task: `go build ./...` and `go test ./...` must be green. Deleting a package means deleting its tests too — no dangling references.
- Node shape on the API is **symbol-only**: `Node{ID, Kind:"symbol", Label, File, Line, Signature, Group}`. No `Status`/`Priority` fields, no lore node kinds.
- API responses carry a **top-level** `schemaVersion`. Use the string constant `SchemaVersion = "1"` (defined in `internal/readmodel`).
- `/api/graph` query contract: `GET /api/graph?symbol=<name>&parent=<optional>`. Missing `symbol` → HTTP 400.
- `/api/health` → `{"status":"ok","version":"<build>","root":"<root>"}` (unchanged).
- The server hosts **no** static content: there is no `/` route; unknown paths return 404.
- OUT OF SCOPE (do NOT touch): the `.lore/` data directory; `README.md`; `internal/config` indexing excludes and the `internal/webserver/dist` doc-comment example in `internal/config/config.go`; the `internal/config/filter_test.go` and `internal/merkle/walk_test.go` fixtures that use `internal/webserver/dist/...` as arbitrary path strings (they create their own temp files and are unaffected by deleting the real `dist/`). These belong to change 0004.
- Commit after each task with a conventional-commit message. Do NOT touch docket metadata (change files, BOARD.md, ADRs) — those live on the `docket` branch, not this feature branch.

---

### Task 1: Reduce the read model to a symbol-only shape (`model.go`)

Strip lore constructs from the core types so the symbol-only graph is the only thing `model.go` can express, and introduce the schema version constant. This is the foundation the later tasks build on.

**Files:**
- Modify: `internal/readmodel/model.go`

**Interfaces:**
- Consumes: nothing (leaf types).
- Produces:
  - `type NodeKind string`; `const NodeSymbol NodeKind = "symbol"` (the ONLY node kind).
  - `type EdgeKind string`; `const EdgeCalls EdgeKind = "calls"` (the ONLY edge kind).
  - `type Node struct { ID string; Kind NodeKind; Label string; File string; Line int; Signature string; Group string }` — JSON tags `id,kind,label,file,omitempty` etc.; NO `Status`/`Priority`.
  - `type Edge struct { Source, Target string; Kind EdgeKind; Conf string }`.
  - `type Graph struct { Focus string; Nodes []Node; Edges []Edge }`.
  - `const SchemaVersion = "1"`.
  - unchanged helpers `symID`, `qname`, `sortGraph`.

- [ ] **Step 1: Note there is no isolated unit test for `model.go`**

`model.go` is exercised transitively by `graph_test.go` (Task 4) and `server_test.go` (Task 6). This task is a type reduction; its verification is `go build` + the existing `TestSymbolNeighborhood`. No new test is written here — the failing-test cycle lands in Tasks 4/6 where behavior is asserted.

- [ ] **Step 2: Edit `model.go` to the symbol-only shape**

Replace the package doc comment and the type/const declarations. The full new file body (imports + declarations) is:

```go
// internal/readmodel/model.go
// Package readmodel converts the codeindex call graph into a single
// JSON-serializable node/edge graph consumed by the headless graph API and CLI.
package readmodel

import "sort"

// SchemaVersion is the top-level version pinned on every graph API response so
// external consumers are insulated from internal shape changes.
const SchemaVersion = "1"

type NodeKind string

const NodeSymbol NodeKind = "symbol"

type EdgeKind string

const EdgeCalls EdgeKind = "calls"

type Node struct {
	ID        string   `json:"id"`
	Kind      NodeKind `json:"kind"`
	Label     string   `json:"label"`
	File      string   `json:"file,omitempty"`
	Line      int      `json:"line,omitempty"`
	Signature string   `json:"signature,omitempty"`
	Group     string   `json:"group,omitempty"` // cluster key (package dir)
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

- [ ] **Step 3: Verify the package does NOT yet build (expected)**

Run: `cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer && go build ./internal/readmodel/`
Expected: FAIL — `graph.go` and `fullgraph.go` still reference the now-removed `NodeDecision`, `NodeItem`, `NodeNote`, `NodePath`, `EdgeAnchors`, `EdgeBlockedBy`, `Status`, `Priority`, and the lore imports. This is expected; Tasks 2–3 remove those references. Do NOT commit a broken build.

- [ ] **Step 4: Proceed to Task 2 before committing**

Because `model.go`, `graph.go`, and `fullgraph.go` form one compile unit, commit Tasks 1–3 together at the end of Task 3 (a single "green" commit). Leave the tree uncommitted here.

---

### Task 2: Strip the lore branch and helpers from `graph.go`

Remove `loreNode`, `RecordNeighborhood`, `Neighborhood`, `openLore`, and `AttachAnchoredLore` — every lore-coupled function — leaving `SymbolNeighborhood` and `openGraph`. This drops the `internal/lore` and `internal/lore/index` imports.

**Files:**
- Modify: `internal/readmodel/graph.go`

**Interfaces:**
- Consumes: `model.go` types from Task 1; `codeindex/internal/graph`, `codeindex/internal/query`.
- Produces:
  - `func SymbolNeighborhood(st *graph.Store, name, parent string) (Graph, error)` — UNCHANGED body.
  - `func openGraph(root string) (*graph.Store, error)` — UNCHANGED body.
  - NO other exported functions. No `internal/lore` imports remain.

- [ ] **Step 1: Rewrite `graph.go` to the symbol-only surface**

Replace the entire file with exactly this (keeps `SymbolNeighborhood` + `openGraph`, drops everything lore):

```go
// internal/readmodel/graph.go
package readmodel

import (
	"path/filepath"

	"codeindex/internal/graph"
	"codeindex/internal/query"
)

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

func openGraph(root string) (*graph.Store, error) {
	if _, err := query.Fresh(root); err != nil {
		return nil, err
	}
	return graph.Open(filepath.Join(root, ".codeindex", "graph.db"))
}
```

- [ ] **Step 2: Do NOT build yet**

`fullgraph.go` still calls `openLore` and `loreNode` (removed above), so the package will not compile until Task 3. Proceed directly to Task 3.

---

### Task 3: Make `FullGraph` symbol-only (`fullgraph.go`), then commit the green read model

Remove the lore overlay from `FullGraph` — no `openLore`, no lore nodes, no anchors/blocked_by edges — so it returns just tier-0 symbols and resolved call edges, grouped by package dir. Then build + test the whole module and commit Tasks 1–3 as one green commit.

**Files:**
- Modify: `internal/readmodel/fullgraph.go`

**Interfaces:**
- Consumes: `model.go` types (Task 1); `codeindex/internal/graph` via `openGraph`.
- Produces: `func FullGraph(root string) (Graph, error)` — symbol-only. Same node-pruning behavior (only symbols that participate in a resolved call edge are emitted).

- [ ] **Step 1: Rewrite `fullgraph.go` symbol-only**

Replace the entire file with exactly this:

```go
package readmodel

import (
	"fmt"
	"path"
)

// pkgOf derives a cluster key from a file path: its directory, e.g.
// "internal/graph/store.go" -> "internal/graph". Root files group as "(root)".
func pkgOf(file string) string {
	d := path.Dir(file)
	if d == "." || d == "" {
		return "(root)"
	}
	return d
}

func symNodeID(id int64) string { return fmt.Sprintf("sym#%d", id) }

// FullGraph returns the entire project symbol graph: all tier-0 symbols that
// participate in a resolved call edge, plus those call edges. Symbol nodes carry
// a Group (package dir) for clustering. Isolated leaf symbols (no resolved call)
// are omitted so the call structure is not buried.
func FullGraph(root string) (Graph, error) {
	st, err := openGraph(root)
	if err != nil {
		return Graph{}, err
	}
	defer st.Close()

	syms, err := st.GraphNodes()
	if err != nil {
		return Graph{}, err
	}
	callEdges, err := st.GraphCallEdges()
	if err != nil {
		return Graph{}, err
	}

	present := make(map[int64]bool, len(syms))
	symByID := make(map[int64]graphSym, len(syms))
	for _, sy := range syms {
		present[sy.ID] = true
		qn := sy.Name
		if sy.Parent != "" {
			qn = sy.Parent + "." + sy.Name
		}
		symByID[sy.ID] = graphSym{qn: qn, file: sy.File, line: sy.StartLine, sig: sy.Signature}
	}

	g := Graph{}
	used := map[int64]bool{}

	for _, e := range callEdges {
		if present[e.Src] && present[e.Dst] && e.Src != e.Dst {
			g.Edges = append(g.Edges, Edge{Source: symNodeID(e.Src), Target: symNodeID(e.Dst), Kind: EdgeCalls})
			used[e.Src] = true
			used[e.Dst] = true
		}
	}

	for id := range used {
		sy := symByID[id]
		g.Nodes = append(g.Nodes, Node{
			ID:        symNodeID(id),
			Kind:      NodeSymbol,
			Label:     sy.qn,
			File:      sy.file,
			Line:      sy.line,
			Signature: sy.sig,
			Group:     pkgOf(sy.file),
		})
	}

	sortGraph(&g)
	return g, nil
}

type graphSym struct {
	qn, file, sig string
	line          int
}
```

- [ ] **Step 2: Build the read model package**

Run: `cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer && go build ./internal/readmodel/`
Expected: PASS (no more lore references; imports `graph`, `query`, `fmt`, `path`, `path/filepath`, `sort` only).

- [ ] **Step 3: Fix the read model tests (`graph_test.go`)**

The existing `graph_test.go` imports lore and tests removed functions. Replace the whole file with only the surviving symbol test (drop `TestAttachAnchoredLore`, `TestNeighborhood*`, `TestRecordNeighborhood`, `openLoreStore`, `writeRepo`, and the lore imports):

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
	ids := map[string]bool{}
	for _, n := range g.Nodes {
		ids[n.ID] = true
	}
	for _, want := range []string{"sym:A", "sym:Helper", "sym:B"} {
		if !ids[want] {
			t.Errorf("missing node %q; got %v", want, ids)
		}
	}
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

- [ ] **Step 4: Test the read model package**

Run: `cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer && go test ./internal/readmodel/`
Expected: PASS.

- [ ] **Step 5: Confirm no lore imports remain in readmodel**

Run: `cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer && grep -rn "internal/lore" internal/readmodel/ || echo "clean"`
Expected: `clean`.

- [ ] **Step 6: Commit the symbol-only read model**

Note: the webserver still references `readmodel.Neighborhood`/`FullGraph` with lore-join expectations in its tests, but `go build ./...` for the webserver package will still compile `FullGraph` (still present) though `Neighborhood` is gone — so a full-module build is NOT green yet. Scope this commit to the readmodel package only.

```bash
cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer
git add internal/readmodel/model.go internal/readmodel/graph.go internal/readmodel/fullgraph.go internal/readmodel/graph_test.go
git commit -m "refactor(readmodel): reduce to symbol-only graph; drop lore overlay"
```

---

### Task 4: Headless symbol-only HTTP API + schemaVersion (`server.go`, delete `static.go`/`dist`)

Re-point `/api/graph` to `?symbol=&parent=` + `SymbolNeighborhood`, wrap responses in a `schemaVersion` envelope, drop the static `/` route, and delete the embedded SPA (`static.go` + `dist/`). This makes the whole module build again.

**Files:**
- Modify: `internal/webserver/server.go`
- Delete: `internal/webserver/static.go`
- Delete: `internal/webserver/dist/` (entire directory)

**Interfaces:**
- Consumes: `readmodel.SymbolNeighborhood`, `readmodel.FullGraph`, `readmodel.SchemaVersion`, `readmodel.Graph`.
- Produces:
  - `func New(root, version string) http.Handler` — routes `/api/health`, `/api/graph`, `/api/graph/full`; NO `/` route.
  - `func Run(root, addr, version string) error` — unchanged signature.
  - graph responses are `{"schemaVersion":"1","focus":...,"nodes":[...],"edges":[...]}`.

- [ ] **Step 1: Delete the static handler and embedded assets**

```bash
cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer
git rm internal/webserver/static.go
git rm -r internal/webserver/dist
```

- [ ] **Step 2: Rewrite `server.go` headless + symbol-only + schemaVersion**

Replace the entire file with exactly this:

```go
// internal/webserver/server.go
// Package webserver serves the codeindex symbol read model over HTTP as a
// headless, versioned JSON API. Read-only; bind to loopback only. No static
// content is hosted.
package webserver

import (
	"encoding/json"
	"log"
	"net/http"

	"codeindex/internal/readmodel"
)

// graphResponse wraps a symbol graph with the top-level schemaVersion pinned on
// every graph API response.
type graphResponse struct {
	SchemaVersion string `json:"schemaVersion"`
	readmodel.Graph
}

func newGraphResponse(g readmodel.Graph) graphResponse {
	return graphResponse{SchemaVersion: readmodel.SchemaVersion, Graph: g}
}

// New returns the HTTP handler for the read-only headless graph API.
func New(root, version string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "ok", "version": version, "root": root,
		})
	})

	mux.HandleFunc("/api/graph", func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		if symbol == "" {
			http.Error(w, "missing required query param: symbol", http.StatusBadRequest)
			return
		}
		parent := r.URL.Query().Get("parent")
		st, err := openGraph(root)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer st.Close()
		g, err := readmodel.SymbolNeighborhood(st, symbol, parent)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, newGraphResponse(g))
	})

	mux.HandleFunc("/api/graph/full", func(w http.ResponseWriter, _ *http.Request) {
		g, err := readmodel.FullGraph(root)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, newGraphResponse(g))
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

Note: `/api/graph` needs a `*graph.Store` to call `SymbolNeighborhood`, obtained via `readmodel.openGraph` — but that is unexported. Do NOT reach into readmodel; instead add a thin exported opener in the webserver package that mirrors it. See Step 3.

- [ ] **Step 3: Add an exported graph opener usable by the handler**

`readmodel.openGraph` is unexported. Rather than widen readmodel's surface, add a package-local opener in the webserver. Create `internal/webserver/graphstore.go`:

```go
// internal/webserver/graphstore.go
package webserver

import (
	"path/filepath"

	"codeindex/internal/graph"
	"codeindex/internal/query"
)

// openGraph freshens the index for root and opens its symbol graph store. The
// caller owns Close.
func openGraph(root string) (*graph.Store, error) {
	if _, err := query.Fresh(root); err != nil {
		return nil, err
	}
	return graph.Open(filepath.Join(root, ".codeindex", "graph.db"))
}
```

- [ ] **Step 4: Verify the whole module builds**

Run: `cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer && go build ./...`
Expected: PASS. (`server_test.go` may still fail to COMPILE under `go vet`/test build because it imports lore — that is fixed in Task 6; a plain `go build ./...` excludes `_test.go` files, so it passes here.)

- [ ] **Step 5: Do NOT commit yet — tests are red**

`server_test.go` still imports `internal/lore` and asserts the old shape, so `go test ./internal/webserver/` will not compile. Fix it in Task 6, then commit Tasks 4–6 together. Leave uncommitted.

---

### Task 5: Delete the orphaned `internal/lore/**` and `internal/tui` trees (carryovers)

Now that no non-lore code imports lore, delete the whole `internal/lore` tree and the orphaned `internal/tui/tree` package (its sole consumer, the `tree` CLI command, was deleted in change 0002; `internal/tui` has no other subpackages).

**Files:**
- Delete: `internal/lore/` (entire directory — `layout.go`, `record.go`, `gitinfo/`, `index/`, and their tests)
- Delete: `internal/tui/` (entire directory — only contains `tree/`)

**Interfaces:**
- Consumes: nothing.
- Produces: nothing. After this task, `grep -rn "internal/lore" --include='*.go'` and `grep -rn "internal/tui" --include='*.go'` return no hits outside deleted files.

- [ ] **Step 1: Confirm nothing outside these trees imports them**

Run:
```bash
cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer
grep -rn "codeindex/internal/lore" --include='*.go' . | grep -v "^./internal/lore/"
grep -rn "codeindex/internal/tui" --include='*.go' . | grep -v "^./internal/tui/"
```
Expected: NO output from either (the only remaining lore importer, `server_test.go`, is fixed in Task 6 — but it is in `internal/webserver`, not these trees, and is handled next; if it still appears here that is fine, it is deleted-by-rewrite in Task 6). If any OTHER file appears, stop and investigate before deleting.

- [ ] **Step 2: Delete the trees**

```bash
cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer
git rm -r internal/lore
git rm -r internal/tui
```

- [ ] **Step 3: Build (test build still red via server_test)**

Run: `cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer && go build ./...`
Expected: PASS. Proceed to Task 6 to make the test build green before committing.

---

### Task 6: Rewrite `server_test.go` to the symbol-only contract; green + commit

Replace the lore-join server tests with symbol-only assertions: health, `/api/graph?symbol=` shape + `schemaVersion`, `/api/graph/full` symbol-only + `schemaVersion`, missing-`symbol` 400, and root-path 404 (static hosting gone). Then run the full suite and commit Tasks 4–6.

**Files:**
- Modify: `internal/webserver/server_test.go`

**Interfaces:**
- Consumes: `webserver.New`; the `/api/*` HTTP contract from Task 4.
- Produces: the test suite that gates the API contract.

- [ ] **Step 1: Write the new `server_test.go` (failing until Task 4/5 land — they already have in this sequence)**

Replace the entire file with exactly this (no lore import; a code-only fixture repo):

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
)

// writeRepo creates a temp repo with code files only (no lore).
func writeRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("CODEINDEX_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"),
		[]byte("package p\nfunc Helper(x int) int { return x + 1 }\nfunc A() int { return Helper(1) }\n"), 0o644); err != nil {
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

func TestGraphEndpointSymbolOnly(t *testing.T) {
	srv := httptest.NewServer(New(writeRepo(t), "test"))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/graph?symbol=A")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var g struct {
		SchemaVersion string `json:"schemaVersion"`
		Focus         string `json:"focus"`
		Nodes         []struct {
			ID, Kind, Label string
		} `json:"nodes"`
		Edges []struct {
			Source, Target, Kind string
		} `json:"edges"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
		t.Fatal(err)
	}
	if g.SchemaVersion != "1" {
		t.Fatalf("schemaVersion = %q, want 1", g.SchemaVersion)
	}
	if g.Focus != "sym:A" {
		t.Fatalf("focus = %q, want sym:A", g.Focus)
	}
	// Every node is a symbol; the focus and its callee Helper are present.
	ids := map[string]bool{}
	for _, n := range g.Nodes {
		if n.Kind != "symbol" {
			t.Fatalf("non-symbol node kind %q in symbol-only API: %+v", n.Kind, n)
		}
		ids[n.ID] = true
	}
	if !ids["sym:A"] || !ids["sym:Helper"] {
		t.Fatalf("expected sym:A and sym:Helper nodes; got %v", ids)
	}
	// The focus calls Helper.
	var hasCallEdge bool
	for _, e := range g.Edges {
		if e.Source == "sym:A" && e.Target == "sym:Helper" && e.Kind == "calls" {
			hasCallEdge = true
		}
		if e.Kind != "calls" {
			t.Fatalf("non-call edge kind %q in symbol-only API: %+v", e.Kind, e)
		}
	}
	if !hasCallEdge {
		t.Fatalf("expected sym:A -> sym:Helper calls edge; edges=%+v", g.Edges)
	}
}

func TestFullGraphEndpointSymbolOnly(t *testing.T) {
	srv := httptest.NewServer(New(writeRepo(t), "test"))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/graph/full")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var g struct {
		SchemaVersion string `json:"schemaVersion"`
		Nodes         []struct {
			ID, Kind, Label, Group string
		} `json:"nodes"`
		Edges []struct {
			Source, Target, Kind string
		} `json:"edges"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
		t.Fatal(err)
	}
	if g.SchemaVersion != "1" {
		t.Fatalf("schemaVersion = %q, want 1", g.SchemaVersion)
	}
	var hasHelper bool
	for _, n := range g.Nodes {
		if n.Kind != "symbol" {
			t.Fatalf("non-symbol node kind %q in symbol-only full graph: %+v", n.Kind, n)
		}
		if n.Label == "Helper" {
			hasHelper = true
		}
	}
	if !hasHelper {
		t.Errorf("expected a Helper symbol node; nodes=%+v", g.Nodes)
	}
	for _, e := range g.Edges {
		if e.Kind != "calls" {
			t.Fatalf("non-call edge kind %q in symbol-only full graph: %+v", e.Kind, e)
		}
	}
}

func TestGraphEndpointMissingSymbol(t *testing.T) {
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

func TestRootPath404_NoStaticHosting(t *testing.T) {
	srv := httptest.NewServer(New(writeRepo(t), "test"))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("root path status = %d, want 404 (static hosting must be gone)", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run the webserver tests**

Run: `cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer && go test ./internal/webserver/ -v`
Expected: PASS for all of `TestHealthEndpoint`, `TestGraphEndpointSymbolOnly`, `TestFullGraphEndpointSymbolOnly`, `TestGraphEndpointMissingSymbol`, `TestRootPath404_NoStaticHosting`.

- [ ] **Step 3: Full module build + test**

Run: `cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer && go build ./... && go test ./...`
Expected: both PASS. No dangling lore/tui references anywhere.

- [ ] **Step 4: Commit the headless API + carryover deletions**

```bash
cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer
git add -A
git commit -m "feat(serve): headless symbol-only graph API; delete web/lore/tui overlay"
```

---

### Task 7: Delete the `web/` app

Remove the entire React lore-graph UI (including the galaxy retheme). It has no Go consumers — the static handler that embedded it is already gone.

**Files:**
- Delete: `web/` (entire directory)

**Interfaces:**
- Consumes: nothing. Produces: nothing.

- [ ] **Step 1: Confirm no Go or build reference to `web/` remains**

Run:
```bash
cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer
grep -rn "\"web/\|/web\b" --include='*.go' . || echo "no go refs"
```
Expected: `no go refs` (the webserver embedded `dist/`, already deleted; nothing embeds `web/`).

- [ ] **Step 2: Delete `web/`**

```bash
cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer
git rm -r web
```

- [ ] **Step 3: Build + test still green**

Run: `cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer && go build ./... && go test ./...`
Expected: both PASS (deleting a non-Go tree cannot affect the Go build; this confirms it).

- [ ] **Step 4: Commit**

```bash
cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer
git commit -m "chore(web): delete the React lore-graph UI"
```

---

### Task 8: Document the graph API contract (`docs/graph-api.md`)

Write the versioned API contract so a future viewer or other tooling can build against it without reading Go.

**Files:**
- Create: `docs/graph-api.md`

**Interfaces:**
- Consumes: the shipped `/api/*` contract from Tasks 4/6.
- Produces: `docs/graph-api.md`.

- [ ] **Step 1: Write `docs/graph-api.md`**

Create the file with exactly this content:

```markdown
# codeindex graph API

`codeindex serve` exposes the project's **symbol** call graph as a headless,
read-only JSON API over loopback HTTP. It hosts no static content. Every graph
response carries a top-level `schemaVersion` so external consumers are insulated
from internal shape changes.

**Current `schemaVersion`: `"1"`**

## Endpoints

### `GET /api/health`

Liveness + build identity.

```json
{ "status": "ok", "version": "<build>", "root": "<indexed repo root>" }
```

### `GET /api/graph?symbol=<name>&parent=<optional>`

The neighborhood of a focus symbol: the focus plus its direct callers and
callees. `symbol` is required (omitting it returns HTTP 400); `parent`
optionally disambiguates a method by its enclosing type.

```json
{
  "schemaVersion": "1",
  "focus": "sym:A",
  "nodes": [ /* Node */ ],
  "edges": [ /* Edge */ ]
}
```

### `GET /api/graph/full`

The whole symbol graph: every tier-0 symbol that participates in a resolved call
edge, plus those edges. Isolated leaf symbols (no resolved call) are omitted so
the call structure is not buried. Nodes carry a `group` (package directory) for
clustering. The response is the whole graph in one payload; there is currently no
pagination.

```json
{
  "schemaVersion": "1",
  "focus": "",
  "nodes": [ /* Node */ ],
  "edges": [ /* Edge */ ]
}
```

## Types

### Node (symbol-only)

```json
{
  "id": "sym:Pkg.Name",
  "kind": "symbol",
  "label": "Pkg.Name",
  "file": "internal/pkg/file.go",
  "line": 42,
  "signature": "func Name(x int) int",
  "group": "internal/pkg"
}
```

- `kind` is always `"symbol"`.
- `file`, `line`, `signature`, `group` are omitted when empty.
- In `/api/graph`, node ids are `sym:<qualified-name>`; in `/api/graph/full`,
  node ids are `sym#<internal-id>` (stable within a single response).

### Edge

```json
{ "source": "sym:A", "target": "sym:Helper", "kind": "calls", "conf": "high" }
```

- `kind` is always `"calls"`.
- `conf` (confidence) is present on neighborhood edges when known; omitted otherwise.

## Versioning

`schemaVersion` is a string. A backward-incompatible change to the node/edge
shape bumps it. Additive, optional fields do not.
```

- [ ] **Step 2: Commit the docs**

```bash
cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer
git add docs/graph-api.md
git commit -m "docs(graph-api): document the versioned symbol-graph API contract"
```

---

### Task 9: Final full-repo gate

Confirm the whole repo is green and free of any lore/web/tui references, and that the `serve` command still wires up.

**Files:** none (verification only).

- [ ] **Step 1: Full build + test**

Run: `cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer && go build ./... && go test ./...`
Expected: both PASS.

- [ ] **Step 2: Confirm no lore/web/tui/dist references linger in Go**

Run:
```bash
cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer
grep -rn "internal/lore\|internal/tui\|webserver/dist\|staticHandler\|RecordNeighborhood\|loreNode\|AttachAnchoredLore\|readmodel.Neighborhood" --include='*.go' . || echo "clean"
```
Expected: `clean`.

- [ ] **Step 3: Confirm `serve` still compiles end-to-end**

Run: `cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer && go vet ./cmd/... ./internal/webserver/ ./internal/readmodel/`
Expected: PASS (no vet errors).

- [ ] **Step 4: Confirm deleted trees are gone**

Run:
```bash
cd /Users/ethanhinson/codeindex/.worktrees/decouple-graph-query-layer
test ! -d web && test ! -d internal/lore && test ! -d internal/tui && test ! -d internal/webserver/dist && echo "all deleted"
```
Expected: `all deleted`.

---

## Self-Review

**1. Spec coverage** (against the change body + spec "Phase 2" and "The graph API contract"):
- Delete `web/` app incl. galaxy retheme → Task 7. ✓
- Delete webserver static handler + `dist` → Task 4. ✓
- Strip readmodel lore overlay (`FullGraph` branch, `RecordNeighborhood`, `loreNode`) + dead helpers (`Neighborhood`, `openLore`, `AttachAnchoredLore`) + `model.go` reduction → Tasks 1–3. ✓
- Keep `SymbolNeighborhood` + symbol-only `FullGraph` → Tasks 2–3. ✓
- CARRYOVER: delete `internal/lore/**` → Task 5. ✓
- CARRYOVER: delete orphaned `internal/tui/tree` → Task 5. ✓
- `serve` headless JSON API: `/api/health`, `/api/graph?symbol=&parent=`, `/api/graph/full` → Task 4. ✓
- Top-level `schemaVersion`; Node symbol-only `{ID, Kind:"symbol", Label, File, Line, Signature, Group}` → Tasks 1, 4. ✓
- Document contract in `docs/graph-api.md` → Task 8. ✓
- Update `server_test.go`: symbol-only shape, `schemaVersion`, root 404 → Task 6. ✓
- Green `go build ./...` / `go test ./...` incrementally → gates in every task + Task 9. ✓
- OUT OF SCOPE respected: `.lore/`, README, config excludes — never touched. ✓

**2. Placeholder scan:** No TBD/TODO/"add appropriate X"/"similar to Task N" — every code and test block is complete and literal. ✓

**3. Type consistency:**
- `NodeSymbol`/`EdgeCalls`/`SchemaVersion` defined in Task 1, used in Tasks 2/3/4/6. ✓
- `SymbolNeighborhood(st *graph.Store, name, parent string)` defined Task 2, called Task 4. ✓
- `openGraph(root)` in readmodel (Task 2, unexported) vs the webserver-local `openGraph` (Task 4 Step 3) — deliberately separate, same signature; the handler uses the webserver-local one. Documented in the task to avoid reaching into readmodel. ✓
- `graphResponse` embeds `readmodel.Graph` so `schemaVersion` is a sibling of `focus`/`nodes`/`edges` in JSON — matches the documented shape in Task 8. ✓
- `symNodeID` (full graph, `sym#<id>`) vs `symID` (neighborhood, `sym:<qname>`) — both preserved from the original code; the doc (Task 8) notes the id-prefix difference. ✓

Plan is complete and internally consistent.

## Execution Handoff

Per the caller's direction, this plan is written and execution is deferred to `superpowers:subagent-driven-development` (Subagent-Driven, the recommended option), run separately by the docket implementer. No interactive choice is surfaced.
