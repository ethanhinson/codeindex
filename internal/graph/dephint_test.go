package graph

import (
	"path/filepath"
	"testing"
)

// depEdgeHint reads back the persisted namespace hint (dst_ns) of the single
// edge of kind k emitted from srcFile. The hint is what the resolution ladder's
// import-mediated rung consumes, and DumpNormalized does not select it, so the
// column is read directly.
func depEdgeHint(t *testing.T, st *Store, srcFile, kind string) string {
	t.Helper()
	rows, err := st.db.Query(
		`SELECT dst_ns FROM edges WHERE src_file=? AND kind=?`, srcFile, kind)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var hints []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			t.Fatal(err)
		}
		hints = append(hints, h)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(hints) != 1 {
		t.Fatalf("want exactly one %s edge from %s, got %d: %v", kind, srcFile, len(hints), hints)
	}
	return hints[0]
}

// TestDepHintPrefersEdgeLocalSource pins the insert-time hint SELECTION for
// dependency edges: an edge-local RawDep.Source is strictly more specific than
// the file-level import binding and therefore wins, exactly as the calls path
// prefers RawCall.NsHint over bind[c.Callee]. Both halves are needed — the
// first proves the edge-local source is consulted at all, the second proves the
// file-level fallback that every pre-existing edge relies on is intact.
//
// Deliberately says nothing about which symbol the edge resolves to or with
// what confidence: the disambiguation, last-write-wins and no-regression
// assertions are separate tests. Nothing named Chunk exists in either fixture,
// so resolution is a constant and the hint is the only variable.
func TestDepHintPrefersEdgeLocalSource(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// A file whose import binds the name Chunk to one namespace while the
	// extends edge itself carries a different, more specific one.
	putFile(t, st, &ParsedFile{
		Path: "pkg/local/a.go",
		Symbols: []Symbol{
			{File: "pkg/local/a.go", Name: "A", Kind: KindType, StartLine: 1, EndLine: 3},
		},
		Deps: []RawDep{
			{EnclosingIdx: -1, Kind: KindImports, Target: "Chunk", Source: "example.com/file/level", Line: 1},
			{EnclosingIdx: 0, Kind: KindExtends, Target: "Chunk", Source: "example.com/edge/local", Line: 2},
		},
	})
	if got := depEdgeHint(t, st, "pkg/local/a.go", string(KindExtends)); got != "example.com/edge/local" {
		t.Errorf("edge-local Source must win: hint = %q, want %q", got, "example.com/edge/local")
	}

	// The same shape with no edge-local Source still falls back to the
	// file-level binding — the behavior every non-Go adapter depends on.
	putFile(t, st, &ParsedFile{
		Path: "pkg/fallback/b.go",
		Symbols: []Symbol{
			{File: "pkg/fallback/b.go", Name: "B", Kind: KindType, StartLine: 1, EndLine: 3},
		},
		Deps: []RawDep{
			{EnclosingIdx: -1, Kind: KindImports, Target: "Chunk", Source: "example.com/file/level", Line: 1},
			{EnclosingIdx: 0, Kind: KindExtends, Target: "Chunk", Source: "", Line: 2},
		},
	})
	if got := depEdgeHint(t, st, "pkg/fallback/b.go", string(KindExtends)); got != "example.com/file/level" {
		t.Errorf("empty Source must fall back to the import binding: hint = %q, want %q", got, "example.com/file/level")
	}
}
