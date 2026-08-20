package wsfresh

import (
	"os"
	"path/filepath"
	"testing"

	"codeindex/internal/config"
	"codeindex/internal/query"
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
	stateIndexed memberState = iota // directory, real source, real index
	stateNoIndex                    // directory + real source, never indexed
	stateAbsent                     // no directory at all
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
		if m.state != stateIndexed {
			continue
		}
		if _, err := query.Fresh(dir); err != nil {
			t.Fatalf("building member %q index: %v", m.id, err)
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

// TestFreshenMissingMember: a declared member absent from disk lands in
// MembersMissing in manifest order and is not counted unindexed.
func TestFreshenMissingMember(t *testing.T) {
	wsRoot := buildWS(t,
		wsMember{id: "app", namespaces: []string{"App"}, src: goSrc("app", "AppOne")},
		wsMember{id: "gone", namespaces: []string{"Gone"}, state: stateAbsent},
	)
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
	if rep.MembersFreshened != 1 {
		t.Fatalf("MembersFreshened = %d, want 1", rep.MembersFreshened)
	}
}

// TestFreshenUnindexedMemberIsNotBuilt: a present member with no index counts
// unindexed and is NOT cold-built — no graph.db appears under it. Availability
// is graph.OpenExisting success and nothing else.
func TestFreshenUnindexedMemberIsNotBuilt(t *testing.T) {
	wsRoot := buildWS(t,
		wsMember{id: "app", namespaces: []string{"App"}, src: goSrc("app", "AppOne")},
		wsMember{id: "raw", namespaces: []string{"Raw"},
			src: goSrc("raw", "RawOne"), state: stateNoIndex},
	)
	rep, err := Freshen(wsRoot)
	if err != nil {
		t.Fatalf("Freshen: %v", err)
	}
	if rep.MembersUnindexed != 1 {
		t.Fatalf("MembersUnindexed = %d, want 1", rep.MembersUnindexed)
	}
	if rep.MembersFreshened != 1 {
		t.Fatalf("MembersFreshened = %d, want 1 (only the indexed member)",
			rep.MembersFreshened)
	}
	db := filepath.Join(wsRoot, "raw", ".codeindex", "graph.db")
	if _, err := os.Stat(db); err == nil {
		t.Fatalf("%s exists: Freshen cold-built an unindexed member", db)
	}
	for _, id := range rep.Dirty {
		if id == "raw" {
			t.Fatal("unindexed member 'raw' appears in Dirty; it must be left alone")
		}
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
	if rep.Resolved {
		t.Fatal("Resolved = true; steps 6-7 are Task 4 and must not run yet")
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
