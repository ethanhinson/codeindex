package wsfresh

import (
	"os"
	"path/filepath"
	"testing"

	"codeindex/internal/config"
	"codeindex/internal/graph"
	"codeindex/internal/query"
	"codeindex/internal/wsresolve"
)

// --- fixtures -------------------------------------------------------------

// memberState is how a declared member exists on disk, borrowing
// internal/wsresolve's vocabulary. Task 5 generalizes these fixtures; this set
// is deliberately minimal but REAL — every index here is built by the actual
// engine over actual source files, because Freshen runs query.Fresh (engine
// .Patch over a working tree) and folds real per-file content hashes. A
// hand-written synthetic index has no source behind it and would make these
// tests vacuous.
type memberState int

const (
	stateIndexed    memberState = iota // directory, real source, real index
	stateNoIndex                       // directory + real source, never indexed
	stateBadVersion                    // directory + real source + a real index at a bogus user_version
	stateAbsent                        // no directory at all
)

// wsMember is one declared member of a fixture workspace.
type wsMember struct {
	id         string
	namespaces []string
	deps       []string
	// src maps a relative file path to its contents. Real source, parsed by
	// the real engine.
	src   map[string]string
	state memberState
}

// buildWS lays out a workspace root: each member's tree under <wsRoot>/<id>
// and the manifest declaring every member in the order given. Indexed members
// are built the way the product does, via query.Fresh on the member root.
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
		for rel, body := range m.src {
			p := filepath.Join(dir, rel)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if m.state != stateIndexed && m.state != stateBadVersion {
			continue
		}
		if _, err := query.Fresh(dir); err != nil {
			t.Fatalf("building member %q index: %v", m.id, err)
		}
		if m.state == stateBadVersion {
			// A REAL index, then a forged version: the file exists and is
			// intact, and only graph.OpenExisting's version check rejects it.
			corruptIndexVersion(t, wsRoot, m.id)
		}
	}
	if err := config.SaveWorkspace(wsRoot, manifest); err != nil {
		t.Fatal(err)
	}
	return wsRoot
}

func goSrc(pkg, fn string) map[string]string {
	return map[string]string{
		pkg + ".go": "package " + pkg + "\n\nfunc " + fn + "() int { return 1 }\n",
	}
}

// --- tests ----------------------------------------------------------------

// TestFreshenRejectsNonWorkspaceRoot: root kind is checked BEFORE the manifest
// load, so a plain repo root reports what it actually is instead of a bare
// fs.ErrNotExist on a path the caller never named.
func TestFreshenRejectsNonWorkspaceRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"),
		[]byte("package a\n\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Freshen(dir)
	if err == nil {
		t.Fatal("Freshen on a repo root: want error, got nil")
	}
	if got := err.Error(); !contains(got, "not a workspace root") {
		t.Fatalf("Freshen error = %q, want it to say the root is not a workspace root", got)
	}
}

// TestFreshenMissingMember (plan test 6, missing half): a declared member absent
// from disk lands in MembersMissing in manifest order, is not counted unindexed,
// and does not by itself make the pass dirty — the second pass over the same
// workspace still takes the gate's clean branch.
//
// The fixture is crossEdgeWS's real two-member pair PLUS the absent member, so
// the workspace still carries a real cross-edge while one declared member is
// gone. A workspace whose only interesting property is the missing member cannot
// show that the rest of the pass survived it.
func TestFreshenMissingMember(t *testing.T) {
	wsRoot := crossEdgeWS(t, wsMember{
		id: "gone", namespaces: []string{"example.com/gone"}, state: stateAbsent,
	})
	rep, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("Freshen: %v", err)
	}
	if len(rep.MembersMissing) != 1 || rep.MembersMissing[0] != "gone" {
		t.Fatalf("MembersMissing = %v, want [gone]", rep.MembersMissing)
	}
	if rep.MembersUnindexed != 0 {
		t.Fatalf("MembersUnindexed = %d, want 0 (a missing member is not unindexed)",
			rep.MembersUnindexed)
	}
	if rep.MembersFreshened != 2 {
		t.Fatalf("MembersFreshened = %d, want 2 (app and lib)", rep.MembersFreshened)
	}
	assertCrossEdge(t, wsRoot, targetFile)

	// The missing member alone must not keep the pass dirty forever.
	second, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("second Freshen: %v", err)
	}
	if len(second.Dirty) != 0 || second.Resolved {
		t.Fatalf("second pass Dirty=%v Resolved=%v — a missing member must not make the pass dirty",
			second.Dirty, second.Resolved)
	}
}

// TestFreshenUnindexedMemberIsNotBuilt (plan test 6, unindexed half): a present
// member with no index counts unindexed and is NOT cold-built — no graph.db
// appears under it, on either pass. Availability is graph.OpenExisting success
// and nothing else, established BEFORE query.Fresh, so Fresh never reaches its
// cold-build branch. Nor does the unindexed member alone make the pass dirty.
func TestFreshenUnindexedMemberIsNotBuilt(t *testing.T) {
	wsRoot := crossEdgeWS(t, wsMember{
		id: "raw", namespaces: []string{"example.com/raw"},
		src: goSrc("raw", "RawOne"), state: stateNoIndex,
	})
	rep, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("Freshen: %v", err)
	}
	if rep.MembersUnindexed != 1 {
		t.Fatalf("MembersUnindexed = %d, want 1", rep.MembersUnindexed)
	}
	if rep.MembersFreshened != 2 {
		t.Fatalf("MembersFreshened = %d, want 2 (only the indexed members)",
			rep.MembersFreshened)
	}
	assertNotBuilt(t, wsRoot, "raw")
	for _, id := range rep.Dirty {
		if id == "raw" {
			t.Fatal("unindexed member 'raw' appears in Dirty; it must be left alone")
		}
	}

	second, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("second Freshen: %v", err)
	}
	if len(second.Dirty) != 0 || second.Resolved {
		t.Fatalf("second pass Dirty=%v Resolved=%v — an unindexed member must not make the pass dirty",
			second.Dirty, second.Resolved)
	}
	// Resolve ran on the first pass too, and it is the second enforcement site
	// for the same never-cold-build invariant.
	assertNotBuilt(t, wsRoot, "raw")
}

// assertNotBuilt fails if an index appeared under a member Freshen was supposed
// to leave alone.
func assertNotBuilt(t *testing.T, wsRoot, memberID string) {
	t.Helper()
	db := filepath.Join(wsRoot, memberID, ".codeindex", "graph.db")
	if _, err := os.Stat(db); err == nil {
		t.Fatalf("%s exists: a member with no index was cold-built", db)
	}
}

// TestFreshenFirstPassIsDirty: with no stamps in a fresh overlay, every
// available member is dirty — the absent stamp is the self-healing signal
// 0013's stamps-last ordering leaves behind.
func TestFreshenFirstPassIsDirty(t *testing.T) {
	wsRoot := buildWS(t,
		wsMember{id: "app", namespaces: []string{"App"}, src: goSrc("app", "AppOne")},
		wsMember{id: "lib", namespaces: []string{"Lib"}, src: goSrc("lib", "LibOne")},
	)
	rep, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("Freshen: %v", err)
	}
	if len(rep.Dirty) != 2 || rep.Dirty[0] != "app" || rep.Dirty[1] != "lib" {
		t.Fatalf("Dirty = %v, want [app lib] in manifest order", rep.Dirty)
	}
	// Dirty members take the gate's other branch: exactly one whole-pass
	// resolution, whose Stats are carried into the Report.
	if !rep.Resolved {
		t.Fatal("Resolved = false; a dirty first pass must re-resolve")
	}
	if rep.Stats.MembersResolved != 2 {
		t.Fatalf("Stats.MembersResolved = %d, want 2 — the Resolve Stats must be carried into the Report",
			rep.Stats.MembersResolved)
	}
}

// TestFreshenCleanSecondPassDoesNotResolve is the gate's clean branch: nothing
// dirty and no registry drift means the derived set is still entailed by what
// is on disk, so the pass ends without re-resolving. (Task 6 adds the full
// content-tuple assertion that no overlay content was written at all; this
// asserts the gate's own decision.)
func TestFreshenCleanSecondPassDoesNotResolve(t *testing.T) {
	wsRoot := buildWS(t,
		wsMember{id: "app", namespaces: []string{"App"}, deps: []string{"lib"},
			src: goSrc("app", "AppOne")},
		wsMember{id: "lib", namespaces: []string{"Lib"}, src: goSrc("lib", "LibOne")},
	)
	if _, err := Freshen(wsRoot); err != nil {
		t.Fatalf("first Freshen: %v", err)
	}
	rep, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("second Freshen: %v", err)
	}
	if len(rep.Dirty) != 0 {
		t.Fatalf("Dirty = %v, want empty on an unchanged workspace", rep.Dirty)
	}
	if rep.Resolved {
		t.Fatal("Resolved = true on a clean pass; the gate must not re-resolve")
	}
	if rep.Stats != (wsresolve.Stats{}) {
		t.Fatalf("Stats = %+v, want the zero value when Resolved is false", rep.Stats)
	}
}

// TestFreshenManifestNamespacesDriftReResolves: namespaces are a ladder INPUT
// (rung 1 resolves on them). Editing only a member's namespaces changes
// resolution while every source file, every merkle root and every stamp stays
// exactly where it was — so nothing is dirty, and only the registry-drift half
// of the gate can catch it. An id-set-only comparison would let the overlay
// serve edges derived from the old claims indefinitely.
func TestFreshenManifestNamespacesDriftReResolves(t *testing.T) {
	wsRoot := freshenedWS(t)
	editManifest(t, wsRoot, func(ws *config.Workspace) {
		ws.Members[1].Namespaces = []string{"Lib", "LibExtra"}
	})
	rep, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("Freshen: %v", err)
	}
	if len(rep.Dirty) != 0 {
		t.Fatalf("Dirty = %v, want empty — no source changed, so drift is the only signal",
			rep.Dirty)
	}
	if !rep.Resolved {
		t.Fatal("Resolved = false after a namespaces: edit; the drift check must catch it")
	}
}

// TestFreshenManifestDepsDriftReResolves is the same argument for deps, the
// other ladder input: Suppress derives member-over-dep precedence from it.
func TestFreshenManifestDepsDriftReResolves(t *testing.T) {
	wsRoot := freshenedWS(t)
	editManifest(t, wsRoot, func(ws *config.Workspace) {
		ws.Members[1].Deps = []string{"app"}
	})
	rep, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("Freshen: %v", err)
	}
	if len(rep.Dirty) != 0 {
		t.Fatalf("Dirty = %v, want empty — no source changed", rep.Dirty)
	}
	if !rep.Resolved {
		t.Fatal("Resolved = false after a deps: edit; the drift check must catch it")
	}
}

// TestFreshenManifestMemberOrderDriftReResolves is the third named component of
// the drift comparison, and the only one no other test moves: member ORDER.
// Swapping two members edits no field of any record — the same ids, roots,
// namespaces and deps are all still present — so an order-insensitive
// comparison sees no drift at all, while the stored registry's ordinals have
// genuinely changed. Nothing is dirty, because no source file and no stamp
// moved; the whole-record, order-sensitive comparison is the only signal.
func TestFreshenManifestMemberOrderDriftReResolves(t *testing.T) {
	wsRoot := freshenedWS(t)
	editManifest(t, wsRoot, func(ws *config.Workspace) {
		ws.Members[0], ws.Members[1] = ws.Members[1], ws.Members[0]
	})
	rep, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("Freshen: %v", err)
	}
	if len(rep.Dirty) != 0 {
		t.Fatalf("Dirty = %v, want empty — only the member order moved", rep.Dirty)
	}
	if !rep.Resolved {
		t.Fatal("Resolved = false after a member reorder; the drift check must catch it")
	}
}

// TestFreshenConvergesOnDuplicateNamespace is the first convergence sibling.
// A duplicate namespace entry is a LEGAL manifest — config validation does not
// reject it — but ReplaceRegistry dedupes on the way in, so the stored form can
// never equal the raw manifest. Comparing the raw slice therefore reports drift
// on every pass and re-resolves the whole workspace forever. Normalizing the
// manifest side with the writer's own normalizer is what makes this converge.
func TestFreshenConvergesOnDuplicateNamespace(t *testing.T) {
	wsRoot := buildWS(t,
		wsMember{id: "app", namespaces: []string{"App", "App"}, deps: []string{"lib"},
			src: goSrc("app", "AppOne")},
		wsMember{id: "lib", namespaces: []string{"Lib"}, src: goSrc("lib", "LibOne")},
	)
	assertConverges(t, wsRoot)
}

// TestFreshenConvergesOnEmptyDepsArray is the second convergence sibling. An
// explicit "deps": [] in the committed manifest unmarshals to a non-nil empty
// slice, while dedupe() stores nil for it — so a raw comparison never settles.
// The manifest is written by hand here because config.SaveWorkspace's
// omitempty tag would drop the field and hide the case entirely.
func TestFreshenConvergesOnEmptyDepsArray(t *testing.T) {
	wsRoot := buildWS(t,
		wsMember{id: "app", namespaces: []string{"App"}, src: goSrc("app", "AppOne")},
		wsMember{id: "lib", namespaces: []string{"Lib"}, src: goSrc("lib", "LibOne")},
	)
	writeManifestJSON(t, wsRoot, `{
  "version": 1,
  "members": [
    {"id": "app", "root": "app", "namespaces": ["App"], "deps": []},
    {"id": "lib", "root": "lib", "namespaces": ["Lib"], "deps": []}
  ]
}
`)
	// Guard the premise: if the manifest ever stopped round-tripping "deps": []
	// as a non-nil empty slice, this test would pass vacuously.
	ws, err := config.LoadWorkspace(wsRoot)
	if err != nil {
		t.Fatalf("LoadWorkspace: %v", err)
	}
	if ws.Members[0].Deps == nil || len(ws.Members[0].Deps) != 0 {
		t.Fatalf(`Deps = %#v, want a non-nil empty slice — the premise of this test`,
			ws.Members[0].Deps)
	}
	assertConverges(t, wsRoot)
}

// assertConverges runs two Freshens and requires the second to be clean: the
// first pass is dirty (no stamps yet) and writes the registry, the second must
// agree with what was written.
func assertConverges(t *testing.T, wsRoot string) {
	t.Helper()
	first, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("first Freshen: %v", err)
	}
	if !first.Resolved {
		t.Fatal("first Freshen did not resolve; the fixture is not exercising the gate")
	}
	second, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("second Freshen: %v", err)
	}
	if len(second.Dirty) != 0 {
		t.Fatalf("second Freshen Dirty = %v, want empty", second.Dirty)
	}
	if second.Resolved {
		t.Fatal("second Freshen re-resolved: the drift comparison does not converge on a legal manifest")
	}
}

// freshenedWS is a two-member workspace already taken through one Freshen, so
// every stamp matches and the registry is in sync — the state in which the only
// thing a manifest edit can move is the drift check.
func freshenedWS(t *testing.T) string {
	t.Helper()
	wsRoot := buildWS(t,
		wsMember{id: "app", namespaces: []string{"App"}, deps: []string{"lib"},
			src: goSrc("app", "AppOne")},
		wsMember{id: "lib", namespaces: []string{"Lib"}, src: goSrc("lib", "LibOne")},
	)
	if _, err := Freshen(wsRoot); err != nil {
		t.Fatalf("priming Freshen: %v", err)
	}
	rep, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("settling Freshen: %v", err)
	}
	if rep.Resolved || len(rep.Dirty) != 0 {
		t.Fatalf("workspace did not settle: Resolved=%v Dirty=%v", rep.Resolved, rep.Dirty)
	}
	return wsRoot
}

// editManifest rewrites the manifest through mutate, touching no source file
// and no index — so merkle roots and stamps are untouched by construction.
func editManifest(t *testing.T, wsRoot string, mutate func(*config.Workspace)) {
	t.Helper()
	ws, err := config.LoadWorkspace(wsRoot)
	if err != nil {
		t.Fatalf("LoadWorkspace: %v", err)
	}
	mutate(ws)
	if err := config.SaveWorkspace(wsRoot, ws); err != nil {
		t.Fatalf("SaveWorkspace: %v", err)
	}
}

// writeManifestJSON writes the manifest bytes verbatim, for shapes
// config.SaveWorkspace cannot emit.
func writeManifestJSON(t *testing.T, wsRoot, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(wsRoot, config.WorkspaceFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- the freshen-failure outcome ------------------------------------------

// breakMemberFreshen makes query.Fresh fail on a member whose index opens
// cleanly. engine.Patch's very first act is loadAssociations, which calls
// config.Load, which fails on a malformed .codeindex.json — so Fresh returns an
// error while the member's graph.db is untouched and graph.OpenExisting (the
// ONE availability predicate, shared with wsresolve.Resolve) still succeeds.
// That combination is the whole point: this member is available and indexed,
// and only its patch failed.
func breakMemberFreshen(t *testing.T, wsRoot, memberID string) {
	t.Helper()
	p := filepath.Join(memberDir(wsRoot, memberID), config.FileName)
	if err := os.WriteFile(p, []byte("{ this is not json"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestFreshenFailureIsItsOwnOutcome: a member that OPENS but whose per-repo
// freshen fails is counted in MembersFreshenFailed, NOT MembersUnindexed.
//
// This is Report's documented relation to wsresolve.Stats with teeth. The
// resolver's availability predicate is graph.OpenExisting alone, so it counts
// this member RESOLVED; folding it into MembersUnindexed would break both
// stated identities at once — MembersUnindexed would exceed
// Stats.MembersUnavailable (0 here), and MembersFreshened alone would no longer
// account for Stats.MembersResolved.
func TestFreshenFailureIsItsOwnOutcome(t *testing.T) {
	wsRoot := crossEdgeWS(t, wsMember{
		id: "brk", namespaces: []string{"example.com/brk"},
		src: goSrc("brk", "BrkOne"),
	})
	breakMemberFreshen(t, wsRoot, "brk")

	// Premise guard: the member is available, and only Fresh fails.
	if _, err := query.Fresh(memberDir(wsRoot, "brk")); err == nil {
		t.Fatal("query.Fresh succeeded on the broken member; the fixture forges nothing")
	}
	if st, err := graph.OpenExisting(memberDB(wsRoot, "brk")); err != nil {
		t.Fatalf("graph.OpenExisting failed on the broken member (%v); it would be "+
			"unindexed, not freshen-failed, and the test proves nothing", err)
	} else {
		st.Close()
	}

	rep, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("Freshen: %v", err)
	}
	if rep.MembersFreshenFailed != 1 {
		t.Errorf("MembersFreshenFailed = %d, want 1", rep.MembersFreshenFailed)
	}
	if rep.MembersUnindexed != 0 {
		t.Errorf("MembersUnindexed = %d, want 0 — a member that opened is indexed; "+
			"only its patch failed", rep.MembersUnindexed)
	}
	if rep.MembersFreshened != 2 {
		t.Errorf("MembersFreshened = %d, want 2 (app and lib)", rep.MembersFreshened)
	}
	if len(rep.MembersMissing) != 0 {
		t.Errorf("MembersMissing = %v, want empty", rep.MembersMissing)
	}
	if !rep.Resolved {
		t.Fatal("first pass did not resolve; Stats is meaningless and the identities untestable")
	}

	// The two identities the Report type comment states.
	if want := rep.MembersUnindexed + len(rep.MembersMissing); rep.Stats.MembersUnavailable != want {
		t.Errorf("Stats.MembersUnavailable = %d, want %d (MembersUnindexed + len(MembersMissing))",
			rep.Stats.MembersUnavailable, want)
	}
	if want := rep.MembersFreshened + rep.MembersFreshenFailed; rep.Stats.MembersResolved != want {
		t.Errorf("Stats.MembersResolved = %d, want %d (MembersFreshened + MembersFreshenFailed) "+
			"— the resolver counts a freshen-failed member as resolved",
			rep.Stats.MembersResolved, want)
	}
}
