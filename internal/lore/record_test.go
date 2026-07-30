package lore

import (
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
