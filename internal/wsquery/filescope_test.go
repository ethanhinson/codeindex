package wsquery

import (
	"path/filepath"
	"strings"
	"testing"

	"codeindex/internal/graph"
	"codeindex/internal/overlay"
	"codeindex/internal/wsresolve"
)

// File-scope source keys (§3.8).
//
// A module-scope reference — an import at file scope — sits inside no symbol,
// so the ladder records its source key as {member, file, ""}. There is no
// symbol to invert, and the empty QName is not a re-map MISS: it is a
// deliberate "the file, no enclosing symbol", the same thing
// graph.Dependent.QName says by falling back to the file. Running such a key
// through ProjectDefs("", "") would find nothing and DROP the reference into
// keys_unmapped — silently losing every cross-repo import the ladder resolved.

// fileScopeEdge adds one cross-edge whose SOURCE is a file with no enclosing
// symbol, arriving at the fixture's lib:Target.
func fileScopeEdge(t *testing.T, ws string) {
	t.Helper()
	ov, err := overlay.Open(overlay.Path(ws))
	if err != nil {
		t.Fatal(err)
	}
	if err := ov.PutCrossEdges([]overlay.CrossEdge{{
		Src:  overlay.SymKey{Member: wsMemberWeb, File: "a.go", QName: ""},
		Dst:  overlay.SymKey{Member: wsMemberLib, File: "target.go", QName: "Target"},
		Kind: "imports", Provenance: "cross_repo_import", Confidence: ConfidenceExact, Line: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := ov.Close(); err != nil {
		t.Fatal(err)
	}
}

// The row is rendered from the recorded key — the FILE — and no key is counted
// unmapped.
func TestFileScopeCrossEdgeRendersAsTheFileAndIsNotAnUnmappedKey(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)
	fileScopeEdge(t, ws)

	a, err := Callers(ws, "Target", 100)
	if err != nil {
		t.Fatal(err)
	}
	got := callerLines(a.Callers)
	// web is manifest index 0 and InEdges orders by (src_file, src_qname), so
	// the empty QName sorts ahead of web's other a.go row.
	const want = "web|services/web/a.go:1|a.go|ambiguous=false|inferred=false"
	found := false
	for _, line := range got {
		if line == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("file-scope caller row missing.\n got: %v\nwant to contain: %s", got, want)
	}
	if a.CallersTotal != len(wantUnionCallers)+1 {
		t.Errorf("CallersTotal = %d, want %d", a.CallersTotal, len(wantUnionCallers)+1)
	}

	text, err := CallersText(ws, "Target", 100)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "keys_unmapped") {
		t.Errorf("a file-scope key was counted as an unmapped key:\n%s", text)
	}
	if !strings.Contains(text, "web: services/web/a.go:1") {
		t.Errorf("rendered text lost the file-scope row:\n%s", text)
	}
}

// The dependents verb reads the same rows, and its OWN half already renders a
// module-scope import with the file as the qualified name (graph.Dependent's
// own fallback). The cross half must say the same thing about the same shape,
// or one list carries two conventions.
func TestFileScopeCrossEdgeIsADependentNamedByItsFile(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)
	fileScopeEdge(t, ws)

	d, err := Dependents(ws, "Target", 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range d.Dependents {
		if r.Repo == wsMemberWeb && r.File == "services/web/a.go" && r.Line == 1 {
			found = true
			if r.QName != "a.go" {
				t.Errorf("file-scope dependent QName = %q, want the file %q", r.QName, "a.go")
			}
			if r.Kind != "imports" {
				t.Errorf("file-scope dependent Kind = %q, want imports", r.Kind)
			}
		}
	}
	if !found {
		t.Fatalf("file-scope dependent row missing: %+v", d.Dependents)
	}
}

// moduleScopeVendoringMember is vendoringMember's shape with the reference at
// MODULE SCOPE: `import Zeta` at file level, resolving into the vendored tier-1
// copy of acme/lib. It is the suppression path's version of the same edge, and
// the canonical PHP `use Vendored\Thing;` shape.
func moduleScopeVendoringMember(t *testing.T, id string) wsresolve.Member {
	t.Helper()
	dir := t.TempDir()

	st, err := graph.Open(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	pf := &graph.ParsedFile{
		Path: "c.go",
		Symbols: []graph.Symbol{
			{File: "c.go", Name: "Caller", Kind: graph.KindFunc, StartLine: 5, EndLine: 6},
		},
		Deps: []graph.RawDep{
			{EnclosingIdx: -1, Kind: graph.KindImports, Target: "Zeta", Source: "acme/lib", Line: 1},
		},
	}
	tx, err := st.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.PutFile(tx, pf, graph.FileMeta{Path: "c.go", Hash: "h", Size: 1, Mtime: 1}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	mapPath := filepath.Join(dir, "dep.map.db")
	m, err := graph.Open(mapPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.WriteDepMeta("acme/lib", "v1.2.3"); err != nil {
		t.Fatal(err)
	}
	if err := m.PutDepSymbols("acme/lib", "v1.2.3", "z.go", "maphash-z", 1, 1,
		[]graph.Symbol{{Name: "Zeta", Kind: graph.KindFunc, StartLine: 1, EndLine: 2}}); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, names, err := st.AttachMap(mapPath, "vendor/acme/lib"); err != nil {
		t.Fatal(err)
	} else if err := st.ReResolve(names); err != nil {
		t.Fatal(err)
	}
	return wsresolve.Member{ID: id, Namespaces: []string{"example.com/app"}, Store: st}
}

// TestModuleScopeSuppressedImportIsDroppedFromTheOwnHalf carries a module-scope
// edge through the WHOLE suppression path with every key produced by the real
// resolver: TierOneEdges must return it (empty SrcName), Suppress must re-point
// it, the ladder must record a cross-edge with an empty source QName, the
// call-site join must match on that empty QName, and the resulting drop key
// must match the own-half answer row — which internal/graph names by its FILE,
// not by the empty string. Every one of those five spellings has to agree, and
// they are the sites that can silently drift apart.
func TestModuleScopeSuppressedImportIsDroppedFromTheOwnHalf(t *testing.T) {
	app := moduleScopeVendoringMember(t, "app")
	lib := ownerMember(t, "lib", "Zeta")

	sup, cross, tierOne := realRecords(t, app, lib)
	if len(tierOne) != 1 || tierOne[0].SrcName != "" || tierOne[0].Kind != "imports" {
		t.Fatalf("tier-1 edges = %+v, want exactly the module-scope import", tierOne)
	}
	if len(cross) != 1 || cross[0].Src.QName != "" || cross[0].Src.File != "c.go" {
		t.Fatalf("cross edges = %+v, want one with a file-scope source key", cross)
	}

	kept, dropped := FilterSuppressedEdges("app", sup, tierOne, cross)
	if len(dropped) != 1 || len(kept) != 0 {
		t.Fatalf("kept %+v / dropped %+v, want the module-scope edge dropped", kept, dropped)
	}

	// The drop must land on the row the own half actually renders. graph's
	// Dependents names a module-scope importer by its file, so a drop key
	// carrying the empty QName would miss it and double-count the import.
	rows, err := app.Store.Dependents("Zeta")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("own dependents rows = %+v, want exactly one", rows)
	}
	drops := dependentDrops(dropped, "Zeta")
	k := inKey{Kind: string(rows[0].Kind), File: rows[0].File, QName: rows[0].QName(), Line: rows[0].Line}
	if !drops.take(k) {
		t.Fatalf("drop key set %+v does not cover the own answer row %+v (key %+v)", drops, rows[0], k)
	}
}
