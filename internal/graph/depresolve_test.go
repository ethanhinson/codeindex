package graph

import (
	"path/filepath"
	"testing"
)

// resolvedDepEdge reads back the single edge of kind k emitted from srcFile,
// joined through to the symbol it actually resolved to. The resolved TARGET is
// the thing under test — dst_ns movement on its own counts for nothing — so
// this reader deliberately returns the destination symbol's file and namespace
// rather than the hint. DumpNormalized selects neither, hence the direct read.
func resolvedDepEdge(t *testing.T, st *Store, srcFile, kind string) (dstFile, dstNamespace, confidence, hint string) {
	t.Helper()
	rows, err := st.db.Query(
		`SELECT e.confidence, e.dst_ns, IFNULL(s.file,''), IFNULL(s.namespace,'')
		   FROM edges e LEFT JOIN symbols s ON s.id = e.dst_symbol_id
		  WHERE e.src_file=? AND e.kind=?`, srcFile, kind)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var n int
	for rows.Next() {
		if err := rows.Scan(&confidence, &hint, &dstFile, &dstNamespace); err != nil {
			t.Fatal(err)
		}
		n++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("want exactly one %s edge from %s, got %d", kind, srcFile, n)
	}
	return dstFile, dstNamespace, confidence, hint
}

// putChunkCandidates inserts the two same-name decoys the disambiguation turns
// on: `Chunk` in tsdb/chunkenc and `Chunk` in tsdb/chunks. Both are tier-0 Go
// symbols whose namespace DeriveNamespace takes from their repo-relative
// directory, which is exactly why an import PATH hint can never equal one and
// must match by nsMatch's suffix comparison.
//
// Ordering matters to the negative half: the plain tier-0 rung orders by file,
// so with no hint the ambiguous winner is deterministically the chunkenc one.
// The fixture therefore makes tsdb/chunks the CORRECT answer, so a hint that
// does nothing cannot accidentally land on the right symbol.
func putChunkCandidates(t *testing.T, st *Store) {
	t.Helper()
	putFile(t, st, &ParsedFile{
		Path: "tsdb/chunkenc/chunk.go",
		Symbols: []Symbol{
			{File: "tsdb/chunkenc/chunk.go", Name: "Chunk", Kind: KindType, StartLine: 1, EndLine: 3},
		},
	})
	putFile(t, st, &ParsedFile{
		Path: "tsdb/chunks/chunk.go",
		Symbols: []Symbol{
			{File: "tsdb/chunks/chunk.go", Name: "Chunk", Kind: KindType, StartLine: 1, EndLine: 3},
		},
	})
}

const chunksImportPath = "github.com/prometheus/prometheus/tsdb/chunks"

// embedder builds the importing file: a Go file in a third package that
// imports one of the two candidates and embeds `Chunk`. The KindImports dep
// mirrors what the Go adapter emits — Target is the import PATH, not the type
// name — so it deliberately puts nothing into the file-level `bind` map under
// the key "Chunk". The extends dep's own Source is therefore the only channel
// through which a hint can reach the resolution ladder, which is what makes
// the two halves below differ in exactly one field.
func embedder(path, source string) *ParsedFile {
	return &ParsedFile{
		Path: path,
		Symbols: []Symbol{
			{File: path, Name: "Series", Kind: KindType, StartLine: 3, EndLine: 6},
		},
		Deps: []RawDep{
			{EnclosingIdx: -1, Kind: KindImports, Target: chunksImportPath, Source: chunksImportPath, Line: 1},
			{EnclosingIdx: 0, Kind: KindExtends, Target: "Chunk", Source: source, Line: 4},
		},
	}
}

// TestDepHintDisambiguatesSameNameTargets is the disambiguation bar for change
// 0017, and the only in-suite assertion that a namespace hint on a subtype edge
// changes WHICH symbol that edge points at. There are no committed graph
// goldens, and every DumpNormalized consumer is a rebuild-equivalence check
// under a single code version, so nothing else in the suite can observe a
// resolution regression here.
//
// Positive: `Chunk` is ambiguous by name across tsdb/chunkenc and tsdb/chunks;
// an extends edge carrying Source=".../tsdb/chunks" must resolve to the
// tsdb/chunks symbol, unambiguously.
//
// Negative (same shape, one field): drop the Source and the edge falls back
// through the plain tier-0 rung, staying ambiguous and — because the fixture is
// ordered so the correct answer sorts second — landing on the WRONG symbol.
// The negative is thus not vacuous: it fails differently in target as well as
// in confidence, so the positive cannot be passing by accident.
func TestDepHintDisambiguatesSameNameTargets(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	putChunkCandidates(t, st)

	putFile(t, st, embedder("scrape/hinted.go", chunksImportPath))
	dstFile, dstNS, conf, hint := resolvedDepEdge(t, st, "scrape/hinted.go", string(KindExtends))
	if dstFile != "tsdb/chunks/chunk.go" {
		t.Errorf("hinted embed must resolve to the imported Chunk: dst file = %q, want %q",
			dstFile, "tsdb/chunks/chunk.go")
	}
	if dstNS != "tsdb/chunks" {
		t.Errorf("hinted embed resolved into namespace %q, want %q", dstNS, "tsdb/chunks")
	}
	if conf != string(ConfUnambiguous) {
		t.Errorf("hinted embed confidence = %q, want %q", conf, ConfUnambiguous)
	}
	// The hint is an import path and the namespace is a repo-relative
	// directory: they are never equal, and the match is nsMatch's suffix leg.
	if hint != chunksImportPath {
		t.Errorf("persisted hint = %q, want the raw import path %q", hint, chunksImportPath)
	}
	if hint == dstNS {
		t.Fatalf("fixture is not exercising the suffix match: hint %q equals namespace %q", hint, dstNS)
	}

	// Same setup, Source removed. Nothing else changes.
	putFile(t, st, embedder("scrape/unhinted.go", ""))
	dstFile, _, conf, hint = resolvedDepEdge(t, st, "scrape/unhinted.go", string(KindExtends))
	if conf != string(ConfAmbiguous) {
		t.Errorf("unhinted embed confidence = %q, want %q", conf, ConfAmbiguous)
	}
	if hint != "" {
		t.Errorf("unhinted embed must carry no hint, got %q", hint)
	}
	if dstFile != "tsdb/chunkenc/chunk.go" {
		t.Errorf("unhinted embed should fall to the first name match, got %q, want %q",
			dstFile, "tsdb/chunkenc/chunk.go")
	}
}
