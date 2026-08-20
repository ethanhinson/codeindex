package wsresolve

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"codeindex/internal/config"
	"codeindex/internal/graph"
	"codeindex/internal/overlay"
)

// --- fixtures -------------------------------------------------------------

// memberState is how a declared member exists on disk. Every state but
// stateIndexed must leave the member unavailable and the pass unbroken.
type memberState int

const (
	stateIndexed    memberState = iota // directory + a usable index
	stateNoIndex                       // directory, no index at all
	stateBadVersion                    // directory + an index at a bogus user_version
	stateAbsent                        // no directory: missing from disk
)

// wsCall is one call site planted in a fixture member index. from names the
// enclosing symbol; callee/qualifier/ns are the reference's own hint columns.
type wsCall struct {
	file, from, callee, qualifier, ns string
	line                              int
}

// wsMember is one declared member of a fixture workspace.
type wsMember struct {
	id         string
	namespaces []string
	deps       []string
	defs       []def
	calls      []wsCall
	state      memberState
}

// buildWS lays out a workspace root: each member's tree under <wsRoot>/<id>,
// its index at <wsRoot>/<id>/.codeindex/graph.db, and the manifest declaring
// every member in the order given.
func buildWS(t *testing.T, members ...wsMember) string {
	t.Helper()
	wsRoot := t.TempDir()

	manifest := &config.Workspace{Version: 1}
	for _, m := range members {
		manifest.Members = append(manifest.Members, config.Member{
			ID: m.id, Root: m.id, Namespaces: m.namespaces, Deps: m.deps,
		})
		if m.state == stateAbsent {
			continue
		}
		dir := filepath.Join(wsRoot, m.id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if m.state == stateNoIndex {
			continue
		}
		writeIndex(t, dir, m.defs, m.calls, m.state == stateBadVersion)
	}
	if err := config.SaveWorkspace(wsRoot, manifest); err != nil {
		t.Fatal(err)
	}
	return wsRoot
}

// writeIndex builds a member index from scratch at <dir>/.codeindex/graph.db.
// graph.Open is legitimate here — it is the BUILD path, banned only from this
// package's non-test code.
func writeIndex(t *testing.T, dir string, defs []def, calls []wsCall, badVersion bool) {
	t.Helper()
	path := filepath.Join(dir, ".codeindex", "graph.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := graph.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	byFile := map[string][]graph.Symbol{}
	var order []string
	for _, d := range defs {
		if _, seen := byFile[d.file]; !seen {
			order = append(order, d.file)
		}
		line := d.line
		if line == 0 {
			line = 1
		}
		byFile[d.file] = append(byFile[d.file], graph.Symbol{
			File: d.file, Name: d.name, Parent: d.parent,
			Kind: graph.KindFunc, StartLine: line, EndLine: line + 1,
		})
	}
	for _, f := range order {
		var raw []graph.RawCall
		for _, c := range calls {
			if c.file != f {
				continue
			}
			idx := -1
			for i, s := range byFile[f] {
				if s.Name == c.from {
					idx = i
					break
				}
			}
			if idx < 0 {
				t.Fatalf("call %q in %s: no enclosing symbol %q", c.callee, f, c.from)
			}
			raw = append(raw, graph.RawCall{
				EnclosingIdx: idx, Callee: c.callee,
				Qualifier: c.qualifier, NsHint: c.ns, Line: c.line,
			})
		}
		tx, err := st.Begin()
		if err != nil {
			t.Fatal(err)
		}
		pf := &graph.ParsedFile{Path: f, Symbols: byFile[f], Calls: raw}
		if _, _, err := st.PutFile(tx, pf, graph.FileMeta{Path: f, Hash: "h-" + f, Size: 1, Mtime: 1}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	if badVersion {
		// OpenRaw + a PRAGMA write is the only way to forge a version mismatch:
		// graph.Open would delete and rebuild the file.
		db, err := graph.OpenRaw(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`PRAGMA user_version = 999999`); err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// twoWayWorkspace is the write-order fixture: m1 sources an edge into m2 AND
// m2 sources one back into m1, so a per-member ReplaceMemberEdges loses one of
// them whichever order it runs in.
func twoWayWorkspace(t *testing.T) string {
	t.Helper()
	return buildWS(t,
		wsMember{
			id: "m1", namespaces: []string{"example.com/m1"},
			defs: []def{{"a.go", "Caller", "", 1}, {"h.go", "Helper", "", 1}},
			calls: []wsCall{
				{file: "a.go", from: "Caller", callee: "Boot", ns: "example.com/m2", line: 7},
			},
		},
		wsMember{
			id: "m2", namespaces: []string{"example.com/m2"},
			defs: []def{{"b.go", "Boot", "", 1}, {"c.go", "Caller2", "", 1}},
			calls: []wsCall{
				{file: "c.go", from: "Caller2", callee: "Helper", ns: "example.com/m1", line: 9},
			},
		},
	)
}

func openOverlay(t *testing.T, wsRoot string) *overlay.Store {
	t.Helper()
	ov, err := overlay.Open(overlay.Path(wsRoot))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ov.Close() })
	return ov
}

// snapshotOverlay renders every derived row in the overlay as a comparable
// string: cross-edges and ambiguities per member, then the suppressions. It
// deliberately omits stamps, whose ResolvedAt moves on every pass.
func snapshotOverlay(t *testing.T, wsRoot string, memberIDs ...string) string {
	t.Helper()
	ov := openOverlay(t, wsRoot)
	var b strings.Builder
	for _, id := range memberIDs {
		edges, err := ov.MemberEdges(id)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range edges {
			fmt.Fprintf(&b, "edge %s %+v\n", id, e)
		}
		ambs, err := ov.AmbiguitiesFor(id)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range ambs {
			fmt.Fprintf(&b, "ambig %s %+v\n", id, a)
		}
	}
	sups, err := ov.Suppressions()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sups {
		fmt.Fprintf(&b, "supp %+v\n", s)
	}
	return b.String()
}

func mustResolve(t *testing.T, wsRoot string) Stats {
	t.Helper()
	st, err := Resolve(wsRoot)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return st
}

func hasEdge(edges []overlay.CrossEdge, src, dst overlay.SymKey) bool {
	for _, e := range edges {
		if e.Src == src && e.Dst == dst {
			return true
		}
	}
	return false
}

// --- the write-order regression -------------------------------------------

// TestWholePassWritesBothDirections is the regression the step 9/10 split
// exists for. ReplaceMemberEdges(M, …) deletes rows incident to M on EITHER
// end, so collapsing the clear-then-put pass back into a per-member
// derive-and-write loop makes the call for one member delete the cross-edge
// the call for the other just wrote. With an edge in each direction, exactly
// one survives that collapse whichever member order the loop takes — this test
// asserts BOTH survive.
func TestWholePassWritesBothDirections(t *testing.T) {
	wsRoot := twoWayWorkspace(t)

	stats := mustResolve(t, wsRoot)
	if stats.CrossEdges != 2 {
		t.Fatalf("Stats.CrossEdges = %d, want 2", stats.CrossEdges)
	}

	ov := openOverlay(t, wsRoot)
	edges, err := ov.MemberEdges("m1")
	if err != nil {
		t.Fatal(err)
	}
	forward := overlay.SymKey{Member: "m1", File: "a.go", QName: "Caller"}
	forwardDst := overlay.SymKey{Member: "m2", File: "b.go", QName: "Boot"}
	backward := overlay.SymKey{Member: "m2", File: "c.go", QName: "Caller2"}
	backwardDst := overlay.SymKey{Member: "m1", File: "h.go", QName: "Helper"}

	if !hasEdge(edges, forward, forwardDst) {
		t.Errorf("m1 -> m2 edge missing; edges = %+v", edges)
	}
	if !hasEdge(edges, backward, backwardDst) {
		t.Errorf("m2 -> m1 edge missing; edges = %+v", edges)
	}
	for _, e := range edges {
		if e.Provenance != "cross_repo_import" || e.Confidence != "exact" {
			t.Errorf("edge %+v: want cross_repo_import/exact", e)
		}
	}
}

// --- stable keys across a member rebuild ----------------------------------

func TestStableKeySurvivesMemberRebuild(t *testing.T) {
	wsRoot := twoWayWorkspace(t)
	mustResolve(t, wsRoot)
	before := snapshotOverlay(t, wsRoot, "m1", "m2")

	// Rebuild m2 from scratch with a leading extra file, so every symbol rowid
	// it had is renumbered.
	m2dir := filepath.Join(wsRoot, "m2")
	if err := os.RemoveAll(filepath.Join(m2dir, ".codeindex")); err != nil {
		t.Fatal(err)
	}
	writeIndex(t, m2dir,
		[]def{{"aaa.go", "Filler", "", 1}, {"aab.go", "Filler2", "", 1},
			{"b.go", "Boot", "", 1}, {"c.go", "Caller2", "", 1}},
		[]wsCall{{file: "c.go", from: "Caller2", callee: "Helper", ns: "example.com/m1", line: 9}},
		false)

	// Every recorded destination key must still name a live symbol in the
	// rebuilt member.
	ov := openOverlay(t, wsRoot)
	edges, err := ov.MemberEdges("m2")
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := graph.OpenExisting(filepath.Join(m2dir, ".codeindex", "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer rebuilt.Close()
	checked := 0
	for _, e := range edges {
		if e.Dst.Member != "m2" {
			continue
		}
		defs, err := rebuilt.ProjectDefs(e.Dst.QName, "")
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, d := range defs {
			if d.File == e.Dst.File && d.QName() == e.Dst.QName {
				found = true
			}
		}
		if !found {
			t.Errorf("cross-edge destination %+v names no live symbol after rebuild", e.Dst)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no cross-edge into m2 to check")
	}

	// And a second pass over the rebuilt member reproduces the same rows.
	mustResolve(t, wsRoot)
	if after := snapshotOverlay(t, wsRoot, "m1", "m2"); after != before {
		t.Errorf("overlay changed across member rebuild:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// --- idempotence ----------------------------------------------------------

func TestResolveIdempotent(t *testing.T) {
	wsRoot := twoWayWorkspace(t)

	first := mustResolve(t, wsRoot)
	beforeRows := snapshotOverlay(t, wsRoot, "m1", "m2")
	ov := openOverlay(t, wsRoot)
	beforeStamps, err := ov.Stamps()
	if err != nil {
		t.Fatal(err)
	}

	second := mustResolve(t, wsRoot)
	if first != second {
		t.Errorf("Stats differ across passes: %+v vs %+v", first, second)
	}
	if afterRows := snapshotOverlay(t, wsRoot, "m1", "m2"); afterRows != beforeRows {
		t.Errorf("overlay content differs across passes:\n%s\nvs\n%s", beforeRows, afterRows)
	}
	afterStamps, err := ov.Stamps()
	if err != nil {
		t.Fatal(err)
	}
	if len(afterStamps) != len(beforeStamps) {
		t.Fatalf("stamps = %d, want %d", len(afterStamps), len(beforeStamps))
	}
	for i := range afterStamps {
		if afterStamps[i].MemberID != beforeStamps[i].MemberID ||
			afterStamps[i].MerkleRoot != beforeStamps[i].MerkleRoot {
			t.Errorf("stamp %d differs beyond ResolvedAt: %+v vs %+v",
				i, beforeStamps[i], afterStamps[i])
		}
	}
}

// --- missing members ------------------------------------------------------

// TestMissingMembersLeftAlone: a declared member absent from disk contributes
// nothing and is never stamped, and a row joining two missing members survives
// a pass untouched — only rows incident to a member being re-resolved are
// cleared.
func TestMissingMembersLeftAlone(t *testing.T) {
	wsRoot := buildWS(t,
		wsMember{
			id: "m1", namespaces: []string{"example.com/m1"},
			defs: []def{{"a.go", "Caller", "", 1}},
			calls: []wsCall{
				{file: "a.go", from: "Caller", callee: "Boot", ns: "example.com/m2", line: 7},
			},
		},
		wsMember{
			id: "m2", namespaces: []string{"example.com/m2"},
			defs: []def{{"b.go", "Boot", "", 1}},
		},
		wsMember{id: "gone1", namespaces: []string{"example.com/gone1"}, state: stateAbsent},
		wsMember{id: "gone2", namespaces: []string{"example.com/gone2"}, state: stateAbsent},
	)

	// Seed a row joining the two missing members.
	orphan := overlay.CrossEdge{
		Src:  overlay.SymKey{Member: "gone1", File: "x.go", QName: "X"},
		Dst:  overlay.SymKey{Member: "gone2", File: "y.go", QName: "Y"},
		Kind: "call", Provenance: "cross_repo_name", Confidence: "inferred", Line: 3,
	}
	seed := openOverlay(t, wsRoot)
	if err := seed.ReplaceMemberEdges("gone1", []overlay.CrossEdge{orphan}, nil, nil); err != nil {
		t.Fatal(err)
	}

	stats := mustResolve(t, wsRoot)
	if stats.MembersResolved != 2 || stats.MembersUnavailable != 2 {
		t.Errorf("Stats = %+v, want 2 resolved / 2 unavailable", stats)
	}

	ov := openOverlay(t, wsRoot)
	edges, err := ov.MemberEdges("gone1")
	if err != nil {
		t.Fatal(err)
	}
	if !hasEdge(edges, orphan.Src, orphan.Dst) {
		t.Errorf("row joining two missing members did not survive; edges = %+v", edges)
	}
	for _, id := range []string{"gone1", "gone2"} {
		if _, ok, err := ov.Stamp(id); err != nil {
			t.Fatal(err)
		} else if ok {
			t.Errorf("missing member %q was stamped", id)
		}
	}
	for _, id := range []string{"m1", "m2"} {
		if _, ok, err := ov.Stamp(id); err != nil {
			t.Fatal(err)
		} else if !ok {
			t.Errorf("available member %q was not stamped", id)
		}
	}
}

// --- no member index is ever written --------------------------------------

// indexBytes snapshots every declared member's index file, recording absence
// as absence. A pass must not change one byte of any of them — not even the
// version-mismatched one, which graph.Open would delete and rebuild.
func indexBytes(t *testing.T, wsRoot string, ids ...string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for _, id := range ids {
		p := filepath.Join(wsRoot, id, ".codeindex", "graph.db")
		b, err := os.ReadFile(p)
		if err != nil {
			if !os.IsNotExist(err) {
				t.Fatal(err)
			}
			out[id] = nil
			continue
		}
		out[id] = b
	}
	return out
}

func TestMemberIndexesNeverWritten(t *testing.T) {
	ids := []string{"good", "bare", "old"}
	wsRoot := buildWS(t,
		wsMember{
			id: "good", namespaces: []string{"example.com/good"},
			defs: []def{{"a.go", "Caller", "", 1}},
			calls: []wsCall{
				{file: "a.go", from: "Caller", callee: "Boot", ns: "example.com/old", line: 7},
			},
		},
		wsMember{id: "bare", namespaces: []string{"example.com/bare"}, state: stateNoIndex},
		wsMember{
			id: "old", namespaces: []string{"example.com/old"},
			defs:  []def{{"b.go", "Boot", "", 1}},
			state: stateBadVersion,
		},
	)

	before := indexBytes(t, wsRoot, ids...)
	if before["bare"] != nil {
		t.Fatal("fixture: bare member should have no index")
	}
	if before["old"] == nil {
		t.Fatal("fixture: old member should have an index")
	}

	stats := mustResolve(t, wsRoot)
	if stats.MembersResolved != 1 || stats.MembersUnavailable != 2 {
		t.Errorf("Stats = %+v, want 1 resolved / 2 unavailable", stats)
	}

	after := indexBytes(t, wsRoot, ids...)
	for _, id := range ids {
		switch {
		case before[id] == nil && after[id] != nil:
			t.Errorf("member %q: index was CREATED by a resolution pass", id)
		case before[id] != nil && after[id] == nil:
			t.Errorf("member %q: index was DELETED by a resolution pass", id)
		case string(before[id]) != string(after[id]):
			t.Errorf("member %q: index bytes changed (%d -> %d)", id, len(before[id]), len(after[id]))
		}
	}

	// The unavailable member is not a candidate target either.
	if stats.CrossEdges != 0 {
		t.Errorf("Stats.CrossEdges = %d, want 0: an unavailable member is not a candidate", stats.CrossEdges)
	}
}

// --- root kind ------------------------------------------------------------

func TestResolveNonWorkspaceRootErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"),
		[]byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Resolve(root)
	if err == nil {
		t.Fatal("Resolve on a repo root returned no error")
	}
	if !strings.Contains(err.Error(), root) {
		t.Errorf("error %q does not name the path %q", err, root)
	}
}

// --- stats ----------------------------------------------------------------

func TestStatsCounts(t *testing.T) {
	wsRoot := buildWS(t,
		wsMember{
			id: "m1", namespaces: []string{"example.com/m1"},
			defs: []def{{"a.go", "Caller", "", 1}},
			calls: []wsCall{
				{file: "a.go", from: "Caller", callee: "Boot", ns: "example.com/m2", line: 7},
				{file: "a.go", from: "Caller", callee: "Nowhere", line: 8},
			},
		},
		wsMember{
			id: "m2", namespaces: []string{"example.com/m2"},
			defs: []def{{"b.go", "Boot", "", 1}},
		},
		wsMember{id: "unbuilt", namespaces: []string{"example.com/unbuilt"}, state: stateNoIndex},
		wsMember{id: "gone", namespaces: []string{"example.com/gone"}, state: stateAbsent},
	)

	stats := mustResolve(t, wsRoot)
	want := Stats{MembersResolved: 2, MembersUnavailable: 2, CrossEdges: 1}
	if stats != want {
		t.Errorf("Stats = %+v, want %+v", stats, want)
	}
	if stats.MembersResolved+stats.MembersUnavailable != 4 {
		t.Errorf("Stats accounts for %d of 4 declared members", stats.MembersResolved+stats.MembersUnavailable)
	}
}

// --- the graph.Open ban ---------------------------------------------------

// TestNoGraphOpenInNonTestCode is the cheap half of the graph.Open hazard bar
// (TestMemberIndexesNeverWritten is the behavioral half). It matches the CALL
// shape: "graph.Open(" — not a bare substring, because ladder.go's doc comment
// legitimately says "graph.OpenExisting", and OpenRaw/OpenExisting are both
// permitted names.
func TestNoGraphOpenInNonTestCode(t *testing.T) {
	call := regexp.MustCompile(`graph\.Open\s*\(`)
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		for i, line := range strings.Split(string(src), "\n") {
			if call.MatchString(line) {
				t.Errorf("%s:%d: graph.Open call in non-test code: %s",
					name, i+1, strings.TrimSpace(line))
			}
		}
	}
	if scanned == 0 {
		t.Fatal("scanned no non-test files")
	}
}
