package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"codeindex/internal/engine"
	"codeindex/internal/graph"
)

// fixture: two Go files with NO static call between them — the runtime
// evidence is the only connection (the hook-dispatch shape).
const fileA = `package p

func StartFlow() int {
	return 1
}
`

const fileB = `package p

func HandleHook() int {
	return 2
}
`

func fixtureIndex(t *testing.T) (root string, st *graph.Store) {
	t.Helper()
	root = t.TempDir()
	for name, content := range map[string]string{"a.go": fileA, "b.go": fileB} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	db := filepath.Join(root, ".codeindex", "graph.db")
	if err := os.MkdirAll(filepath.Dir(db), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Build(root, db); err != nil {
		t.Fatal(err)
	}
	st, err := graph.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return root, st
}

const profile = `{"cxprof":1,"lang":"go","unit":"samples","hz":99,"start":100,"end":160,"commit":"abc","tag":"dev"}
{"st":[["a.go",4],["/opt/vendor/dispatch.php",99],["b.go",4]],"n":7}
{"st":[["a.go",4]],"n":3}
{"st":[["not-json`

func TestIngestHookDispatchShape(t *testing.T) {
	root, st := fixtureIndex(t)
	dir := SpoolDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	spool := filepath.Join(dir, "100-1.cxprof.jsonl")
	if err := os.WriteFile(spool, []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}

	stats, err := Ingest(st, root, spool)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if stats.Skipped != 1 {
		t.Fatalf("Skipped = %d, want 1 (truncated line tolerated)", stats.Skipped)
	}
	if stats.Samples != 10 {
		t.Fatalf("Samples = %d, want 10", stats.Samples)
	}
	// 3 frames total resolvable-attempted: a.go x2 + b.go (vendor path is
	// outside the repo and doesn't count as a frame attempt? It does count:
	// FramesTotal counts every well-formed frame).
	if stats.FramesTotal != 4 || stats.FramesResolved != 3 {
		t.Fatalf("frames = %d/%d, want 3/4 resolved", stats.FramesResolved, stats.FramesTotal)
	}

	edges, err := st.ObsEdges()
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 1 {
		t.Fatalf("obs edges = %d, want 1", len(edges))
	}
	e := edges[0]
	if e.SrcKey != graph.ObsKey("a.go", "", "StartFlow") ||
		e.DstKey != graph.ObsKey("b.go", "", "HandleHook") {
		t.Fatalf("edge %s -> %s", e.SrcKey, e.DstKey)
	}
	if !e.Indirect {
		t.Fatal("edge through unresolvable vendor frame must be flagged indirect")
	}
	if e.Weight != 7 {
		t.Fatalf("weight = %d, want 7", e.Weight)
	}

	heat, err := st.ObsHeatByKey()
	if err != nil {
		t.Fatal(err)
	}
	start := heat[graph.ObsKey("a.go", "", "StartFlow")]
	hook := heat[graph.ObsKey("b.go", "", "HandleHook")]
	if start.Total != 10 || start.Entry != 10 || start.Leaf != 3 {
		t.Fatalf("StartFlow heat = %+v", start)
	}
	if hook.Total != 7 || hook.Leaf != 7 || hook.Entry != 0 {
		t.Fatalf("HandleHook heat = %+v", hook)
	}

	// Idempotence: same file, same hash -> no double counting.
	if _, err := Ingest(st, root, spool); err != nil {
		t.Fatal(err)
	}
	edges, _ = st.ObsEdges()
	if len(edges) != 1 || edges[0].Weight != 7 {
		t.Fatalf("re-ingest changed evidence: %+v", edges)
	}

	// Spool sweep picks up nothing new either.
	out, err := IngestSpool(st, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("spool sweep re-ingested: %+v", out)
	}
}

func TestParseRejectsNonProfiles(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "x.cxprof.jsonl")
	os.WriteFile(bad, []byte(`{"not":"cxprof"}`), 0o644)
	if _, err := Parse(bad); err == nil {
		t.Fatal("want header error")
	}
	os.WriteFile(bad, []byte(`{"cxprof":2,"lang":"go","unit":"samples","hz":9,"start":1,"end":2}`), 0o644)
	if _, err := Parse(bad); err == nil {
		t.Fatal("want version error")
	}
}
