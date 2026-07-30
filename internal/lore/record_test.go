package lore

import (
	"reflect"
	"strings"
	"testing"
)

func TestRecordRoundTrip(t *testing.T) {
	in := Record{
		ID: "dec-01AN4Z07BY79KA1307SR9X4MV3", Type: TypeDecision,
		Title: "Use Go for the engine", Status: "active", Date: "2026-07-29",
		Anchors: []Anchor{{Path: "internal/engine/"}, {Symbol: "ResolveImports"}},
		Refs:    []Ref{{Kind: "gh-issue", Value: "ethanhinson/codeindex#12"}},
		Body:    "Rationale.\n\n## Alternatives considered\nRust — rejected.\n",
	}
	b, err := in.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), "---\n") {
		t.Fatalf("no frontmatter delimiter:\n%s", b)
	}
	out, err := Parse(b, TypeDecision)
	if err != nil {
		t.Fatal(err)
	}
	if out.ID != in.ID || out.Title != in.Title || out.Status != in.Status ||
		len(out.Anchors) != 2 || out.Anchors[0].Path != "internal/engine/" ||
		out.Anchors[1].Symbol != "ResolveImports" ||
		len(out.Refs) != 1 || out.Refs[0].Kind != "gh-issue" ||
		out.Body != in.Body {
		t.Fatalf("round-trip mismatch:\n%+v\nvs\n%+v", out, in)
	}
}

func TestParseItemFields(t *testing.T) {
	src := `---
id: itm-01AN4Z07BY79KA1307SR9X4MV4
title: Migrate resolver tests
status: open
date: 2026-07-29
priority: p1
blocked_by: [itm-01AN4Z07BY79KA1307SR9X4MV5]
tags: [tech-debt]
---
Body here.
`
	r, err := Parse([]byte(src), TypeItem)
	if err != nil {
		t.Fatal(err)
	}
	if r.Priority != "p1" || len(r.BlockedBy) != 1 || r.Tags[0] != "tech-debt" {
		t.Fatalf("item fields: %+v", r)
	}
}

func TestParseErrors(t *testing.T) {
	if _, err := Parse([]byte("no frontmatter"), TypeNote); err == nil {
		t.Fatal("want error for missing frontmatter")
	}
	if _, err := Parse([]byte("---\nid: [broken\n---\n"), TypeNote); err == nil {
		t.Fatal("want error for bad yaml")
	}
}

func TestNewIDAndSlug(t *testing.T) {
	if id := NewID(TypeItem); !strings.HasPrefix(id, "itm-") || len(id) != 4+26 {
		t.Fatalf("bad id %q", id)
	}
	if s := Slug("Use Go: for the engine!"); s != "use-go-for-the-engine" {
		t.Fatalf("slug %q", s)
	}
}

func TestRoundTripBodyWithLeadingNewline(t *testing.T) {
	in := Record{ID: "note-01AN4Z07BY79KA1307SR9X4MV9", Type: TypeNote,
		Title: "N", Date: "2026-07-29", Body: "\nleading blank line kept\n"}
	b, err := in.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	out, err := Parse(b, TypeNote)
	if err != nil {
		t.Fatal(err)
	}
	if out.Body != in.Body {
		t.Fatalf("body round-trip: %q != %q", out.Body, in.Body)
	}
}

// TestExtraFieldsRoundTrip verifies that unknown frontmatter keys survive
// Parse → Marshal → Parse without disturbing known fields.
func TestExtraFieldsRoundTrip(t *testing.T) {
	src := "---\nid: dec-01AN4Z07BY79KA1307SR9X4MV3\ntitle: Test extra fields\nstatus: active\ndate: 2026-07-29\nhook: \"future field\"\nclaimed_at: 2026-08-01T00:00:00Z\n---\nBody.\n"
	r, err := Parse([]byte(src), TypeDecision)
	if err != nil {
		t.Fatal(err)
	}

	// Extra map must capture the unknown keys.
	if r.Extra == nil {
		t.Fatal("Extra is nil, expected map with unknown keys")
	}
	if got, ok := r.Extra["hook"]; !ok || got != "future field" {
		t.Fatalf("Extra[hook] = %v, want 'future field'", got)
	}
	if _, ok := r.Extra["claimed_at"]; !ok {
		t.Fatal("Extra[claimed_at] missing")
	}

	// Known fields must NOT appear in Extra.
	knownInExtra := []string{"id", "title", "status", "date", "supersedes", "superseded_by",
		"priority", "blocked_by", "tags", "anchors", "refs"}
	for _, k := range knownInExtra {
		if _, found := r.Extra[k]; found {
			t.Fatalf("known key %q must not appear in Extra", k)
		}
	}

	// Marshal then Parse again — both keys must survive byte-level and in Extra.
	b, err := r.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	marshaled := string(b)
	if !strings.Contains(marshaled, "hook:") {
		t.Fatalf("hook: not found in marshaled output:\n%s", marshaled)
	}
	if !strings.Contains(marshaled, "claimed_at:") {
		t.Fatalf("claimed_at: not found in marshaled output:\n%s", marshaled)
	}

	r2, err := Parse(b, TypeDecision)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Extra == nil {
		t.Fatal("Extra nil after second parse")
	}
	if got, ok := r2.Extra["hook"]; !ok || got != "future field" {
		t.Fatalf("hook after second parse = %v, want 'future field'", got)
	}
	if _, ok := r2.Extra["claimed_at"]; !ok {
		t.Fatal("claimed_at missing after second parse")
	}

	// Known fields must be unchanged.
	if r2.ID != "dec-01AN4Z07BY79KA1307SR9X4MV3" || r2.Title != "Test extra fields" ||
		r2.Status != "active" || r2.Body != "Body.\n" {
		t.Fatalf("known fields corrupted: %+v", r2)
	}
}

// TestNoExtraForKnownKeys verifies that a record with only known frontmatter
// keys produces a nil Extra (no pollution).
func TestNoExtraForKnownKeys(t *testing.T) {
	in := Record{
		ID: "dec-01AN4Z07BY79KA1307SR9X4MV3", Type: TypeDecision,
		Title: "Clean record", Status: "active", Date: "2026-07-29",
		Body: "Rationale.\n",
	}
	b, err := in.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	out, err := Parse(b, TypeDecision)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Extra) != 0 {
		t.Fatalf("expected nil/empty Extra for known-only fields, got %v", out.Extra)
	}
}

// TestKnownKeysCoversWireFields asserts knownKeys length equals wire struct
// field count so a future field can't be forgotten.
func TestKnownKeysCoversWireFields(t *testing.T) {
	wireType := reflect.TypeOf(wire{})
	if len(knownKeys) != wireType.NumField() {
		t.Fatalf("knownKeys has %d entries but wire has %d fields — update knownKeys",
			len(knownKeys), wireType.NumField())
	}
}
