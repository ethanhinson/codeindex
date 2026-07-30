package index

import (
	"os"
	"path/filepath"
	"testing"

	"codeindex/internal/graph"
	"codeindex/internal/lore"
)

func fixtureGraph(t *testing.T, dir string) string {
	t.Helper()
	dbPath := filepath.Join(dir, ".codeindex", "graph.db")
	os.MkdirAll(filepath.Dir(dbPath), 0o755)
	st, err := graph.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	tx, err := st.Begin()
	if err != nil {
		t.Fatal(err)
	}
	pf := &graph.ParsedFile{Path: "a.go", Symbols: []graph.Symbol{
		{File: "a.go", Name: "ResolveImports", Kind: graph.KindFunc,
			Signature: "func ResolveImports()", StartLine: 1, EndLine: 2},
	}}
	if _, _, err := st.PutFile(tx, pf, graph.FileMeta{Path: "a.go", Hash: "h", Size: 1, Mtime: 1}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return dbPath
}

func anchored(id string, a ...lore.Anchor) StoredRecord {
	return StoredRecord{Record: lore.Record{ID: id, Type: lore.TypeDecision, Anchors: a}}
}

func TestStaleRecords(t *testing.T) {
	root := t.TempDir()
	db := fixtureGraph(t, root)
	os.MkdirAll(filepath.Join(root, "internal", "engine"), 0o755)

	recs := []StoredRecord{
		anchored("dec-ok", lore.Anchor{Symbol: "ResolveImports"},
			lore.Anchor{Path: "internal/engine/"}),
		anchored("dec-gone-sym", lore.Anchor{Symbol: "DeletedSymbol"}),
		anchored("dec-gone-path", lore.Anchor{Path: "no/such/dir/"}),
		anchored("dec-unanchored"),
	}
	stale, err := StaleRecords(root, db, recs)
	if err != nil {
		t.Fatal(err)
	}
	if stale["dec-ok"] || !stale["dec-gone-sym"] || !stale["dec-gone-path"] ||
		stale["dec-unanchored"] {
		t.Fatalf("stale map: %+v", stale)
	}
}

func TestStaleRecordsWithoutGraphDB(t *testing.T) {
	root := t.TempDir()
	recs := []StoredRecord{anchored("dec-s", lore.Anchor{Symbol: "Whatever"})}
	stale, err := StaleRecords(root, filepath.Join(root, "missing.db"), recs)
	if err != nil {
		t.Fatal(err)
	}
	if stale["dec-s"] {
		t.Fatal("symbol anchors must be skipped when graph.db is absent")
	}
}
