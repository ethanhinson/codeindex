package graph

import (
	"path/filepath"
	"testing"
)

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
			{EnclosingIdx: 0, Callee: "Helper", Line: 11},                                     // resolved
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
