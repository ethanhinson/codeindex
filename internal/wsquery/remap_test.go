package wsquery

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"codeindex/internal/graph"
	"codeindex/internal/overlay"
	"codeindex/internal/wsfresh"
)

func marshalClause(t *testing.T, c Clause) string {
	t.Helper()
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// --- fixtures --------------------------------------------------------------

// symdef is one tier-0 definition to plant in a fixture member index.
type symdef struct {
	file   string
	name   string
	parent string
	line   int
}

// defsStore builds a small hand-made member index. graph.Open is the BUILD
// path and is legitimate in test code.
func defsStore(t *testing.T, defs ...symdef) *graph.Store {
	t.Helper()
	st, err := graph.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	byFile := map[string][]graph.Symbol{}
	var order []string
	for _, d := range defs {
		line := d.line
		if line == 0 {
			line = 1
		}
		if _, seen := byFile[d.file]; !seen {
			order = append(order, d.file)
		}
		byFile[d.file] = append(byFile[d.file], graph.Symbol{
			File: d.file, Name: d.name, Parent: d.parent,
			Kind: graph.KindFunc, StartLine: line, EndLine: line + 1,
		})
	}
	for _, f := range order {
		tx, err := st.Begin()
		if err != nil {
			t.Fatal(err)
		}
		pf := &graph.ParsedFile{Path: f, Symbols: byFile[f]}
		if _, _, err := st.PutFile(tx, pf, graph.FileMeta{Path: f, Hash: "h", Size: 1, Mtime: 1}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

func key(qname string) overlay.SymKey {
	return overlay.SymKey{Member: "m", File: "does/not/matter.go", QName: qname}
}

func mustRemap(t *testing.T, r DefsReader, k overlay.SymKey) Remap {
	t.Helper()
	got, err := RemapKey(r, k)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// --- the split -------------------------------------------------------------

// QName() is Parent + "." + Name, so the inverse splits on the LAST dot.
// Dotted parents are ordinary in TS and Python; a first-dot split mis-parents
// every nested symbol in two of the four supported languages.
func TestQNameSplitsOnTheLastDot(t *testing.T) {
	cases := []struct{ qname, parent, name string }{
		{"Handle", "", "Handle"},
		{"Server.Handle", "Server", "Handle"},
		{"outer.Inner.method", "outer.Inner", "method"},
		{"a.b.c.d", "a.b.c", "d"},
	}
	for _, c := range cases {
		parent, name := splitQName(c.qname)
		if parent != c.parent || name != c.name {
			t.Errorf("splitQName(%q) = (%q, %q), want (%q, %q)",
				c.qname, parent, name, c.parent, c.name)
		}
	}
}

// The split against a DOTTED-PARENT fixture. A single-dot fixture cannot catch
// a first-dot split: this one can, because a first-dot split asks the member DB
// for name "Inner.method" under parent "outer", which no row has.
func TestRemapInvertsADottedParentKey(t *testing.T) {
	st := defsStore(t, symdef{"ts/x.ts", "method", "outer.Inner", 12})

	got := mustRemap(t, st, key("outer.Inner.method"))
	if got.Status != RemapExact {
		t.Fatalf("status = %v, want RemapExact (a first-dot split would find nothing)", got.Status)
	}
	if got.Symbol.Name != "method" || got.Symbol.Parent != "outer.Inner" {
		t.Fatalf("symbol = %+v, want the nested TS/Python symbol", got.Symbol)
	}
}

// --- the three cardinalities ----------------------------------------------

func TestRemapExactlyOneSymbolIsUsed(t *testing.T) {
	st := defsStore(t, symdef{"a.go", "Handle", "", 3})

	got := mustRemap(t, st, key("Handle"))
	if got.Status != RemapExact {
		t.Fatalf("status = %v, want RemapExact", got.Status)
	}
	if got.Symbol.File != "a.go" || got.Symbol.StartLine != 3 {
		t.Fatalf("symbol = %+v, want a.go:3", got.Symbol)
	}
}

// Zero symbols — the member was rebuilt and the symbol is gone. The reference
// is DROPPED and COUNTED, never silently swallowed.
func TestRemapZeroSymbolsIsUnmappedNotAGuess(t *testing.T) {
	st := defsStore(t, symdef{"a.go", "Something", "", 1})

	got := mustRemap(t, st, key("Handle"))
	if got.Status != RemapUnmapped {
		t.Fatalf("status = %v, want RemapUnmapped", got.Status)
	}
	if got.Symbol != (graph.Symbol{}) || len(got.Candidates) != 0 {
		t.Fatalf("an unmapped key must carry no symbol and no candidates: %+v", got)
	}
}

// Many, narrowed by the EXACT-parent filter to one. ProjectDefs treats
// parent == "" as no parent restriction, so a dotless key legitimately comes
// back with the top-level symbol AND every method of that name.
func TestRemapManyNarrowsOnTheExactParent(t *testing.T) {
	st := defsStore(t,
		symdef{"a.go", "Handle", "", 3},
		symdef{"b.go", "Handle", "Server", 9},
	)

	got := mustRemap(t, st, key("Handle"))
	if got.Status != RemapExact {
		t.Fatalf("status = %v, want RemapExact after the exact-parent filter", got.Status)
	}
	if got.Symbol.Parent != "" || got.Symbol.File != "a.go" {
		t.Fatalf("symbol = %+v, want the top-level a.go definition", got.Symbol)
	}
}

// Several survive the exact-parent filter: the reference is AMBIGUOUS with its
// candidates. Picking the first row would manufacture an exact-looking answer
// out of a genuinely ambiguous key — the one thing D3 forbids end to end.
func TestRemapManySurvivorsAreAmbiguousNeverTheFirstRow(t *testing.T) {
	st := defsStore(t,
		symdef{"a.go", "Handle", "", 3},
		symdef{"b.go", "Handle", "", 9},
	)

	got := mustRemap(t, st, key("Handle"))
	if got.Status != RemapAmbiguous {
		t.Fatalf("status = %v, want RemapAmbiguous", got.Status)
	}
	if got.Symbol != (graph.Symbol{}) {
		t.Fatalf("an ambiguous re-map must resolve to NO symbol, got %+v", got.Symbol)
	}
	if len(got.Candidates) != 2 {
		t.Fatalf("candidates = %+v, want both definitions", got.Candidates)
	}
	if got.Candidates[0].File != "a.go" || got.Candidates[1].File != "b.go" {
		t.Fatalf("candidates = %+v, want ProjectDefs' (file, start_line, name) order", got.Candidates)
	}
}

// The exact-parent filter is a NARROWING of a many-set, not a fourth drop
// condition. When it narrows to nothing the full candidate set is rendered
// ambiguous: dropping would hide rows the DB really returned, and the
// exactly-one rule above accepts a row without re-checking its parent, so the
// narrowing must not be more destructive than either neighbouring rule.
func TestRemapAllCandidatesFilteredOutStaysAmbiguous(t *testing.T) {
	st := defsStore(t,
		symdef{"a.go", "Handle", "Server", 3},
		symdef{"b.go", "Handle", "Client", 9},
	)

	got := mustRemap(t, st, key("Handle"))
	if got.Status != RemapAmbiguous {
		t.Fatalf("status = %v, want RemapAmbiguous", got.Status)
	}
	if len(got.Candidates) != 2 {
		t.Fatalf("candidates = %+v, want both rows kept", got.Candidates)
	}
}

// --- the named risk: the wrong reader --------------------------------------

// The re-map reads ProjectDefs, which filters tier = 0. A VENDORED TIER-1
// SNAPSHOT must never come back as a cross-repo target — that is the exact
// failure member-over-dep precedence exists to prevent, and it is what
// Definitions (no tier filter, no Tier/Namespace) would re-admit.
//
// The fixture is guarded against vacuity: Definitions DOES see the vendored
// row, so the tier filter is the only thing keeping it out of the answer.
func TestRemapNeverAdmitsAVendoredTierOneSnapshot(t *testing.T) {
	st := vendoredStore(t)

	all, err := st.Definitions("Zeta", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) == 0 {
		t.Fatal("fixture is vacuous: the vendored Zeta is not in the index at all")
	}

	got := mustRemap(t, st, key("Zeta"))
	if got.Status != RemapUnmapped {
		t.Fatalf("status = %v (symbol %+v, candidates %+v), want RemapUnmapped: "+
			"the only Zeta in this index is a vendored tier-1 snapshot",
			got.Status, got.Symbol, got.Candidates)
	}
}

// --- keys_unmapped ---------------------------------------------------------

// The drop is COUNTED at the re-map site, so a reference cannot be dropped
// without the clause learning about it.
func TestUnmappedKeysAreCountedIntoTheClause(t *testing.T) {
	st := defsStore(t, symdef{"a.go", "Present", "", 1})
	s := sessionFrom("/ws", threeMemberManifest(), wsfresh.Report{Resolved: true}, nil)

	if _, err := s.remapKey(st, key("Gone")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.remapKey(st, key("AlsoGone")); err != nil {
		t.Fatal(err)
	}
	if got, err := s.remapKey(st, key("Present")); err != nil {
		t.Fatal(err)
	} else if got.Status != RemapExact {
		t.Fatalf("status = %v, want RemapExact", got.Status)
	}

	c := s.clause("callers")
	if c.KeysUnmapped != 2 {
		t.Fatalf("keys_unmapped = %d, want 2", c.KeysUnmapped)
	}
	if want := "; keys_unmapped: 2"; !strings.Contains(c.String(), want) {
		t.Fatalf("clause text %q does not surface %q", c.String(), want)
	}
	if want := `"keys_unmapped":2`; !strings.Contains(marshalClause(t, c), want) {
		t.Fatalf("clause JSON %s does not surface %q", marshalClause(t, c), want)
	}
}

// keys_unmapped is a disclosure of an anomaly, not one of D6's three reserved
// fields: with nothing dropped there is nothing to disclose, and printing a
// zero on every workspace answer is noise the reader learns to skip.
func TestKeysUnmappedIsOmittedWhenNothingWasDropped(t *testing.T) {
	s := sessionFrom("/ws", threeMemberManifest(), wsfresh.Report{Resolved: true}, nil)

	c := s.clause("callers")
	if c.KeysUnmapped != 0 {
		t.Fatalf("keys_unmapped = %d, want 0", c.KeysUnmapped)
	}
	if strings.Contains(c.String(), "keys_unmapped") {
		t.Fatalf("clause text %q mentions keys_unmapped with nothing dropped", c.String())
	}
	if strings.Contains(marshalClause(t, c), "keys_unmapped") {
		t.Fatalf("clause JSON %s mentions keys_unmapped with nothing dropped", marshalClause(t, c))
	}
}
