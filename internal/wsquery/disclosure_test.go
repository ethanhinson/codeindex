package wsquery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeindex/internal/overlay"
	"codeindex/internal/query"
)

// Disclosure tests for the three answer-layer gaps §3.8/§5/§4.2 leave open:
// an ambiguous re-map's CANDIDATES, an unresolved row's OWNING MEMBER, and a
// schema-mismatched overlay read on the query path.
//
// They share one fixture rather than the three-member unionFixture, because
// each needs a shape that fixture deliberately does not have: two symbols in
// ONE member sharing a qualified name (so a stable key inverts ambiguously), a
// call with no resolvable definition, and a find corpus where two members both
// overflow the limit. Bolting those onto unionFixture would move the hand-
// computed expectations twenty other tests are written against.

// disclosureFixture writes a two-member workspace and builds both indexes.
//
// web carries TWO top-level `Shared` functions in two sibling packages, so
// ProjectDefs("Shared", "") returns both and the stable key
// {web, pkga/shared.go, Shared} inverts AMBIGUOUSLY. lib's Target calls
// fmt.Println, which no member defines, so its callee row is unresolved.
func disclosureFixture(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".codeindex"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "version": 1,
  "members": [
    {"id": "web", "root": "services/web", "namespaces": ["App\\Web"]},
    {"id": "lib", "root": "services/lib", "namespaces": ["App\\Lib"]}
  ]
}
`
	if err := os.WriteFile(filepath.Join(ws, ".codeindex", "workspace.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"services/web/pkga/shared.go": `package pkga

func Shared() {}

func AlphaOne() {}

func AlphaTwo() {}
`,
		"services/web/pkgb/shared.go": `package pkgb

func Shared() {}
`,
		"services/lib/target.go": `package lib

import "fmt"

func Target() {
	fmt.Println("x")
}

func AlphaThree() {}

func AlphaFour() {}
`,
	}
	for name, content := range files {
		p := filepath.Join(ws, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range []string{"web", "lib"} {
		if _, err := query.Fresh(filepath.Join(ws, "services", m)); err != nil {
			t.Fatalf("building member %s: %v", m, err)
		}
	}

	ov, err := overlay.Open(overlay.Path(ws))
	if err != nil {
		t.Fatal(err)
	}
	// One inbound cross-edge whose SOURCE key is the ambiguous one.
	if err := ov.PutCrossEdges([]overlay.CrossEdge{{
		Src:  overlay.SymKey{Member: "web", File: "pkga/shared.go", QName: "Shared"},
		Dst:  overlay.SymKey{Member: "lib", File: "target.go", QName: "Target"},
		Kind: "calls", Provenance: "cross_repo_import", Confidence: ConfidenceExact, Line: 3,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := ov.Close(); err != nil {
		t.Fatal(err)
	}
	return ws
}

// TestAmbiguousCrossRefEmitsEveryCandidateAtTheANSWERLayer is §3.8's
// "rendered as ambiguous WITH ITS CANDIDATES", asserted where the reader meets
// it rather than on RemapKey alone.
//
// The load-bearing property is twofold. Never manufacturing an exact-looking
// answer: every emitted row is flagged Ambiguous. And actually DISCLOSING the
// candidates: one row per candidate, each at ITS OWN resolved path — never a
// single row bearing the key's recorded path, which is the write-time datum the
// stable key exists because it may have moved.
func TestAmbiguousCrossRefEmitsEveryCandidateAtTheANSWERLayer(t *testing.T) {
	defer cleanFreshen(t)()
	ws := disclosureFixture(t)

	a, err := Callers(ws, "Target", 100)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range a.Callers {
		if c.Repo != "web" {
			continue
		}
		if !c.Ambiguous {
			t.Errorf("cross row %s:%d is not flagged ambiguous; an ambiguous key must never render as an exact answer", c.File, c.Line)
		}
		got = append(got, fmt.Sprintf("%s|%s:%d|%s", c.Repo, c.File, c.Line, c.QName))
	}
	want := []string{
		"web|services/web/pkga/shared.go:3|Shared",
		"web|services/web/pkgb/shared.go:3|Shared",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("ambiguous cross rows =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if a.CallersTotal != len(a.Callers) {
		t.Errorf("CallersTotal = %d, want %d: the total must count the rows the expansion produced",
			a.CallersTotal, len(a.Callers))
	}
}

// TestUnresolvedCalleeRowNamesItsMember reads §5's rule — the member id
// prefixes the path only when Repo is non-empty — at the row that has NO path. An unresolved
// callee — stdlib or external, the common case — must still say which member's
// code made the call, because a bare anchor merges every member's rows into one
// list.
func TestUnresolvedCalleeRowNamesItsMember(t *testing.T) {
	defer cleanFreshen(t)()
	ws := disclosureFixture(t)

	text, err := CalleesText(ws, "Target", 100)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "-> lib: unresolved") {
		t.Errorf("callees text does not attribute the unresolved row to a member:\n%s", text)
	}
}

// TestOverlaySchemaMismatchIsReadEmptyAndDISCLOSEDNeverRebuilt is finding #10's
// cross-site asymmetry, closed. WorkspaceStatus version-gates before
// overlay.Open because Open DELETES AND RECREATES a mismatched file; the
// read-only query path must not do on §4.2's degrade path what the diagnostic
// refuses to do.
//
// Two halves, and the second is the one that matters: the overlay is left
// alone, AND the answer says so. A silently empty overlay is an answer missing
// every cross-member edge with nothing disclosing it — the D7 hard fail.
func TestOverlaySchemaMismatchIsReadEmptyAndDISCLOSEDNeverRebuilt(t *testing.T) {
	defer cleanFreshen(t)()
	ws := disclosureFixture(t)
	ovPath := overlay.Path(ws)

	forgeOverlayVersion(t, ovPath, overlay.SchemaVersion()+1)
	before, err := os.ReadFile(ovPath)
	if err != nil {
		t.Fatal(err)
	}

	text, err := CallersText(ws, "Target", 100)
	if err != nil {
		t.Fatalf("a schema-mismatched overlay must degrade the query, not fail it: %v", err)
	}
	if strings.Contains(text, "pkga/shared.go") {
		t.Errorf("cross rows survived an unreadable overlay:\n%s", text)
	}
	if !strings.Contains(text, "overlay_unreadable: overlay schema v") {
		t.Errorf("the clause does not disclose the unreadable overlay — silent staleness:\n%s", text)
	}

	after, err := os.ReadFile(ovPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("the query REWROTE the schema-mismatched overlay; a read-only path must never rebuild it")
	}
	if got, err := overlay.FileSchemaVersion(ovPath); err != nil || got != overlay.SchemaVersion()+1 {
		t.Errorf("overlay version = %d (err %v), want the forged version left untouched", got, err)
	}
}

// TestOverlaySchemaGuardRedensWhenTheGateIsRemoved is the mutation evidence for
// the guard above: with the version gate defeated — i.e. opening the mismatched
// overlay the way openUnion used to — the file is destroyed and recreated
// empty, which is exactly what the guard exists to prevent.
func TestOverlaySchemaGuardRedensWhenTheGateIsRemoved(t *testing.T) {
	ws := disclosureFixture(t)
	ovPath := overlay.Path(ws)
	forgeOverlayVersion(t, ovPath, overlay.SchemaVersion()+1)
	before, err := os.ReadFile(ovPath)
	if err != nil {
		t.Fatal(err)
	}
	ov, err := overlay.Open(ovPath) // the ungated call
	if err != nil {
		t.Fatal(err)
	}
	ov.Close()
	after, err := os.ReadFile(ovPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) == string(after) {
		t.Fatal("overlay.Open no longer rebuilds a mismatched overlay: the version gate in openUnion is guarding nothing, and this test's premise is stale")
	}
}

// forgeOverlayVersion stamps a wrong user_version onto an existing overlay.
// OpenRaw plus a PRAGMA write is the only way to do it: overlay.Open is the
// very function that refuses to leave a mismatch in place.
func forgeOverlayVersion(t *testing.T, path string, version int) {
	t.Helper()
	db, err := overlay.OpenRaw(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, version)); err != nil {
		t.Fatal(err)
	}
}

// TestBYDESIGNFindTotalIsTheSumOfPerMemberCappedTotals characterizes finding
// #11: find's Total is Σ min(n_i, limit), so with TWO members overflowing a
// limit of 1 the answer prints a total of 2 above a single row — neither the
// corpus total (4 Alpha* symbols) nor the row count.
//
// This asserts what the code DOES and must not be read as endorsing the number.
// The behaviour is forced: per-member `unlimited` would move a one-member
// workspace's Total off the single-repo answer's (§7.4's byte-equality bar) and
// recounting rows would contradict §3.2. See findWorkspace's doc comment.
func TestBYDESIGNFindTotalIsTheSumOfPerMemberCappedTotals(t *testing.T) {
	defer cleanFreshen(t)()
	ws := disclosureFixture(t)

	// Both members hold two Alpha* symbols, so both overflow limit 1.
	for _, m := range []string{"web", "lib"} {
		inner, err := query.Find(filepath.Join(ws, "services", m), "Alpha", "", "", 100)
		if err != nil {
			t.Fatal(err)
		}
		if inner.Total < 2 {
			t.Fatalf("member %s matched %d Alpha symbols, want at least 2: the fixture no longer overflows the limit",
				m, inner.Total)
		}
	}

	a, err := Find(ws, "Alpha", "", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Results) != 1 {
		t.Fatalf("rows = %d, want the limit (1)", len(a.Results))
	}
	if a.Total != 2 {
		t.Errorf("Total = %d, want 2 — the SUM of the two per-member CAPPED totals, not the corpus total and not the row count", a.Total)
	}
}
