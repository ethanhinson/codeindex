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
