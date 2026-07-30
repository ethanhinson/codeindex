package index

import (
	"math"
	"path/filepath"
	"strings"
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

// TestInsertEventAndEventsForSHAPrefixes verifies the round-trip for
// InsertEvent and EventsForSHAPrefixes, including both-direction prefix matching.
func TestInsertEventAndEventsForSHAPrefixes(t *testing.T) {
	s := openStore(t)

	// Insert two events with different SHAs.
	const full40 = "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	const other40 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := s.InsertEvent(full40, "deploy", "ok", "prod", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}
	if err := s.InsertEvent(other40, "deploy", "failed", "", "2026-01-02T00:00:00Z"); err != nil {
		t.Fatalf("InsertEvent other: %v", err)
	}

	// Query by 7-char short SHA (prefix of full40) — short is prefix of stored full40.
	short7 := full40[:7]
	evs, err := s.EventsForSHAPrefixes([]string{short7})
	if err != nil {
		t.Fatalf("EventsForSHAPrefixes: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d: %+v", len(evs), evs)
	}
	if evs[0].SHA != full40 || evs[0].Type != "deploy" || evs[0].Status != "ok" {
		t.Fatalf("event mismatch: %+v", evs[0])
	}

	// Query by full40 as prefix → full40 is a prefix of itself.
	evs2, err := s.EventsForSHAPrefixes([]string{full40})
	if err != nil {
		t.Fatalf("EventsForSHAPrefixes full: %v", err)
	}
	if len(evs2) != 1 || evs2[0].SHA != full40 {
		t.Fatalf("full SHA query: %+v", evs2)
	}

	// Reverse direction: stored SHA is a 7-char short, query with full 40-char
	// that starts with those 7 chars. HasPrefix(full40, short7stored) is true.
	s2 := openStore(t)
	const short7stored = "a1b2c3d"
	if err := s2.InsertEvent(short7stored, "test", "ok", "", "2026-01-03T00:00:00Z"); err != nil {
		t.Fatalf("InsertEvent short: %v", err)
	}
	// full40 starts with "a1b2c3d4..." so HasPrefix(full40, short7stored) = true.
	evs3, err := s2.EventsForSHAPrefixes([]string{full40})
	if err != nil {
		t.Fatalf("EventsForSHAPrefixes reverse: %v", err)
	}
	if len(evs3) != 1 {
		t.Fatalf("reverse direction: want 1, got %d: %+v", len(evs3), evs3)
	}
	if evs3[0].SHA != short7stored {
		t.Fatalf("reverse direction: got SHA %q, want %q", evs3[0].SHA, short7stored)
	}

	// Empty prefixes → no results.
	evs4, err := s.EventsForSHAPrefixes(nil)
	if err != nil || len(evs4) != 0 {
		t.Fatalf("empty prefixes: %v %+v", err, evs4)
	}

	// No-match query.
	evs5, err := s.EventsForSHAPrefixes([]string{"00000000"})
	if err != nil || len(evs5) != 0 {
		t.Fatalf("no-match: %v %+v", err, evs5)
	}
}

// TestEventsForSHAPrefixesOldestFirst verifies results are ordered by created ASC.
func TestEventsForSHAPrefixesOldestFirst(t *testing.T) {
	s := openStore(t)
	const sha = "deadbeef"
	if err := s.InsertEvent(sha, "deploy", "ok", "", "2026-06-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertEvent(sha, "deploy", "failed", "", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	evs, err := s.EventsForSHAPrefixes([]string{sha})
	if err != nil || len(evs) != 2 {
		t.Fatalf("got %d events, err=%v", len(evs), err)
	}
	// Oldest first: 2026-01-01 before 2026-06-01.
	if !strings.HasPrefix(evs[0].Created, "2026-01") || !strings.HasPrefix(evs[1].Created, "2026-06") {
		t.Fatalf("order wrong: %+v", evs)
	}
}

func TestLoreLinksUpsertAndLoad(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "lore.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rec := lore.Record{ID: "itm-A", Type: lore.TypeItem, Title: "A",
		Related: []string{"dec-B", "some-slug"}}
	if err := st.Upsert(rec, "repo", "items/a.md"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := st.Get("itm-A")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if len(got.Related) != 2 || got.Related[0] != "dec-B" || got.Related[1] != "some-slug" {
		t.Fatalf("Related = %v", got.Related)
	}
	// Re-upsert with fewer links replaces, not appends.
	rec.Related = []string{"dec-B"}
	if err := st.Upsert(rec, "repo", "items/a.md"); err != nil {
		t.Fatal(err)
	}
	got, _, _ = st.Get("itm-A")
	if len(got.Related) != 1 {
		t.Fatalf("after re-upsert Related = %v", got.Related)
	}
}

// TestEventsForSHAPrefixesEmptySHANeverMatches verifies that events with
// empty sha="" are never returned by EventsForSHAPrefixes, even if they exist
// in the database. They are valid for storage (CI evidence) but not queryable.
func TestEventsForSHAPrefixesEmptySHANeverMatches(t *testing.T) {
	s := openStore(t)
	// Insert an event with empty SHA (as loreEvent would do when no commit is available).
	if err := s.InsertEvent("", "ci", "ok", "no commit info", "2026-07-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	// Insert a normal event with a real SHA.
	const realSHA = "abc123def456"
	if err := s.InsertEvent(realSHA, "deploy", "ok", "", "2026-07-02T00:00:00Z"); err != nil {
		t.Fatal(err)
	}

	// Query with a prefix that matches the real event should only return it.
	evs, err := s.EventsForSHAPrefixes([]string{"abc"})
	if err != nil {
		t.Fatalf("EventsForSHAPrefixes: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("want 1 event (empty-SHA filtered out), got %d: %+v", len(evs), evs)
	}
	if evs[0].SHA != realSHA {
		t.Fatalf("got event with wrong SHA: %q (want %q)", evs[0].SHA, realSHA)
	}

	// Query with "x" prefix that matches neither should return nothing.
	evs2, err := s.EventsForSHAPrefixes([]string{"x"})
	if err != nil {
		t.Fatalf("EventsForSHAPrefixes with non-match: %v", err)
	}
	if len(evs2) != 0 {
		t.Fatalf("non-matching query should return 0 events, got %d: %+v", len(evs2), evs2)
	}

	// Query with full realSHA should return it.
	evs3, err := s.EventsForSHAPrefixes([]string{realSHA})
	if err != nil {
		t.Fatalf("EventsForSHAPrefixes full SHA: %v", err)
	}
	if len(evs3) != 1 {
		t.Fatalf("full SHA query: want 1, got %d: %+v", len(evs3), evs3)
	}
}
