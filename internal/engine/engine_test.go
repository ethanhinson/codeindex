package engine

import (
	"os"
	"path/filepath"
	"testing"

	"codeindex/internal/graph"
)

// writeTree writes files (name->content) into a fresh temp dir and returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func snapshot(t *testing.T, dbPath string) graph.Snapshot {
	t.Helper()
	st, err := graph.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	snap, err := st.DumpNormalized()
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// assertIncrementalEqualsFull is the walking skeleton's core proof: after
// mutating the tree and applying an incremental patch, the graph must equal a
// full rebuild of the mutated tree.
func assertIncrementalEqualsFull(t *testing.T, dir string) {
	t.Helper()
	inc := filepath.Join(dir, "inc.db")
	// (Build of the original tree already happened; caller mutated the tree.)
	if _, err := Patch(dir, inc); err != nil {
		t.Fatalf("patch: %v", err)
	}
	full := filepath.Join(t.TempDir(), "full.db")
	if _, err := Build(dir, full); err != nil {
		t.Fatalf("full build: %v", err)
	}
	if diff := snapshot(t, inc).Diff(snapshot(t, full)); diff != "" {
		t.Fatalf("incremental != full rebuild:\n%s", diff)
	}
}

const fileA = `package p
func Helper(x int) int { return x + 1 }
func A() int { return Helper(1) }
`

const fileB = `package p
func B() int { return Helper(2) }
func C() int { return B() }
`

func TestIncrementalEqualsFull_EditBody(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.go": fileA, "b.go": fileB})
	if _, err := Build(dir, filepath.Join(dir, "inc.db")); err != nil {
		t.Fatal(err)
	}
	// Edit a body (adds a call B->Helper), shifting lines.
	os.WriteFile(filepath.Join(dir, "b.go"),
		[]byte("package p\n\nfunc B() int { return Helper(2) + Helper(3) }\nfunc C() int { return B() }\n"), 0o644)
	assertIncrementalEqualsFull(t, dir)
}

func TestIncrementalEqualsFull_RenameHotSymbol(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.go": fileA, "b.go": fileB})
	if _, err := Build(dir, filepath.Join(dir, "inc.db")); err != nil {
		t.Fatal(err)
	}
	// Rename Helper -> Assist in a.go. Inbound edges (A, B call Helper) must
	// re-resolve: Helper becomes unresolved, Assist newly defined but uncalled.
	os.WriteFile(filepath.Join(dir, "a.go"),
		[]byte("package p\nfunc Assist(x int) int { return x + 1 }\nfunc A() int { return Assist(1) }\n"), 0o644)
	assertIncrementalEqualsFull(t, dir)
}

func TestIncrementalEqualsFull_AddAndDeleteFile(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.go": fileA, "b.go": fileB})
	if _, err := Build(dir, filepath.Join(dir, "inc.db")); err != nil {
		t.Fatal(err)
	}
	// Add a new file that calls Helper, and delete b.go.
	os.WriteFile(filepath.Join(dir, "d.go"),
		[]byte("package p\nfunc D() int { return Helper(9) }\n"), 0o644)
	os.Remove(filepath.Join(dir, "b.go"))
	assertIncrementalEqualsFull(t, dir)
}

func TestAmbiguousResolution(t *testing.T) {
	// Two definitions of Helper -> calls resolve ambiguously, deterministically.
	dir := writeTree(t, map[string]string{
		"a.go": "package p\nfunc Helper() int { return 1 }\nfunc A() int { return Helper() }\n",
		"b.go": "package p\nfunc Helper() int { return 2 }\n",
	})
	db := filepath.Join(dir, "g.db")
	if _, err := Build(dir, db); err != nil {
		t.Fatal(err)
	}
	snap := snapshot(t, db)
	found := false
	for _, e := range snap.Edges {
		if contains(e, "ambiguous") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an ambiguous edge; got %v", snap.Edges)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
