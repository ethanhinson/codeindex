package index

import (
	"path/filepath"
	"testing"

	"codeindex/internal/lore"
)

func openStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "lore.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUpsertGetAll(t *testing.T) {
	s := openStore(t)
	r := lore.Record{
		ID: "dec-A", Type: lore.TypeDecision, Title: "T", Status: "active",
		Date:    "2026-07-29",
		Anchors: []lore.Anchor{{Symbol: "Foo"}},
		Refs:    []lore.Ref{{Kind: "gh-issue", Value: "e/x#1"}},
		Body:    "body",
	}
	if err := s.Upsert(r, "repo", "/repo/.lore/decisions/a.md"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Get("dec-A")
	if err != nil || !ok {
		t.Fatalf("get: %v ok=%v", err, ok)
	}
	if got.Layer != "repo" || got.Title != "T" || len(got.Anchors) != 1 ||
		got.Anchors[0].Symbol != "Foo" || got.Refs[0].Kind != "gh-issue" {
		t.Fatalf("got %+v", got)
	}
	// Upsert replaces children, not duplicates them.
	r.Anchors = []lore.Anchor{{Path: "internal/"}}
	if err := s.Upsert(r, "repo", "/repo/.lore/decisions/a.md"); err != nil {
		t.Fatal(err)
	}
	got, _, _ = s.Get("dec-A")
	if len(got.Anchors) != 1 || got.Anchors[0].Path != "internal/" {
		t.Fatalf("anchors after re-upsert: %+v", got.Anchors)
	}
	all, err := s.All()
	if err != nil || len(all) != 1 {
		t.Fatalf("all: %v n=%d", err, len(all))
	}
}

func TestDeleteByFileAndHashes(t *testing.T) {
	s := openStore(t)
	r := lore.Record{ID: "note-B", Type: lore.TypeNote, Title: "n", Date: "2026-07-29"}
	if err := s.Upsert(r, "overlay", "/o/notes/b.md"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFileHash("/o/notes/b.md", "h1"); err != nil {
		t.Fatal(err)
	}
	m, err := s.FileHashes()
	if err != nil || m["/o/notes/b.md"] != "h1" {
		t.Fatalf("hashes %v %v", m, err)
	}
	if err := s.DeleteByFile("/o/notes/b.md"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFileHash("/o/notes/b.md"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Get("note-B"); ok {
		t.Fatal("record survived DeleteByFile")
	}
}

func TestGetMissingAndDirect(t *testing.T) {
	s := openStore(t)
	if _, ok, err := s.Get("dec-NONE"); err != nil || ok {
		t.Fatalf("missing get: ok=%v err=%v", ok, err)
	}
	r := lore.Record{ID: "itm-X", Type: lore.TypeItem, Title: "t", Date: "2026-07-29",
		Tags: []string{"a"}, BlockedBy: []string{"itm-Y"}}
	if err := s.Upsert(r, "repo", "/f.md"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Get("itm-X")
	if err != nil || !ok || got.Tags[0] != "a" || got.BlockedBy[0] != "itm-Y" {
		t.Fatalf("direct get: %+v ok=%v err=%v", got, ok, err)
	}
}

func TestUpsertPreservesStale(t *testing.T) {
	s := openStore(t)
	r := lore.Record{ID: "dec-S", Type: lore.TypeDecision, Title: "t", Date: "2026-07-29"}
	if err := s.Upsert(r, "repo", "/s.md"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetStale("dec-S", true); err != nil {
		t.Fatal(err)
	}
	if err := s.Upsert(r, "repo", "/s.md"); err != nil {
		t.Fatal(err)
	}
	got, _, _ := s.Get("dec-S")
	if !got.Stale {
		t.Fatal("re-upsert reset stale; it must be preserved")
	}
}
