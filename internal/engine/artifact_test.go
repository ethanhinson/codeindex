package engine

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codeindex/internal/graph"
)

// The core guarantee: an artifact exported at tree state A, imported after
// the tree mutated to state B (edit + add + delete), patches to exactly what
// a from-scratch build at B produces.
func TestImportThenPatchEqualsRebuild(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a/one.go":   "package a\nfunc One() int { return Two() }\n",
		"a/two.go":   "package a\nfunc Two() int { return 2 }\n",
		"b/three.go": "package b\nfunc Three() int { return 3 }\n",
	})
	artifact := filepath.Join(dir, "artifact.db")
	if _, err := Export(dir, artifact); err != nil {
		t.Fatal(err)
	}

	// Mutate to state B: edit, add, delete.
	os.WriteFile(filepath.Join(dir, "a", "two.go"),
		[]byte("package a\nfunc Two() int { return Three() }\nfunc Four() int { return 4 }\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "c"), 0o755)
	os.WriteFile(filepath.Join(dir, "c", "new.go"),
		[]byte("package c\nfunc New() int { return 5 }\n"), 0o644)
	os.Remove(filepath.Join(dir, "b", "three.go"))

	// Import into a clean .codeindex (simulate a fresh checkout's index dir).
	os.RemoveAll(filepath.Join(dir, ".codeindex"))
	if _, err := Import(dir, artifact); err != nil {
		t.Fatal(err)
	}
	rebuilt := filepath.Join(t.TempDir(), "rebuild.db")
	if _, err := Build(dir, rebuilt); err != nil {
		t.Fatal(err)
	}
	imported := snapshot(t, filepath.Join(dir, ".codeindex", "graph.db"))
	if diff := imported.Diff(snapshot(t, rebuilt)); diff != "" {
		t.Fatalf("imported+patched != full rebuild at state B:\n%s", diff)
	}
}

// Fresh-checkout property: content identical, every mtime different — the
// hash fallback must find nothing to re-parse.
func TestImportMtimeOnlyDriftIsFree(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a/one.go": "package a\nfunc One() int { return 1 }\n",
		"a/two.go": "package a\nfunc Two() int { return 2 }\n",
	})
	artifact := filepath.Join(dir, "artifact.db")
	if _, err := Export(dir, artifact); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour)
	for _, f := range []string{"a/one.go", "a/two.go"} {
		if err := os.Chtimes(filepath.Join(dir, f), future, future); err != nil {
			t.Fatal(err)
		}
	}
	os.RemoveAll(filepath.Join(dir, ".codeindex"))
	st, err := Import(dir, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if st.FilesParsed != 0 || st.Deleted != 0 {
		t.Fatalf("mtime-only drift should patch 0 files; got %+v", st)
	}
}

// A stale artifact (wrong schema version) must be rejected before anything
// is installed.
func TestImportRejectsSchemaMismatch(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a/one.go": "package a\nfunc One() int { return 1 }\n",
	})
	stale := filepath.Join(dir, "stale.db")
	db, err := sql.Open("sqlite3", stale)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, graph.SchemaVersion()-1)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE t(x)`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	_, err = Import(dir, stale)
	if err == nil {
		t.Fatal("schema-mismatch artifact must be rejected")
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".codeindex", "graph.db")); !os.IsNotExist(statErr) {
		t.Fatal("no index should be installed on rejection")
	}
}
