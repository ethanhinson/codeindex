package index

import (
	"os"
	"path/filepath"
	"testing"

	"codeindex/internal/lore"
)

func writeRec(t *testing.T, dir, name, id, title string, typ lore.Type) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	r := lore.Record{ID: id, Type: typ, Title: title, Date: "2026-07-29",
		Status: lore.DefaultStatus(typ), Body: "b\n"}
	b, err := r.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func testLayout(t *testing.T) lore.Layout {
	t.Helper()
	t.Setenv("CODEINDEX_HOME", t.TempDir())
	l, err := lore.NewLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestReindexAddChangeRemove(t *testing.T) {
	l := testLayout(t)
	db := filepath.Join(t.TempDir(), "lore.db")
	p := writeRec(t, l.Dir("repo", lore.TypeDecision), "a.md", "dec-A", "First", lore.TypeDecision)
	writeRec(t, l.Dir("overlay", lore.TypeNote), "n.md", "note-N", "Private", lore.TypeNote)

	s, rep, err := Reindex(l, db)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Indexed != 2 || len(rep.Errors) != 0 {
		t.Fatalf("report %+v", rep)
	}
	all, _ := s.All()
	if len(all) != 2 {
		t.Fatalf("n=%d", len(all))
	}
	s.Close()

	// Unchanged files are not re-parsed (Indexed == 0 on second run).
	s, rep, err = Reindex(l, db)
	if err != nil || rep.Indexed != 0 {
		t.Fatalf("second run: %+v %v", rep, err)
	}
	s.Close()

	// Change + remove are picked up.
	writeRec(t, l.Dir("repo", lore.TypeDecision), "a.md", "dec-A", "Renamed", lore.TypeDecision)
	os.Remove(filepath.Join(l.Dir("overlay", lore.TypeNote), "n.md"))
	s, rep, err = Reindex(l, db)
	if err != nil || rep.Indexed != 1 || rep.Removed != 1 {
		t.Fatalf("third run: %+v %v", rep, err)
	}
	defer s.Close()
	got, ok, _ := s.Get("dec-A")
	if !ok || got.Title != "Renamed" || got.File != p {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
	if _, ok, _ := s.Get("note-N"); ok {
		t.Fatal("removed record still indexed")
	}
}

func TestReindexFailOpenOnMalformed(t *testing.T) {
	l := testLayout(t)
	dir := l.Dir("repo", lore.TypeNote)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "bad.md"), []byte("no frontmatter"), 0o644)
	writeRec(t, dir, "good.md", "note-G", "Good", lore.TypeNote)

	s, rep, err := Reindex(l, filepath.Join(t.TempDir(), "lore.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if len(rep.Errors) != 1 || rep.Indexed != 1 {
		t.Fatalf("report %+v", rep)
	}
}

func TestSessionsIndexAsSessionLayer(t *testing.T) {
	l := testLayout(t)
	writeRec(t, l.SessionsDir(), "s.md", "note-S", "Session note", lore.TypeNote)
	s, _, err := Reindex(l, filepath.Join(t.TempDir(), "lore.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, ok, _ := s.Get("note-S")
	if !ok || got.Layer != "session" {
		t.Fatalf("session layer: %+v ok=%v", got, ok)
	}
}
