package search

import (
	"os"
	"path/filepath"
	"testing"

	"codeindex/internal/graph"
)

func TestDistinctiveWords(t *testing.T) {
	got := distinctiveWords("Can't bind the list of struct pointer in query", 2)
	if len(got) != 2 || got[0] != "pointer" || got[1] != "struct" {
		t.Fatalf("words = %v", got)
	}
	if w := distinctiveWords("is it the a of", 2); len(w) != 0 {
		t.Fatalf("stopword-only query yielded %v", w)
	}
}

func TestQuotedPhrase(t *testing.T) {
	if p := quotedPhrase(`logger prints "headers already written" twice`); p != "headers already written" {
		t.Fatalf("phrase = %q", p)
	}
	if p := quotedPhrase("no quotes here"); p != "" {
		t.Fatalf("phrase = %q", p)
	}
}

// literalFixture builds a real working tree + matching index: the symptom
// string lives inside PayCharge's span; util.go is the decoy.
func literalFixture(t *testing.T) (string, *graph.Store) {
	t.Helper()
	root := t.TempDir()
	pay := `package p

func PayCharge() error {
	// retries the gateway
	return errors.New("payment gateway timeout exceeded")
}
`
	util := `package p

func UtilThing() int {
	return 42
}
`
	os.MkdirAll(filepath.Join(root, "src"), 0o755)
	os.WriteFile(filepath.Join(root, "src", "pay.go"), []byte(pay), 0o644)
	os.WriteFile(filepath.Join(root, "src", "util.go"), []byte(util), 0o644)

	st, err := graph.Open(filepath.Join(root, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	tx, err := st.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []graph.Symbol{
		{File: "src/pay.go", Name: "PayCharge", Kind: graph.KindFunc, StartLine: 3, EndLine: 6},
		{File: "src/util.go", Name: "UtilThing", Kind: graph.KindFunc, StartLine: 3, EndLine: 5},
	} {
		pf := &graph.ParsedFile{Path: s.File, Symbols: []graph.Symbol{s}}
		if _, _, err := st.PutFile(tx, pf, graph.FileMeta{Path: s.File, Hash: s.File, Size: 1, Mtime: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return root, st
}

func TestLiteralLaneSymptomMatch(t *testing.T) {
	root, st := literalFixture(t)

	t.Setenv("CODEINDEX_LITERAL_LANE", "1")
	lane := buildLiteralLane(st, root, "payment gateway timeout on checkout", "", 2)
	if len(lane.rank) == 0 {
		t.Fatal("lane inactive")
	}
	// PayCharge must be lane rank 0 (both words co-occur in its span).
	syms, _, _ := st.AllSymbolsWithCallers()
	var payID int64
	for _, s := range syms {
		if s.Name == "PayCharge" {
			payID = s.ID
		}
	}
	if r, ok := lane.rank[payID]; !ok || r != 0 {
		t.Fatalf("PayCharge lane rank = %d (ok=%v)", r, ok)
	}
	if lane.conf <= 1.0 {
		t.Fatalf("co-occurrence should raise conf, got %f", lane.conf)
	}
	// Full 3-content-word query counts as a phrase — but only pins if it
	// occurs verbatim; "payment gateway timeout on checkout" does not.
	for _, id := range lane.pins {
		if id == payID {
			t.Fatal("non-verbatim phrase must not pin")
		}
	}

	// Quoted verbatim string pins.
	lane = buildLiteralLane(st, root, `error says "payment gateway timeout exceeded"`, "", 2)
	pinned := false
	for _, id := range lane.pins {
		if id == payID {
			pinned = true
		}
	}
	if !pinned {
		t.Fatal("verbatim quoted phrase did not pin PayCharge")
	}

	// error_text path pins too.
	lane = buildLiteralLane(st, root, "weird checkout bug", "payment gateway timeout exceeded", 2)
	pinned = false
	for _, id := range lane.pins {
		if id == payID {
			pinned = true
		}
	}
	if !pinned {
		t.Fatal("error_text did not pin PayCharge")
	}
}

func TestLiteralLaneDisabledWithoutRoot(t *testing.T) {
	_, st := literalFixture(t)
	lane := buildLiteralLane(st, "", "payment gateway timeout", "", 2)
	if len(lane.rank) != 0 || len(lane.pins) != 0 {
		t.Fatal("lane must be inactive without a root")
	}
}
