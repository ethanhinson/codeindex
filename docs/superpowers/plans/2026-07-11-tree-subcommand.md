# `tree` Subcommand Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `codeindex tree <repo>`: an interactive Bubble Tea TUI that explores the index as directories → files → symbols, with a live filter and a detail side panel; static indented output when stdout is not a TTY.

**Architecture:** A new `internal/tui/tree` package holds pure tree logic (build, flatten, filter, static render) plus the Bubble Tea model/view. `internal/graph` gains one read query, `ProjectSymbols()`. A new `case "tree"` in `cmd/codeindex` wires it up after the standard `query.Fresh` freshness check.

**Tech Stack:** Go 1.26, `github.com/charmbracelet/bubbletea` (v1), `github.com/charmbracelet/lipgloss`, existing SQLite store (`internal/graph`).

**Spec:** `docs/superpowers/specs/2026-07-11-tree-subcommand-design.md`

## Global Constraints

- New dependencies limited to `github.com/charmbracelet/bubbletea` and `github.com/charmbracelet/lipgloss` (plus their transitive deps).
- No schema change: `ProjectSymbols()` reads the existing `symbols` view (`tier=0` only). `schemaVersion` stays `7`.
- The query layer (`internal/query`) stays TUI-free; all TUI code lives in `internal/tui/tree`.
- CLI arg handling follows the existing `main.go` style: positional `os.Args`, `switch` dispatch, `fatal(err)` on error.
- v1 scope cuts (from spec): no editor-jump key, no call/dependency pivots, no mouse.
- All file paths in the index are repo-relative with `/` separators; tree code splits on `/`, never `filepath`.
- Run `go test ./...` from the repo root; CGO is required (works out of the box on macOS with Xcode CLT).

---

### Task 1: `Store.ProjectSymbols()` query

**Files:**
- Modify: `internal/graph/store.go` (add method near `Definitions`, ~line 580)
- Test: `internal/graph/store_test.go` (new file — the package has no test file yet)

**Interfaces:**
- Consumes: existing `symbols` SQL view, `graph.Symbol` struct.
- Produces: `func (s *Store) ProjectSymbols() ([]Symbol, error)` — every tier-0 symbol with `File`, `Name`, `Parent`, `Kind`, `Signature`, `StartLine`, `EndLine` populated, ordered by `file, start_line, id`. Later tasks feed this straight into `BuildTree`.

- [ ] **Step 1: Write the failing test**

Create `internal/graph/store_test.go`:

```go
package graph

import (
	"path/filepath"
	"testing"
)

// putFile inserts a parsed file into a fresh store inside one transaction.
func putFile(t *testing.T, st *Store, pf *ParsedFile) {
	t.Helper()
	tx, err := st.Begin()
	if err != nil {
		t.Fatal(err)
	}
	meta := FileMeta{Path: pf.Path, Hash: "h-" + pf.Path, Size: 1, Mtime: 1}
	if _, _, err := st.PutFile(tx, pf, meta); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectSymbolsOrderedByFileAndLine(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "g.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	putFile(t, st, &ParsedFile{Path: "b/second.go", Symbols: []Symbol{
		{File: "b/second.go", Name: "Beta", Kind: KindFunc,
			Signature: "func Beta()", StartLine: 10, EndLine: 12},
	}})
	putFile(t, st, &ParsedFile{Path: "a/first.go", Symbols: []Symbol{
		{File: "a/first.go", Name: "Store", Kind: KindType,
			Signature: "type Store struct", StartLine: 5, EndLine: 20},
		{File: "a/first.go", Name: "Close", Parent: "Store", Kind: KindMethod,
			Signature: "func (s *Store) Close() error", StartLine: 22, EndLine: 24},
	}})

	syms, err := st.ProjectSymbols()
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) != 3 {
		t.Fatalf("want 3 symbols, got %d: %+v", len(syms), syms)
	}
	// Ordered by file, then start line.
	want := []struct {
		file, name, parent string
		kind               SymbolKind
		line               int
	}{
		{"a/first.go", "Store", "", KindType, 5},
		{"a/first.go", "Close", "Store", KindMethod, 22},
		{"b/second.go", "Beta", "", KindFunc, 10},
	}
	for i, w := range want {
		g := syms[i]
		if g.File != w.file || g.Name != w.name || g.Parent != w.parent ||
			g.Kind != w.kind || g.StartLine != w.line {
			t.Errorf("syms[%d] = %+v, want %+v", i, g, w)
		}
	}
	if syms[0].Signature != "type Store struct" {
		t.Errorf("signature not populated: %q", syms[0].Signature)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/ -run TestProjectSymbols -v`
Expected: FAIL to compile with `st.ProjectSymbols undefined`.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/graph/store.go`, directly above `func (s *Store) Definitions`:

```go
// ProjectSymbols returns every project (tier-0) symbol ordered by file then
// start line — the load query for the tree explorer.
func (s *Store) ProjectSymbols() ([]Symbol, error) {
	rows, err := s.db.Query(
		`SELECT file, name, parent, kind, signature, start_line, end_line
		 FROM symbols WHERE tier=0 ORDER BY file, start_line, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Symbol
	for rows.Next() {
		var sy Symbol
		var kind string
		if err := rows.Scan(&sy.File, &sy.Name, &sy.Parent, &kind,
			&sy.Signature, &sy.StartLine, &sy.EndLine); err != nil {
			return nil, err
		}
		sy.Kind = SymbolKind(kind)
		out = append(out, sy)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/graph/ -run TestProjectSymbols -v`
Expected: PASS

- [ ] **Step 5: Run the full suite to check nothing broke**

Run: `go test ./...`
Expected: all packages PASS

- [ ] **Step 6: Commit**

```bash
git add internal/graph/store.go internal/graph/store_test.go
git commit -m "graph: add ProjectSymbols query for the tree explorer"
```

---

### Task 2: Tree construction (`BuildTree`)

**Files:**
- Create: `internal/tui/tree/node.go`
- Test: `internal/tui/tree/node_test.go`

**Interfaces:**
- Consumes: `[]graph.Symbol` from `Store.ProjectSymbols()` (Task 1).
- Produces (used by Tasks 3–8):

```go
type NodeKind int
const (
	KindDir NodeKind = iota
	KindFile
	KindSymbol
)

type Node struct {
	Label     string  // dir/file base name, or symbol display name
	Kind      NodeKind
	SymKind   string  // "func" | "method" | "type" (symbols only)
	File      string  // repo-relative path (files and symbols)
	Line      int     // start line (symbols only)
	Signature string
	SymName   string  // raw symbol name, for caller/callee lookups
	SymParent string  // raw parent, for caller/callee lookups
	Children  []*Node
	Expanded  bool
}

func BuildTree(syms []graph.Symbol) *Node // virtual root: KindDir, Label "", Expanded true
```

Nesting rules: directories → files → symbols; a symbol with `Parent` set nests under the type node of that name in the same file when one exists, otherwise it hangs off the file with label `Parent.Name`. Child order: dirs before files (each alphabetical); symbols by start line. Everything starts collapsed except the virtual root.

- [ ] **Step 1: Write the failing test**

Create `internal/tui/tree/node_test.go`:

```go
package tree

import (
	"testing"

	"codeindex/internal/graph"
)

func fixtureSymbols() []graph.Symbol {
	return []graph.Symbol{
		{File: "internal/graph/store.go", Name: "Store", Kind: graph.KindType,
			Signature: "type Store struct", StartLine: 5, EndLine: 90},
		{File: "internal/graph/store.go", Name: "Close", Parent: "Store",
			Kind: graph.KindMethod, Signature: "func (s *Store) Close() error",
			StartLine: 40, EndLine: 42},
		{File: "internal/graph/store.go", Name: "open", Kind: graph.KindFunc,
			Signature: "func open()", StartLine: 100, EndLine: 110},
		{File: "internal/query/query.go", Name: "Fresh", Kind: graph.KindFunc,
			Signature: "func Fresh()", StartLine: 10, EndLine: 20},
		// Method whose parent type lives in another file: falls back to the file.
		{File: "internal/query/query.go", Name: "Orphan", Parent: "Ghost",
			Kind: graph.KindMethod, Signature: "func (g Ghost) Orphan()",
			StartLine: 30, EndLine: 31},
		{File: "main.go", Name: "main", Kind: graph.KindFunc,
			Signature: "func main()", StartLine: 1, EndLine: 3},
	}
}

// child finds a direct child by label or fails the test.
func child(t *testing.T, n *Node, label string) *Node {
	t.Helper()
	for _, c := range n.Children {
		if c.Label == label {
			return c
		}
	}
	t.Fatalf("node %q has no child %q (children: %v)", n.Label, label, labels(n))
	return nil
}

func labels(n *Node) []string {
	var out []string
	for _, c := range n.Children {
		out = append(out, c.Label)
	}
	return out
}

func TestBuildTreeStructure(t *testing.T) {
	root := BuildTree(fixtureSymbols())

	// Dirs sort before files: internal/ before main.go.
	if got := labels(root); len(got) != 2 || got[0] != "internal" || got[1] != "main.go" {
		t.Fatalf("root children = %v, want [internal main.go]", got)
	}

	internal := child(t, root, "internal")
	if internal.Kind != KindDir || internal.Expanded {
		t.Fatalf("internal: kind=%v expanded=%v, want dir, collapsed", internal.Kind, internal.Expanded)
	}

	storeGo := child(t, child(t, internal, "graph"), "store.go")
	if storeGo.Kind != KindFile || storeGo.File != "internal/graph/store.go" {
		t.Fatalf("store.go node = %+v", storeGo)
	}

	// Symbols ordered by line: Store (5) before open (100).
	if got := labels(storeGo); len(got) != 2 || got[0] != "Store" || got[1] != "open" {
		t.Fatalf("store.go children = %v, want [Store open]", got)
	}

	// Method nests under its type node.
	storeType := child(t, storeGo, "Store")
	closeM := child(t, storeType, "Close")
	if closeM.SymKind != "method" || closeM.SymParent != "Store" || closeM.Line != 40 {
		t.Fatalf("Close node = %+v", closeM)
	}

	// Orphan method (no type node in file) hangs off the file, qualified.
	queryGo := child(t, child(t, internal, "query"), "query.go")
	orphan := child(t, queryGo, "Ghost.Orphan")
	if orphan.SymName != "Orphan" || orphan.SymParent != "Ghost" {
		t.Fatalf("orphan node = %+v", orphan)
	}
}

func TestBuildTreeEmpty(t *testing.T) {
	root := BuildTree(nil)
	if root == nil || len(root.Children) != 0 || !root.Expanded {
		t.Fatalf("empty root = %+v", root)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/tree/ -v`
Expected: FAIL to compile (package does not exist yet / `BuildTree` undefined).

- [ ] **Step 3: Write the implementation**

Create `internal/tui/tree/node.go`:

```go
// Package tree renders the index as an explorable directory → file → symbol
// tree: pure tree logic here, the Bubble Tea UI in model.go/view.go.
package tree

import (
	"sort"
	"strings"

	"codeindex/internal/graph"
)

type NodeKind int

const (
	KindDir NodeKind = iota
	KindFile
	KindSymbol
)

type Node struct {
	Label     string
	Kind      NodeKind
	SymKind   string
	File      string
	Line      int
	Signature string
	SymName   string
	SymParent string
	Children  []*Node
	Expanded  bool
}

// BuildTree arranges symbols into directories → files → symbols, nesting a
// method under its parent type when that type is defined in the same file.
func BuildTree(syms []graph.Symbol) *Node {
	root := &Node{Kind: KindDir, Expanded: true}
	dirs := map[string]*Node{"": root}
	files := map[string]*Node{}

	var dirFor func(path string) *Node
	dirFor = func(path string) *Node {
		if n, ok := dirs[path]; ok {
			return n
		}
		parent, base := splitPath(path)
		n := &Node{Label: base, Kind: KindDir}
		dirs[path] = n
		p := dirFor(parent)
		p.Children = append(p.Children, n)
		return n
	}
	fileFor := func(path string) *Node {
		if n, ok := files[path]; ok {
			return n
		}
		parent, base := splitPath(path)
		n := &Node{Label: base, Kind: KindFile, File: path}
		files[path] = n
		d := dirFor(parent)
		d.Children = append(d.Children, n)
		return n
	}

	// First pass: top-level symbols; remember them for member nesting.
	byFileName := map[string]*Node{}
	for i := range syms {
		s := &syms[i]
		if s.Parent != "" {
			continue
		}
		n := symbolNode(s)
		fileFor(s.File).Children = append(fileFor(s.File).Children, n)
		byFileName[s.File+"\x00"+s.Name] = n
	}
	// Second pass: members nest under their type when it exists in the file.
	for i := range syms {
		s := &syms[i]
		if s.Parent == "" {
			continue
		}
		n := symbolNode(s)
		if t, ok := byFileName[s.File+"\x00"+s.Parent]; ok {
			t.Children = append(t.Children, n)
		} else {
			n.Label = s.Parent + "." + s.Name
			fileFor(s.File).Children = append(fileFor(s.File).Children, n)
		}
	}

	sortTree(root)
	return root
}

func symbolNode(s *graph.Symbol) *Node {
	return &Node{
		Label: s.Name, Kind: KindSymbol, SymKind: string(s.Kind),
		File: s.File, Line: s.StartLine, Signature: s.Signature,
		SymName: s.Name, SymParent: s.Parent,
	}
}

// splitPath splits a repo-relative slash path into parent dir and base name.
func splitPath(p string) (dir, base string) {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i], p[i+1:]
	}
	return "", p
}

// sortTree orders children: dirs, then files (each alphabetical), then
// symbols by line.
func sortTree(n *Node) {
	sort.SliceStable(n.Children, func(i, j int) bool {
		a, b := n.Children[i], n.Children[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Kind == KindSymbol {
			return a.Line < b.Line
		}
		return a.Label < b.Label
	})
	for _, c := range n.Children {
		sortTree(c)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/tree/ -v`
Expected: PASS (both tests)

- [ ] **Step 5: Commit**

```bash
git add internal/tui/tree/node.go internal/tui/tree/node_test.go
git commit -m "tree: build dir/file/symbol tree from index symbols"
```

---

### Task 3: Flatten for display (`Visible`, `ParentIndex`)

**Files:**
- Create: `internal/tui/tree/flatten.go`
- Test: `internal/tui/tree/flatten_test.go`

**Interfaces:**
- Consumes: `*Node` from Task 2.
- Produces (used by the model and view):

```go
type Row struct {
	Node  *Node
	Depth int
}
func Visible(root *Node) []Row          // preorder rows; descends only into Expanded nodes; virtual root excluded
func ParentIndex(rows []Row, i int) int // row index of rows[i]'s parent, -1 for top level
```

- [ ] **Step 1: Write the failing test**

Create `internal/tui/tree/flatten_test.go`:

```go
package tree

import "testing"

func rowLabels(rows []Row) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Node.Label)
	}
	return out
}

func TestVisibleRespectsExpansion(t *testing.T) {
	root := BuildTree(fixtureSymbols())

	// All collapsed: only top level shows.
	got := rowLabels(Visible(root))
	want := []string{"internal", "main.go"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("collapsed rows = %v, want %v", got, want)
	}

	// Expand internal/ → its children appear, at depth 1.
	child(t, root, "internal").Expanded = true
	rows := Visible(root)
	got = rowLabels(rows)
	want = []string{"internal", "graph", "query", "main.go"}
	if len(got) != 4 || got[1] != "graph" || got[3] != "main.go" {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	if rows[0].Depth != 0 || rows[1].Depth != 1 {
		t.Fatalf("depths = %d,%d want 0,1", rows[0].Depth, rows[1].Depth)
	}
}

func TestVisibleDeepExpansion(t *testing.T) {
	root := BuildTree(fixtureSymbols())
	internal := child(t, root, "internal")
	internal.Expanded = true
	graphDir := child(t, internal, "graph")
	graphDir.Expanded = true
	storeGo := child(t, graphDir, "store.go")
	storeGo.Expanded = true

	got := rowLabels(Visible(root))
	// internal, graph, store.go, Store, open, query, main.go
	if len(got) != 7 || got[3] != "Store" || got[4] != "open" {
		t.Fatalf("rows = %v", got)
	}
}

func TestParentIndex(t *testing.T) {
	root := BuildTree(fixtureSymbols())
	internal := child(t, root, "internal")
	internal.Expanded = true
	child(t, internal, "graph").Expanded = true
	rows := Visible(root)
	// rows: internal(0) graph(1) store.go(2) query(3) main.go(4)
	if p := ParentIndex(rows, 2); p != 1 {
		t.Errorf("parent of store.go = %d, want 1", p)
	}
	if p := ParentIndex(rows, 3); p != 0 {
		t.Errorf("parent of query = %d, want 0", p)
	}
	if p := ParentIndex(rows, 0); p != -1 {
		t.Errorf("parent of internal = %d, want -1", p)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/tree/ -run 'TestVisible|TestParentIndex' -v`
Expected: FAIL to compile (`Row`, `Visible`, `ParentIndex` undefined).

- [ ] **Step 3: Write the implementation**

Create `internal/tui/tree/flatten.go`:

```go
package tree

// Row is one drawable line of the tree: a node at a depth.
type Row struct {
	Node  *Node
	Depth int
}

// Visible returns the rows a renderer should draw: a preorder walk that
// descends only into expanded nodes. The virtual root is not a row.
func Visible(root *Node) []Row {
	var out []Row
	var walk func(n *Node, depth int)
	walk = func(n *Node, depth int) {
		for _, c := range n.Children {
			out = append(out, Row{Node: c, Depth: depth})
			if c.Expanded && len(c.Children) > 0 {
				walk(c, depth+1)
			}
		}
	}
	walk(root, 0)
	return out
}

// ParentIndex returns the row index of rows[i]'s parent, or -1 when rows[i]
// is top-level. In a preorder flattening the parent is the nearest earlier
// row with a smaller depth.
func ParentIndex(rows []Row, i int) int {
	for j := i - 1; j >= 0; j-- {
		if rows[j].Depth < rows[i].Depth {
			return j
		}
	}
	return -1
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/tree/ -v`
Expected: PASS (all tests so far)

- [ ] **Step 5: Commit**

```bash
git add internal/tui/tree/flatten.go internal/tui/tree/flatten_test.go
git commit -m "tree: flatten expanded nodes into drawable rows"
```

---

### Task 4: Live filter (`FilterTree`, `Matches`)

**Files:**
- Create: `internal/tui/tree/filter.go`
- Test: `internal/tui/tree/filter_test.go`

**Interfaces:**
- Consumes: `*Node` from Task 2.
- Produces (used by the model and view):

```go
func FilterTree(root *Node, q string) *Node // pruned copy; nil when nothing matches; q=="" returns root unchanged
func Matches(n *Node, q string) bool        // case-insensitive substring on Label, or Parent.Name for symbols
```

Semantics: a node is kept if it matches or any descendant matches. Ancestors of matches are `Expanded` so every match is visible. A node that itself matches keeps its whole subtree (collapsed) so the user can drill in. The returned tree is a copy — clearing the filter restores the original untouched.

- [ ] **Step 1: Write the failing test**

Create `internal/tui/tree/filter_test.go`:

```go
package tree

import "testing"

func TestFilterTreeExpandsPathsToMatches(t *testing.T) {
	root := BuildTree(fixtureSymbols())
	f := FilterTree(root, "fresh")
	if f == nil {
		t.Fatal("expected a match for 'fresh'")
	}
	rows := Visible(f)
	got := rowLabels(rows)
	// Ancestor chain auto-expanded: internal → query → query.go → Fresh.
	want := []string{"internal", "query", "query.go", "Fresh"}
	if len(got) != 4 {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rows = %v, want %v", got, want)
		}
	}
}

func TestFilterTreeCaseInsensitive(t *testing.T) {
	root := BuildTree(fixtureSymbols())
	if FilterTree(root, "STORE") == nil {
		t.Fatal("filter should be case-insensitive")
	}
}

func TestFilterTreeMatchKeepsSubtreeCollapsed(t *testing.T) {
	root := BuildTree(fixtureSymbols())
	f := FilterTree(root, "store.go")
	rows := Visible(f)
	got := rowLabels(rows)
	// store.go matches; its symbols stay present but collapsed.
	if got[len(got)-1] != "store.go" {
		t.Fatalf("rows = %v, want to end at store.go", got)
	}
	last := rows[len(rows)-1].Node
	if len(last.Children) != 2 || last.Expanded {
		t.Fatalf("matched node should keep children collapsed: %+v", last)
	}
}

func TestFilterTreeNoMatch(t *testing.T) {
	root := BuildTree(fixtureSymbols())
	if f := FilterTree(root, "zzzznothing"); f != nil {
		t.Fatalf("expected nil, got %v", labels(f))
	}
}

func TestFilterTreeDoesNotMutateOriginal(t *testing.T) {
	root := BuildTree(fixtureSymbols())
	FilterTree(root, "fresh")
	if child(t, root, "internal").Expanded {
		t.Fatal("filter mutated the original tree")
	}
}

func TestFilterTreeEmptyQueryReturnsRoot(t *testing.T) {
	root := BuildTree(fixtureSymbols())
	if FilterTree(root, "") != root {
		t.Fatal("empty query should return the original root")
	}
}

func TestMatchesQualifiedName(t *testing.T) {
	root := BuildTree(fixtureSymbols())
	storeGo := child(t, child(t, child(t, root, "internal"), "graph"), "store.go")
	closeM := child(t, child(t, storeGo, "Store"), "Close")
	if !Matches(closeM, "store.close") {
		t.Fatal("qualified name should match")
	}
	if Matches(closeM, "fresh") {
		t.Fatal("unrelated query should not match")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/tree/ -run TestFilter -v`
Expected: FAIL to compile (`FilterTree`, `Matches` undefined).

- [ ] **Step 3: Write the implementation**

Create `internal/tui/tree/filter.go`:

```go
package tree

import "strings"

// FilterTree returns a pruned copy of root: nodes that match q, plus their
// ancestors (expanded, so every match is visible). A matching node keeps its
// whole subtree, collapsed. Returns nil when nothing matches; returns root
// itself when q is empty.
func FilterTree(root *Node, q string) *Node {
	if strings.TrimSpace(q) == "" {
		return root
	}
	return filterNode(root, strings.ToLower(q))
}

func filterNode(n *Node, needle string) *Node {
	var kids []*Node
	for _, c := range n.Children {
		if matchesLower(c, needle) {
			cp := *c
			cp.Expanded = false
			kids = append(kids, &cp)
			continue
		}
		if fc := filterNode(c, needle); fc != nil {
			kids = append(kids, fc)
		}
	}
	if kids == nil {
		return nil
	}
	cp := *n
	cp.Children = kids
	cp.Expanded = true
	return &cp
}

// Matches reports whether the node matches the query (case-insensitive
// substring on the label, or on Parent.Name for symbols).
func Matches(n *Node, q string) bool {
	return matchesLower(n, strings.ToLower(q))
}

func matchesLower(n *Node, needle string) bool {
	if strings.Contains(strings.ToLower(n.Label), needle) {
		return true
	}
	if n.Kind == KindSymbol && n.SymParent != "" {
		return strings.Contains(strings.ToLower(n.SymParent+"."+n.SymName), needle)
	}
	return false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/tree/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/tree/filter.go internal/tui/tree/filter_test.go
git commit -m "tree: live filter with ancestor auto-expand"
```

---

### Task 5: Static non-TTY renderer (`Static`)

**Files:**
- Create: `internal/tui/tree/static.go`
- Test: `internal/tui/tree/static_test.go`

**Interfaces:**
- Consumes: `*Node` from Task 2.
- Produces: `func Static(root *Node) string` — fully expanded plain-text tree: dirs as `name/`, files as `name`, symbols as `name  kind  :line`, two-space indent per depth. Used by Task 8's non-TTY path.

- [ ] **Step 1: Write the failing golden test**

Create `internal/tui/tree/static_test.go`:

```go
package tree

import "testing"

func TestStaticRendersFullTree(t *testing.T) {
	got := Static(BuildTree(fixtureSymbols()))
	want := `internal/
  graph/
    store.go
      Store  type  :5
        Close  method  :40
      open  func  :100
  query/
    query.go
      Fresh  func  :10
      Ghost.Orphan  method  :30
main.go
  main  func  :1
`
	if got != want {
		t.Errorf("static output mismatch:\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func TestStaticEmpty(t *testing.T) {
	if got := Static(BuildTree(nil)); got != "" {
		t.Errorf("empty tree should render empty string, got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/tree/ -run TestStatic -v`
Expected: FAIL to compile (`Static` undefined).

- [ ] **Step 3: Write the implementation**

Create `internal/tui/tree/static.go`:

```go
package tree

import (
	"fmt"
	"strings"
)

// Static renders the fully expanded tree as plain indented text — the
// non-TTY output of the tree command.
func Static(root *Node) string {
	var b strings.Builder
	var walk func(n *Node, depth int)
	walk = func(n *Node, depth int) {
		for _, c := range n.Children {
			b.WriteString(strings.Repeat("  ", depth))
			switch c.Kind {
			case KindDir:
				b.WriteString(c.Label + "/")
			case KindFile:
				b.WriteString(c.Label)
			case KindSymbol:
				fmt.Fprintf(&b, "%s  %s  :%d", c.Label, c.SymKind, c.Line)
			}
			b.WriteByte('\n')
			walk(c, depth+1)
		}
	}
	walk(root, 0)
	return b.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/tree/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/tree/static.go internal/tui/tree/static_test.go
git commit -m "tree: static indented renderer for non-TTY output"
```

---

### Task 6: Bubble Tea model and key handling

**Files:**
- Modify: `go.mod`, `go.sum` (add bubbletea + lipgloss)
- Create: `internal/tui/tree/model.go`
- Test: `internal/tui/tree/model_test.go`

**Interfaces:**
- Consumes: `BuildTree`, `Visible`, `ParentIndex`, `FilterTree` (Tasks 2–4); `graph.Caller`/`graph.Callee` types.
- Produces (used by Task 7's view and Task 8's wiring):

```go
type CountSource interface { // *graph.Store satisfies this
	Callers(name, parent string) ([]graph.Caller, error)
	Callees(name, parent string) ([]graph.Callee, error)
}
func NewModel(repoName string, root *Node, total int, source CountSource) Model
// Model implements tea.Model. Internal state the view reads:
//   rows []Row, cursor int, offset int, width/height int,
//   filtering bool, query string, counts map[string]symCounts
//   (symCounts{callers, callees int; err bool}, keyed SymParent+"\x00"+SymName)
// m.current() *Node — node under cursor; m.treeHeight() int — height minus chrome (4 lines)
```

Key behavior (normal mode): `q`/`ctrl+c` quit; `up`/`k`, `down`/`j` move; `right`/`l` expand; `left`/`h` collapse, or jump to parent when already collapsed; `enter` toggles; `/` enters filter mode; `esc` clears an applied filter. Filter mode: runes append to query (live re-filter), `backspace` deletes, `esc` clears and exits, `enter` keeps the filter and exits typing. Counts are fetched synchronously on cursor landing on a symbol, cached per `parent.name`.

- [ ] **Step 1: Add the dependencies**

```bash
go get github.com/charmbracelet/bubbletea@latest github.com/charmbracelet/lipgloss@latest
go mod tidy
```

Expected: `go.mod` gains both under `require`; `go build ./...` still succeeds.

- [ ] **Step 2: Write the failing test**

Create `internal/tui/tree/model_test.go`:

```go
package tree

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"codeindex/internal/graph"
)

// fakeCounts is a CountSource with canned results.
type fakeCounts struct{ calls int }

func (f *fakeCounts) Callers(name, parent string) ([]graph.Caller, error) {
	f.calls++
	return make([]graph.Caller, 3), nil
}
func (f *fakeCounts) Callees(name, parent string) ([]graph.Callee, error) {
	return make([]graph.Callee, 1), nil
}

func key(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func newTestModel(t *testing.T, src CountSource) Model {
	t.Helper()
	m := NewModel("repo", BuildTree(fixtureSymbols()), 6, src)
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return upd.(Model)
}

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		upd, _ := m.Update(key(k))
		m = upd.(Model)
	}
	return m
}

func TestNavigationMovesCursor(t *testing.T) {
	m := newTestModel(t, nil)
	if m.cursor != 0 {
		t.Fatalf("initial cursor = %d", m.cursor)
	}
	m = press(t, m, "down", "j")
	// Only 2 top-level rows: cursor clamps at 1.
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.cursor)
	}
	m = press(t, m, "up")
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.cursor)
	}
}

func TestExpandCollapseAndParentJump(t *testing.T) {
	m := newTestModel(t, nil)
	m = press(t, m, "right") // expand internal/
	if len(m.rows) != 4 {
		t.Fatalf("rows after expand = %d, want 4", len(m.rows))
	}
	m = press(t, m, "down") // onto graph/
	m = press(t, m, "left") // collapsed already → jump to parent internal/
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 (parent)", m.cursor)
	}
	m = press(t, m, "enter") // toggle internal/ closed
	if len(m.rows) != 2 {
		t.Fatalf("rows after collapse = %d, want 2", len(m.rows))
	}
}

func TestQuitKeys(t *testing.T) {
	m := newTestModel(t, nil)
	if _, cmd := m.Update(key("q")); cmd == nil {
		t.Fatal("q should quit")
	}
	if _, cmd := m.Update(key("ctrl+c")); cmd == nil {
		t.Fatal("ctrl+c should quit")
	}
}

func TestFilterModeLiveNarrowing(t *testing.T) {
	m := newTestModel(t, nil)
	m = press(t, m, "/")
	if !m.filtering {
		t.Fatal("/ should enter filter mode")
	}
	m = press(t, m, "f", "r", "e", "s", "h")
	got := rowLabels(m.rows)
	if len(got) != 4 || got[3] != "Fresh" {
		t.Fatalf("filtered rows = %v, want path to Fresh", got)
	}
	// enter keeps the filter, exits typing.
	m = press(t, m, "enter")
	if m.filtering || m.query != "fresh" || len(m.rows) != 4 {
		t.Fatalf("after enter: filtering=%v query=%q rows=%d", m.filtering, m.query, len(m.rows))
	}
	// esc in nav mode clears the applied filter.
	m = press(t, m, "esc")
	if m.query != "" || len(m.rows) != 2 {
		t.Fatalf("after esc: query=%q rows=%d", m.query, len(m.rows))
	}
}

func TestFilterEscAndBackspace(t *testing.T) {
	m := newTestModel(t, nil)
	m = press(t, m, "/", "x", "y", "backspace")
	if m.query != "x" {
		t.Fatalf("query = %q, want x", m.query)
	}
	m = press(t, m, "esc")
	if m.filtering || m.query != "" || len(m.rows) != 2 {
		t.Fatalf("esc should clear: filtering=%v query=%q rows=%d", m.filtering, m.query, len(m.rows))
	}
}

func TestFilterNoMatchShowsEmpty(t *testing.T) {
	m := newTestModel(t, nil)
	m = press(t, m, "/", "z", "z", "z", "z")
	if len(m.rows) != 0 {
		t.Fatalf("rows = %d, want 0 for no match", len(m.rows))
	}
	// Navigation on an empty row set must not panic.
	m = press(t, m, "down", "up", "enter")
}

func TestCountsFetchedLazilyAndCached(t *testing.T) {
	src := &fakeCounts{}
	m := newTestModel(t, src)
	// Drill to a symbol: expand internal, graph, store.go, then move onto Store.
	m = press(t, m, "right", "down", "right", "down", "right", "down")
	n := m.current()
	if n == nil || n.Kind != KindSymbol {
		t.Fatalf("cursor not on a symbol: %+v", n)
	}
	c, ok := m.counts[n.SymParent+"\x00"+n.SymName]
	if !ok || c.callers != 3 || c.callees != 1 {
		t.Fatalf("counts = %+v ok=%v", c, ok)
	}
	before := src.calls
	m = press(t, m, "up", "down") // revisit same symbol
	if src.calls != before {
		t.Fatalf("counts not cached: %d calls, want %d", src.calls, before)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/tui/tree/ -run 'TestNavigation|TestExpand|TestQuit|TestFilter|TestCounts' -v`
Expected: FAIL to compile (`Model`, `NewModel`, `CountSource` undefined).

- [ ] **Step 4: Write the implementation**

Create `internal/tui/tree/model.go`:

```go
package tree

import (
	tea "github.com/charmbracelet/bubbletea"

	"codeindex/internal/graph"
)

// CountSource supplies caller/callee counts for the detail pane.
// *graph.Store satisfies it.
type CountSource interface {
	Callers(name, parent string) ([]graph.Caller, error)
	Callees(name, parent string) ([]graph.Callee, error)
}

type symCounts struct {
	callers, callees int
	err              bool
}

// Model is the Bubble Tea model for the tree explorer.
type Model struct {
	repoName string
	root     *Node
	total    int
	source   CountSource

	rows   []Row
	cursor int
	offset int
	width  int
	height int

	filtering bool
	query     string
	filtered  *Node // non-nil while a filter is applied

	counts map[string]symCounts
}

func NewModel(repoName string, root *Node, total int, source CountSource) Model {
	m := Model{repoName: repoName, root: root, total: total, source: source,
		counts: map[string]symCounts{}}
	m.refresh()
	return m
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.scrollIntoView()
		return m, nil
	case tea.KeyMsg:
		if m.filtering {
			return m.updateFilter(msg)
		}
		return m.updateNav(msg)
	}
	return m, nil
}

func (m Model) updateNav(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		m.scrollIntoView()
		m.fetchCounts()
	case "down", "j":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
		m.scrollIntoView()
		m.fetchCounts()
	case "right", "l":
		if n := m.current(); n != nil && len(n.Children) > 0 && !n.Expanded {
			n.Expanded = true
			m.refresh()
		}
	case "left", "h":
		if n := m.current(); n != nil {
			if n.Expanded {
				n.Expanded = false
				m.refresh()
			} else if p := ParentIndex(m.rows, m.cursor); p >= 0 {
				m.cursor = p
				m.scrollIntoView()
			}
		}
	case "enter":
		if n := m.current(); n != nil && len(n.Children) > 0 {
			n.Expanded = !n.Expanded
			m.refresh()
		}
	case "/":
		m.filtering = true
	case "esc":
		if m.query != "" {
			m.clearFilter()
		}
	}
	return m, nil
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.filtering = false
		m.clearFilter()
	case "enter":
		m.filtering = false
	case "backspace":
		if len(m.query) > 0 {
			r := []rune(m.query)
			m.query = string(r[:len(r)-1])
			m.applyFilter()
		}
	default:
		if msg.Type == tea.KeyRunes {
			m.query += string(msg.Runes)
			m.applyFilter()
		}
	}
	return m, nil
}

// current returns the node under the cursor, nil when there are no rows.
func (m *Model) current() *Node {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return m.rows[m.cursor].Node
}

// refresh recomputes visible rows from the active tree and re-clamps
// cursor, scroll, and counts.
func (m *Model) refresh() {
	src := m.root
	if m.filtered != nil {
		src = m.filtered
	}
	m.rows = Visible(src)
	if m.cursor > len(m.rows)-1 {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.scrollIntoView()
	m.fetchCounts()
}

func (m *Model) applyFilter() {
	if m.query == "" {
		m.filtered = nil
	} else if f := FilterTree(m.root, m.query); f != nil {
		m.filtered = f
	} else {
		m.filtered = &Node{Kind: KindDir, Expanded: true} // no matches: empty tree
	}
	m.cursor, m.offset = 0, 0
	m.refresh()
}

func (m *Model) clearFilter() {
	m.query = ""
	m.filtered = nil
	m.cursor, m.offset = 0, 0
	m.refresh()
}

// treeHeight is the drawable row count: total height minus header, footer,
// and the pane's top/bottom borders.
func (m *Model) treeHeight() int {
	h := m.height - 4
	if h < 1 {
		h = 1
	}
	return h
}

func (m *Model) scrollIntoView() {
	h := m.treeHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// fetchCounts lazily loads caller/callee counts for the selected symbol.
// Failures are cached as err so the detail pane shows "—" without retry loops.
func (m *Model) fetchCounts() {
	n := m.current()
	if n == nil || n.Kind != KindSymbol || m.source == nil {
		return
	}
	key := n.SymParent + "\x00" + n.SymName
	if _, ok := m.counts[key]; ok {
		return
	}
	callers, err1 := m.source.Callers(n.SymName, n.SymParent)
	callees, err2 := m.source.Callees(n.SymName, n.SymParent)
	m.counts[key] = symCounts{callers: len(callers), callees: len(callees),
		err: err1 != nil || err2 != nil}
}
```

Note: `Model` methods with pointer receivers mutate the local copy inside the value-receiver `Update` — that copy is what gets returned, so the mutations stick. This is standard Bubble Tea practice.

Add a temporary `View` stub so the package compiles (Task 7 replaces it). Put it at the bottom of `model.go`:

```go
// View is implemented in view.go (Task 7). Temporary stub.
func (m Model) View() string { return "" }
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/tui/tree/ -v`
Expected: PASS (all tests)

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/tui/tree/model.go internal/tui/tree/model_test.go
git commit -m "tree: Bubble Tea model with navigation, filter mode, lazy counts"
```

---

### Task 7: View rendering and styles

**Files:**
- Create: `internal/tui/tree/styles.go`
- Create: `internal/tui/tree/view.go`
- Modify: `internal/tui/tree/model.go` (delete the `View` stub added in Task 6)
- Test: `internal/tui/tree/view_test.go`

**Interfaces:**
- Consumes: `Model` internals from Task 6 (`rows`, `cursor`, `offset`, `width`, `height`, `filtering`, `query`, `counts`, `current()`, `treeHeight()`), `Matches` from Task 4.
- Produces: `func (m Model) View() string` — completes the `tea.Model` interface.

Layout rules (from spec): header line (repo name left, symbol count right); two rounded-border panes — tree 60% / detail 40%; detail pane hidden below 80 columns; footer key hints (filter prompt while typing); "terminal too small" notice below 40×10.

- [ ] **Step 1: Write the failing smoke test**

Create `internal/tui/tree/view_test.go`:

```go
package tree

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestViewSmoke(t *testing.T) {
	m := newTestModel(t, &fakeCounts{})
	out := m.View()
	for _, want := range []string{"codeindex tree", "repo", "6 symbols",
		"internal", "main.go", "q quit"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q", want)
		}
	}
}

func TestViewDetailPaneShowsSymbol(t *testing.T) {
	m := newTestModel(t, &fakeCounts{})
	m = press(t, m, "right", "down", "right", "down", "right", "down") // onto Store
	out := m.View()
	for _, want := range []string{"type Store struct", "store.go:5",
		"called by", "3", "calls", "1"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail pane missing %q", want)
		}
	}
}

func TestViewNarrowHidesDetailPane(t *testing.T) {
	m := newTestModel(t, &fakeCounts{})
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	m = upd.(Model)
	m = press(t, m, "right", "down", "right", "down", "right", "down")
	if out := m.View(); strings.Contains(out, "called by") {
		t.Error("detail pane should be hidden below 80 columns")
	}
}

func TestViewTooSmall(t *testing.T) {
	m := newTestModel(t, nil)
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 30, Height: 8})
	m = upd.(Model)
	if out := m.View(); !strings.Contains(out, "too small") {
		t.Errorf("expected too-small notice, got %q", out)
	}
}

func TestViewFilterPrompt(t *testing.T) {
	m := newTestModel(t, nil)
	m = press(t, m, "/", "f", "r")
	if out := m.View(); !strings.Contains(out, "filter: fr") {
		t.Error("footer should show the live filter prompt")
	}
}

func TestViewNeverPanicsWhileNavigating(t *testing.T) {
	m := newTestModel(t, &fakeCounts{})
	for _, k := range []string{"right", "down", "right", "down", "down",
		"down", "down", "left", "/", "z", "z", "esc", "up"} {
		m = press(t, m, k)
		_ = m.View()
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/tree/ -run TestView -v`
Expected: FAIL — the stub `View` returns `""`, so every `strings.Contains` assertion fails.

- [ ] **Step 3: Write the styles**

Create `internal/tui/tree/styles.go`:

```go
package tree

import "github.com/charmbracelet/lipgloss"

// Adaptive colors keep the view readable on both light and dark terminals.
var (
	accent    = lipgloss.AdaptiveColor{Light: "63", Dark: "111"}
	muted     = lipgloss.AdaptiveColor{Light: "245", Dark: "243"}
	borderCol = lipgloss.AdaptiveColor{Light: "250", Dark: "238"}
	cursorBg  = lipgloss.AdaptiveColor{Light: "254", Dark: "236"}

	headerStyle = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	countStyle  = lipgloss.NewStyle().Foreground(muted)
	footerStyle = lipgloss.NewStyle().Foreground(muted).Padding(0, 1)
	paneStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
			BorderForeground(borderCol).Padding(0, 1)
	cursorStyle = lipgloss.NewStyle().Background(cursorBg).Bold(true)
	dirStyle    = lipgloss.NewStyle().Bold(true)
	badgeStyle  = lipgloss.NewStyle().Foreground(muted)
	matchStyle  = lipgloss.NewStyle().Foreground(accent)
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(accent)
)
```

- [ ] **Step 4: Write the view; delete the stub**

Delete the `View` stub from `model.go`, then create `internal/tui/tree/view.go`:

```go
package tree

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	if m.width < 40 || m.height < 10 {
		return "terminal too small — enlarge to at least 40×10\n"
	}
	showDetail := m.width >= 80
	treeW := m.width
	if showDetail {
		treeW = m.width * 6 / 10
	}
	h := m.treeHeight()

	// paneStyle's border+padding add 4 columns; Width/Height size the inside.
	treePane := paneStyle.Width(treeW - 4).Height(h).Render(m.renderRows(treeW-4, h))
	body := treePane
	if showDetail {
		detailW := m.width - treeW
		detailPane := paneStyle.Width(detailW - 4).Height(h).
			Render(m.renderDetail(detailW - 4))
		body = lipgloss.JoinHorizontal(lipgloss.Top, treePane, detailPane)
	}
	return m.renderHeader() + "\n" + body + "\n" + m.renderFooter()
}

func (m Model) renderHeader() string {
	left := headerStyle.Render("codeindex tree — " + m.repoName)
	right := countStyle.Render(fmt.Sprintf("%d symbols", m.total))
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) renderFooter() string {
	if m.filtering {
		return footerStyle.Render("filter: " + m.query + "▌  esc clear · enter keep")
	}
	hint := "↑↓ move · ←→ collapse/expand · / filter · enter toggle · q quit"
	if m.query != "" {
		hint = "filtered: “" + m.query + "” · esc clear · " + hint
	}
	return footerStyle.Render(hint)
}

func (m Model) renderRows(w, h int) string {
	if len(m.rows) == 0 {
		return badgeStyle.Render("no matches")
	}
	var b strings.Builder
	end := m.offset + h
	if end > len(m.rows) {
		end = len(m.rows)
	}
	for i := m.offset; i < end; i++ {
		b.WriteString(m.renderRow(m.rows[i], i == m.cursor, w))
		if i < end-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (m Model) renderRow(r Row, selected bool, w int) string {
	aff := "  "
	if len(r.Node.Children) > 0 {
		aff = "▸ "
		if r.Node.Expanded {
			aff = "▾ "
		}
	}
	label := r.Node.Label
	if r.Node.Kind == KindDir {
		label += "/"
	}
	badge := ""
	if r.Node.Kind == KindSymbol {
		badge = r.Node.SymKind
	}
	avail := w
	if badge != "" {
		avail -= len(badge) + 2
	}
	text := truncate(strings.Repeat("  ", r.Depth)+aff+label, avail)

	if selected {
		if badge != "" {
			text += "  " + badge
		}
		return cursorStyle.Width(w).Render(text)
	}
	switch {
	case m.query != "" && Matches(r.Node, m.query):
		text = matchStyle.Render(text)
	case r.Node.Kind == KindDir:
		text = dirStyle.Render(text)
	}
	if badge != "" {
		text += "  " + badgeStyle.Render(badge)
	}
	return text
}

func (m Model) renderDetail(w int) string {
	n := m.current()
	if n == nil {
		return ""
	}
	var b strings.Builder
	switch n.Kind {
	case KindDir:
		b.WriteString(titleStyle.Render(n.Label + "/"))
	case KindFile:
		b.WriteString(titleStyle.Render(n.Label) + "\n")
		b.WriteString(badgeStyle.Render(n.File))
	case KindSymbol:
		name := n.SymName
		if n.SymParent != "" {
			name = n.SymParent + "." + n.SymName
		}
		b.WriteString(titleStyle.Render(truncate(name, w)) + "\n")
		fmt.Fprintf(&b, "%s · %s\n\n", n.SymKind,
			truncate(fmt.Sprintf("%s:%d", n.File, n.Line), w-len(n.SymKind)-3))
		if n.Signature != "" {
			b.WriteString(wrap(n.Signature, w) + "\n\n")
		}
		if c, ok := m.counts[n.SymParent+"\x00"+n.SymName]; ok {
			if c.err {
				b.WriteString("called by  —\ncalls      —")
			} else {
				fmt.Fprintf(&b, "called by  %d\ncalls      %d", c.callers, c.callees)
			}
		}
	}
	return b.String()
}

// truncate shortens s to w display cells with a trailing ellipsis.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

// wrap hard-wraps s at w cells (signatures are code: no word wrapping needed).
func wrap(s string, w int) string {
	if w <= 0 {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\n' || col >= w {
			b.WriteByte('\n')
			col = 0
			if r == '\n' {
				continue
			}
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/tui/tree/ -v`
Expected: PASS. Note: lipgloss detects no TTY under `go test` and drops color codes, so `strings.Contains` assertions see plain text — this is expected and fine.

- [ ] **Step 6: Run the full suite**

Run: `go test ./...`
Expected: all packages PASS

- [ ] **Step 7: Commit**

```bash
git add internal/tui/tree/styles.go internal/tui/tree/view.go internal/tui/tree/view_test.go internal/tui/tree/model.go
git commit -m "tree: two-pane lipgloss view with detail panel and filter footer"
```

---

### Task 8: Wire the `tree` subcommand, docs, manual verification

**Files:**
- Create: `cmd/codeindex/tree.go`
- Modify: `cmd/codeindex/main.go` (usage string ~line 32; add `case "tree":` in the dispatch switch before `case "enclosing":`)
- Modify: `README.md` (Commands block, after the `enclosing` line)

**Interfaces:**
- Consumes: `query.Fresh`, `graph.Open`, `Store.ProjectSymbols` (Task 1), `BuildTree` (Task 2), `Static` (Task 5), `NewModel` (Task 6), `progress.IsTTY`.
- Produces: the user-facing `codeindex tree <repo>` command.

- [ ] **Step 1: Create the command runner**

Create `cmd/codeindex/tree.go`:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"codeindex/internal/graph"
	"codeindex/internal/progress"
	"codeindex/internal/query"
	tuitree "codeindex/internal/tui/tree"
)

// runTree freshens the index and explores it: interactive Bubble Tea UI on a
// TTY, static indented tree otherwise.
func runTree(root string) error {
	if _, err := query.Fresh(root); err != nil {
		return err
	}
	st, err := graph.Open(filepath.Join(root, ".codeindex", "graph.db"))
	if err != nil {
		return err
	}
	defer st.Close()

	syms, err := st.ProjectSymbols()
	if err != nil {
		return err
	}
	if len(syms) == 0 {
		fmt.Println("index is empty: no symbols found")
		return nil
	}
	node := tuitree.BuildTree(syms)

	if !progress.IsTTY(os.Stdout) {
		fmt.Print(tuitree.Static(node))
		return nil
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	m := tuitree.NewModel(filepath.Base(abs), node, len(syms), st)
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}
```

- [ ] **Step 2: Add the dispatch case and usage entry**

In `cmd/codeindex/main.go`, add to the switch (before `case "enclosing":`):

```go
	case "tree":
		if err := runTree(root); err != nil {
			fatal(err)
		}
```

And in the usage string, add `tree` after `grep`:

```go
		"usage: codeindex <build|refresh|status|callers|callees|impact|dependents|deps|find|grep|tree|depmap|attach|export|import|enclosing|mcp|bench> <repo-root> ...")
```

- [ ] **Step 3: Build and run the full suite**

Run: `go build ./... && go test ./...`
Expected: builds clean, all tests PASS.

- [ ] **Step 4: Verify the non-TTY path against this repo**

Run: `go run ./cmd/codeindex tree . | head -30`
Expected: a static indented tree starting with top-level dirs (e.g. `bench/`, `cmd/`), files beneath, symbols as `Name  kind  :line`. No ANSI escapes, exit 0.

Run: `echo $?`
Expected: `0`

- [ ] **Step 5: Verify the interactive path manually**

This step needs a real terminal — if executing this plan as a subagent, report it for the human to check instead of skipping silently.

Run in a terminal: `go run ./cmd/codeindex tree .`
Check: two bordered panes; arrow keys move; `→` expands; `/` narrows live and auto-expands to matches; selecting a symbol shows signature and `called by`/`calls` counts; `q` quits and restores the screen; resizing below 40×10 shows the too-small notice and recovers.

- [ ] **Step 6: Update README**

In `README.md`, add to the Commands block after the `enclosing` line:

```
codeindex tree <repo>                     interactive tree explorer (static print when piped)
```

- [ ] **Step 7: Commit**

```bash
git add cmd/codeindex/tree.go cmd/codeindex/main.go README.md
git commit -m "cli: add tree subcommand — interactive index explorer"
```
