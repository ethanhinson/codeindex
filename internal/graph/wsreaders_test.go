package graph

import (
	"path/filepath"
	"testing"
)

func putFileWithHash(t *testing.T, st *Store, pf *ParsedFile, hash string) {
	t.Helper()
	tx, err := st.Begin()
	if err != nil {
		t.Fatal(err)
	}
	meta := FileMeta{Path: pf.Path, Hash: hash, Size: 1, Mtime: 1}
	if _, _, err := st.PutFile(tx, pf, meta); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestMemberMerkleRootDeterministic(t *testing.T) {
	st := wsFixture(t)
	a, err := st.MemberMerkleRoot()
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.MemberMerkleRoot()
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("root not deterministic: %q vs %q", a, b)
	}
}

func TestMemberMerkleRootChangesOnFileHashChange(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	pf := &ParsedFile{
		Path: "pkg/a.go",
		Symbols: []Symbol{
			{File: "pkg/a.go", Name: "Boot", Kind: KindFunc, StartLine: 1, EndLine: 2},
		},
	}
	putFileWithHash(t, st, pf, "hash-1")
	r1, err := st.MemberMerkleRoot()
	if err != nil {
		t.Fatal(err)
	}

	putFileWithHash(t, st, pf, "hash-2")
	r2, err := st.MemberMerkleRoot()
	if err != nil {
		t.Fatal(err)
	}

	if r1 == r2 {
		t.Fatalf("root did not change after merkle hash change: %q", r1)
	}
}

// TestMemberMerkleRootChangesOnDepReattach is the load-bearing test: a depmap
// re-attach at a new version must change the root even though no project
// file's bytes changed. A merkle-only fold would miss this, leaving a
// staleness gate to skip a member whose contribution actually moved.
func TestMemberMerkleRootChangesOnDepReattach(t *testing.T) {
	dir := t.TempDir()

	buildMap := func(version string) string {
		mapPath := filepath.Join(dir, "acme-"+version+".map.db")
		m, err := Open(mapPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := m.WriteDepMeta("acme/z", version); err != nil {
			t.Fatal(err)
		}
		if err := m.PutDepSymbols("acme/z", version, "z.go", "maphash", 1, 1, []Symbol{
			{Name: "Zeta", Kind: KindFunc, StartLine: 1, EndLine: 2},
		}); err != nil {
			t.Fatal(err)
		}
		if err := m.Close(); err != nil {
			t.Fatal(err)
		}
		return mapPath
	}

	st, err := Open(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	putFile(t, st, &ParsedFile{
		Path: "pkg/a.go",
		Symbols: []Symbol{
			{File: "pkg/a.go", Name: "Boot", Kind: KindFunc, StartLine: 1, EndLine: 2},
		},
	})

	if _, _, _, err := st.AttachMap(buildMap("v1.0.0"), "vendor/acme"); err != nil {
		t.Fatal(err)
	}
	r1, err := st.MemberMerkleRoot()
	if err != nil {
		t.Fatal(err)
	}

	// Re-attach at a new version. No project file changed.
	if _, _, _, err := st.AttachMap(buildMap("v2.0.0"), "vendor/acme"); err != nil {
		t.Fatal(err)
	}
	r2, err := st.MemberMerkleRoot()
	if err != nil {
		t.Fatal(err)
	}

	if r1 == r2 {
		t.Fatalf("root did not change after dep re-attach at new version: %q", r1)
	}
}

func TestMemberMerkleRootStableAcrossNoOp(t *testing.T) {
	st := wsTierFixture(t)
	r1, err := st.MemberMerkleRoot()
	if err != nil {
		t.Fatal(err)
	}
	// Unrelated no-op reads.
	if _, err := st.UnresolvedEdges(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ProjectDefs("Helper", ""); err != nil {
		t.Fatal(err)
	}
	r2, err := st.MemberMerkleRoot()
	if err != nil {
		t.Fatal(err)
	}
	if r1 != r2 {
		t.Fatalf("root changed across no-op reads: %q vs %q", r1, r2)
	}
}

func TestMemberMerkleRootEmptyIndex(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	r, err := st.MemberMerkleRoot()
	if err != nil {
		t.Fatal(err)
	}
	if r == "" {
		t.Fatal("empty index returned empty root")
	}
}

// wsFixture builds a small index exercising all three edge classes the
// workspace readers must distinguish: resolved, unresolved symbol-sourced,
// and file-level import (src_symbol_id = 0).
func wsFixture(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	// b.go defines Helper, so a.go's call to it resolves.
	putFile(t, st, &ParsedFile{
		Path: "pkg/b.go",
		Symbols: []Symbol{
			{File: "pkg/b.go", Name: "Helper", Kind: KindFunc, StartLine: 1, EndLine: 2},
		},
	})
	putFile(t, st, &ParsedFile{
		Path: "pkg/a.go",
		Symbols: []Symbol{
			{File: "pkg/a.go", Name: "Run", Parent: "Server", Kind: KindMethod, StartLine: 10, EndLine: 20},
			{File: "pkg/a.go", Name: "Boot", Kind: KindFunc, StartLine: 30, EndLine: 40},
		},
		Calls: []RawCall{
			{EnclosingIdx: 0, Callee: "Helper", Line: 11},                                      // resolved
			{EnclosingIdx: 0, Callee: "Zeta", Qualifier: "Client", NsHint: "acme/z", Line: 12}, // unresolved
			{EnclosingIdx: 1, Callee: "Alpha", Line: 31},                                       // unresolved
		},
		Deps: []RawDep{
			// file-level import: no enclosing symbol => src_symbol_id = 0
			{EnclosingIdx: -1, Kind: KindImports, Target: "acme/z", Source: "acme/z", Line: 1},
		},
	})

	// Guard: the exclusion assertions below are vacuous unless the fixture
	// really holds all three edge classes.
	count := func(where string) int {
		var n int
		if err := st.db.QueryRow(`SELECT COUNT(*) FROM edges WHERE ` + where).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`dst_symbol_id != 0`); n == 0 {
		t.Fatalf("fixture has no resolved edge")
	}
	if n := count(`src_symbol_id = 0 AND kind = 'imports'`); n == 0 {
		t.Fatalf("fixture has no file-level import edge")
	}
	return st
}

// wsTierFixture builds an index with an attached depmap so all three
// destination classes exist: tier-1-resolved, tier-0-resolved, and unresolved.
func wsTierFixture(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()

	// A depmap file: tier-1 symbols only, stamped with its namespace.
	mapPath := filepath.Join(dir, "acme.map.db")
	m, err := Open(mapPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.WriteDepMeta("acme/z", "v1.2.3"); err != nil {
		t.Fatal(err)
	}
	if err := m.PutDepSymbols("acme/z", "v1.2.3", "z.go", "maphash", 1, 1, []Symbol{
		{Name: "Zeta", Kind: KindFunc, StartLine: 1, EndLine: 2},
		{Name: "Yankee", Kind: KindFunc, StartLine: 5, EndLine: 6},
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := Open(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	putFile(t, st, &ParsedFile{
		Path: "pkg/b.go",
		Symbols: []Symbol{
			{File: "pkg/b.go", Name: "Helper", Kind: KindFunc, StartLine: 1, EndLine: 2},
		},
	})
	putFile(t, st, &ParsedFile{
		Path: "pkg/a.go",
		Symbols: []Symbol{
			{File: "pkg/a.go", Name: "Run", Parent: "Server", Kind: KindMethod, StartLine: 10, EndLine: 20},
			{File: "pkg/a.go", Name: "Boot", Kind: KindFunc, StartLine: 30, EndLine: 40},
		},
		Calls: []RawCall{
			{EnclosingIdx: 0, Callee: "Helper", Line: 11}, // tier-0 resolved
			{EnclosingIdx: 0, Callee: "Zeta", Line: 12},   // tier-1 after attach
			{EnclosingIdx: 1, Callee: "Yankee", Line: 31}, // tier-1 after attach
			{EnclosingIdx: 1, Callee: "Alpha", Line: 32},  // unresolved
		},
		Deps: []RawDep{
			{EnclosingIdx: -1, Kind: KindImports, Target: "acme/z", Source: "acme/z", Line: 1},
		},
	})

	ns, _, names, err := st.AttachMap(mapPath, "vendor/acme")
	if err != nil {
		t.Fatal(err)
	}
	if ns != "acme/z" {
		t.Fatalf("attached namespace = %q", ns)
	}
	if err := st.ReResolve(names); err != nil {
		t.Fatal(err)
	}

	// Guard: exclusion assertions are vacuous unless every class is present.
	count := func(where string) int {
		var n int
		if err := st.db.QueryRow(`SELECT COUNT(*) FROM edges e
			LEFT JOIN symbols d ON d.id = e.dst_symbol_id WHERE ` + where).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`e.dst_symbol_id != 0 AND d.tier = 1`); n < 2 {
		t.Fatalf("fixture has %d tier-1-resolved edges, want >= 2", n)
	}
	if n := count(`e.dst_symbol_id != 0 AND d.tier = 0`); n == 0 {
		t.Fatalf("fixture has no tier-0-resolved edge")
	}
	if n := count(`e.dst_symbol_id = 0 AND e.src_symbol_id != 0`); n == 0 {
		t.Fatalf("fixture has no unresolved symbol-sourced edge")
	}
	if n := count(`e.src_symbol_id = 0 AND e.kind = 'imports'`); n == 0 {
		t.Fatalf("fixture has no file-level import edge")
	}
	return st
}

func TestTierOneEdges(t *testing.T) {
	st := wsTierFixture(t)
	got, err := st.TierOneEdges()
	if err != nil {
		t.Fatal(err)
	}
	// Ordered by (src_file, src_name, ...) => Boot before Run.
	want := []TierOneEdge{
		{UnresolvedEdge: UnresolvedEdge{SrcFile: "pkg/a.go", SrcName: "Boot",
			DstName: "Yankee", Kind: "calls", Line: 31}, DstNamespace: "acme/z"},
		{UnresolvedEdge: UnresolvedEdge{SrcFile: "pkg/a.go", SrcName: "Run", SrcParent: "Server",
			DstName: "Zeta", Kind: "calls", Line: 12}, DstNamespace: "acme/z"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d edges %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("edge %d: got %+v want %+v", i, got[i], want[i])
		}
	}
	// Exclusions: nothing tier-0-resolved (Helper) or unresolved (Alpha).
	for _, e := range got {
		if e.DstName == "Helper" || e.DstName == "Alpha" {
			t.Errorf("unexpected edge returned: %+v", e)
		}
	}
}

func TestTierOneEdgesDeterministicOrder(t *testing.T) {
	st := wsTierFixture(t)
	a, err := st.TierOneEdges()
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.TierOneEdges()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != len(b) {
		t.Fatalf("length differs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("order not stable at %d: %+v vs %+v", i, a[i], b[i])
		}
	}
	if len(a) < 2 || a[0].SrcName != "Boot" || a[1].SrcName != "Run" {
		t.Fatalf("unexpected order: %+v", a)
	}
}

func TestUnresolvedEdgesExcludesResolvedAndFileLevelImports(t *testing.T) {
	st := wsFixture(t)
	got, err := st.UnresolvedEdges()
	if err != nil {
		t.Fatal(err)
	}
	want := []UnresolvedEdge{
		{SrcFile: "pkg/a.go", SrcName: "Boot", SrcParent: "", DstName: "Alpha", Kind: "calls", Line: 31},
		{SrcFile: "pkg/a.go", SrcName: "Run", SrcParent: "Server", DstName: "Zeta",
			DstQualifier: "Client", DstNS: "acme/z", Kind: "calls", Line: 12},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d edges %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("edge %d: got %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestUnresolvedEdgesDeterministicOrder(t *testing.T) {
	st := wsFixture(t)
	a, err := st.UnresolvedEdges()
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.UnresolvedEdges()
	if err != nil {
		t.Fatal(err)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("order not stable at %d: %+v vs %+v", i, a[i], b[i])
		}
	}
	// (src_file, src_name, ...) => Boot sorts before Run.
	if len(a) < 2 || a[0].SrcName != "Boot" || a[1].SrcName != "Run" {
		t.Fatalf("unexpected order: %+v", a)
	}
}

func TestProjectDefs(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	putFile(t, st, &ParsedFile{
		Path: "src/svc.go",
		Symbols: []Symbol{
			{File: "src/svc.go", Name: "Widget", Kind: KindType, StartLine: 5, EndLine: 9},
			{File: "src/svc.go", Name: "Build", Parent: "Widget", Kind: KindMethod, StartLine: 20, EndLine: 25},
			{File: "src/svc.go", Name: "Build", Parent: "Gadget", Kind: KindMethod, StartLine: 40, EndLine: 45},
		},
	})
	// A vendored (tier-1) symbol of the same name must never be returned.
	putFile(t, st, &ParsedFile{
		Path: "vendor/widget.go",
		Symbols: []Symbol{
			{File: "vendor/widget.go", Name: "Widget", Namespace: "acme/vendored",
				Tier: 1, Kind: KindType, StartLine: 1, EndLine: 3},
		},
	})

	t.Run("tier0 only", func(t *testing.T) {
		got, err := st.ProjectDefs("Widget", "")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d defs %+v, want 1", len(got), got)
		}
		if got[0].File != "src/svc.go" || got[0].Tier != 0 {
			t.Fatalf("got %+v, want the tier-0 src/svc.go definition", got[0])
		}
		if got[0].Namespace == "" {
			t.Errorf("Namespace not populated: %+v", got[0])
		}
		if got[0].QName() != "Widget" {
			t.Errorf("QName = %q", got[0].QName())
		}
	})

	t.Run("bare name returns both parents in order", func(t *testing.T) {
		got, err := st.ProjectDefs("Build", "")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d defs %+v, want 2", len(got), got)
		}
		if got[0].StartLine != 20 || got[1].StartLine != 40 {
			t.Fatalf("wrong order: %+v", got)
		}
	})

	t.Run("parent qualified", func(t *testing.T) {
		got, err := st.ProjectDefs("Build", "Gadget")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Parent != "Gadget" {
			t.Fatalf("got %+v, want only the Gadget method", got)
		}
		if got[0].QName() != "Gadget.Build" {
			t.Errorf("QName = %q", got[0].QName())
		}
	})

	t.Run("no match is empty not error", func(t *testing.T) {
		got, err := st.ProjectDefs("NoSuchThing", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %+v, want empty", got)
		}
		got, err = st.ProjectDefs("Build", "Nobody")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %+v, want empty", got)
		}
	})
}
