package index

import (
	"math"
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

// TestMetaRoundTrip verifies that Meta returns empty string when unset, and
// SetMeta followed by Meta returns the stored value. It also verifies that the
// reserved 'schema' key is not collided with.
// TestAddSignalsConfidenceFormula verifies that AddSignals correctly increments
// survived and churn_lines, and computes confidence = ln(1+survived)/ln(21).
func TestAddSignalsConfidenceFormula(t *testing.T) {
	s := openStore(t)
	r := lore.Record{ID: "dec-SIG", Type: lore.TypeDecision, Title: "t", Date: "2026-07-29"}
	if err := s.Upsert(r, "repo", "/sig.md"); err != nil {
		t.Fatal(err)
	}

	// Initial state: survived=0, churn_lines=0, confidence=0.
	got, _, _ := s.Get("dec-SIG")
	if got.Survived != 0 || got.ChurnLines != 0 || got.Confidence != 0 {
		t.Fatalf("initial: %+v", got)
	}

	// First AddSignals: survived=1, churn=10.
	if err := s.AddSignals("dec-SIG", 1, 10); err != nil {
		t.Fatalf("AddSignals: %v", err)
	}
	got, _, _ = s.Get("dec-SIG")
	if got.Survived != 1 {
		t.Fatalf("survived: want 1, got %d", got.Survived)
	}
	if got.ChurnLines != 10 {
		t.Fatalf("churn_lines: want 10, got %d", got.ChurnLines)
	}
	// confidence = ln(1+1)/ln(21) = ln(2)/ln(21)
	wantConf := math.Log(2) / math.Log(21)
	if math.Abs(got.Confidence-wantConf) > 1e-9 {
		t.Fatalf("confidence: want %.10f, got %.10f", wantConf, got.Confidence)
	}

	// Second AddSignals: survived=2, churn_lines=25.
	if err := s.AddSignals("dec-SIG", 1, 15); err != nil {
		t.Fatalf("AddSignals 2: %v", err)
	}
	got, _, _ = s.Get("dec-SIG")
	if got.Survived != 2 || got.ChurnLines != 25 {
		t.Fatalf("after 2nd: survived=%d churn=%d", got.Survived, got.ChurnLines)
	}
	wantConf2 := math.Log(3) / math.Log(21)
	if math.Abs(got.Confidence-wantConf2) > 1e-9 {
		t.Fatalf("confidence after 2nd: want %.10f, got %.10f", wantConf2, got.Confidence)
	}

	// Accumulated to survived=20 → confidence should be capped at 1.0.
	if err := s.AddSignals("dec-SIG", 18, 0); err != nil {
		t.Fatalf("AddSignals cap: %v", err)
	}
	got, _, _ = s.Get("dec-SIG")
	// survived=20 → ln(21)/ln(21) = 1.0 exactly
	if math.Abs(got.Confidence-1.0) > 1e-9 {
		t.Fatalf("confidence at survived=20: want 1.0, got %.10f", got.Confidence)
	}

	// Adding more should stay capped at 1.0.
	if err := s.AddSignals("dec-SIG", 5, 0); err != nil {
		t.Fatalf("AddSignals over cap: %v", err)
	}
	got, _, _ = s.Get("dec-SIG")
	if got.Confidence > 1.0 {
		t.Fatalf("confidence exceeded 1.0: %f", got.Confidence)
	}
}

// TestAddSignalsNonexistentRecordNoOp verifies AddSignals is a no-op for unknown IDs.
func TestAddSignalsNonexistentRecordNoOp(t *testing.T) {
	s := openStore(t)
	if err := s.AddSignals("dec-GHOST", 1, 5); err != nil {
		t.Fatalf("AddSignals unknown id: %v", err)
	}
}

func TestMetaRoundTrip(t *testing.T) {
	s := openStore(t)

	// Unset key returns empty string, no error.
	v, err := s.Meta("last_scanned_commit")
	if err != nil {
		t.Fatalf("Meta unset key: %v", err)
	}
	if v != "" {
		t.Fatalf("Meta unset key: want empty, got %q", v)
	}

	// SetMeta then Meta round-trips.
	if err := s.SetMeta("last_scanned_commit", "abc123"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	v, err = s.Meta("last_scanned_commit")
	if err != nil {
		t.Fatalf("Meta after set: %v", err)
	}
	if v != "abc123" {
		t.Fatalf("Meta after set: want %q, got %q", "abc123", v)
	}

	// Update works (upsert semantics).
	if err := s.SetMeta("last_scanned_commit", "def456"); err != nil {
		t.Fatalf("SetMeta update: %v", err)
	}
	v, err = s.Meta("last_scanned_commit")
	if err != nil || v != "def456" {
		t.Fatalf("Meta after update: %v %q", err, v)
	}

	// The 'schema' key written by Open is still intact (no collision).
	sv, err := s.Meta("schema")
	if err != nil {
		t.Fatalf("Meta schema: %v", err)
	}
	if sv == "" {
		t.Fatal("Meta schema: schema key must not be empty")
	}
}
