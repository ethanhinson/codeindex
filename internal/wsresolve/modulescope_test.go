package wsresolve

import (
	"path/filepath"
	"testing"

	"codeindex/internal/graph"
	"codeindex/internal/overlay"
)

// Module-scope references (§3.3, D3 rung 1).
//
// An import statement sits inside no symbol, so its edge carries
// src_symbol_id = 0 and its ladder source key is {member, file, ""} — the FILE,
// with no enclosing symbol. These are the edges rung 1 was designed for: the
// import IS the namespace hint. The two tests below cover the ladder in
// isolation and the graph-reader-to-ladder seam, because the defect that
// motivated them lived in the seam and a hand-built edge alone cannot see it.

// TestRungOneResolvesAModuleScopeImport is the ladder half: an edge with no
// source symbol resolves at rung 1 like any other and records an empty source
// QName rather than a "." or a dropped row.
func TestRungOneResolvesAModuleScopeImport(t *testing.T) {
	src := newMember(t, "s", []string{"example.com/s"}, nil)
	a := newMember(t, "a", []string{"example.com/a"}, nil, def{"a/a.go", "Boot", "", 3})

	e := graph.UnresolvedEdge{
		SrcFile: "s.go", SrcName: "", SrcParent: "",
		DstName: "Boot", DstNS: "example.com/a", Kind: "imports", Line: 1,
	}
	got := run(t, src, []Member{src, a}, e)
	wantOneCross(t, got, overlay.CrossEdge{
		Src:  overlay.SymKey{Member: "s", File: "s.go", QName: ""},
		Dst:  overlay.SymKey{Member: "a", File: "a/a.go", QName: "Boot"},
		Kind: "imports", Provenance: "cross_repo_import", Confidence: "exact", Line: 1,
	})
}

// TestModuleScopeImportFromTheReaderResolvesExact is the seam: the edge is not
// hand-written but READ from a member index built with a real module-scope
// import, exactly the shape the live corpus has (flask importing
// werkzeug.exceptions.HTTPException at file scope). If UnresolvedEdges drops
// module-scope rows, the ladder gets nothing and this test sees zero records —
// which is the whole defect, invisible to a hand-built edge.
func TestModuleScopeImportFromTheReaderResolvesExact(t *testing.T) {
	st, err := graph.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	tx, err := st.Begin()
	if err != nil {
		t.Fatal(err)
	}
	pf := &graph.ParsedFile{
		Path: "app.py",
		Symbols: []graph.Symbol{
			{File: "app.py", Name: "handle", Kind: graph.KindFunc, StartLine: 10, EndLine: 12},
		},
		// `from example.com/a import Boot` — module scope, no enclosing symbol.
		Deps: []graph.RawDep{
			{EnclosingIdx: -1, Kind: graph.KindImports, Target: "Boot",
				Source: "example.com/a", Line: 1},
		},
	}
	if _, _, err := st.PutFile(tx, pf, graph.FileMeta{Path: "app.py", Hash: "h", Size: 1, Mtime: 1}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	src := Member{ID: "s", Namespaces: []string{"example.com/s"}, Store: st}
	a := newMember(t, "a", []string{"example.com/a"}, nil, def{"a/a.go", "Boot", "", 3})

	edges, err := st.UnresolvedEdges()
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 1 || edges[0].Kind != "imports" || edges[0].SrcName != "" {
		t.Fatalf("reader returned %+v, want exactly the module-scope import edge", edges)
	}

	got := run(t, src, []Member{src, a}, edges...)
	wantOneCross(t, got, overlay.CrossEdge{
		Src:  overlay.SymKey{Member: "s", File: "app.py", QName: ""},
		Dst:  overlay.SymKey{Member: "a", File: "a/a.go", QName: "Boot"},
		Kind: "imports", Provenance: "cross_repo_import", Confidence: "exact", Line: 1,
	})
}
