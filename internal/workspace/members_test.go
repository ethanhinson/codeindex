package workspace

import (
	"testing"
)

// roots returns the member roots in returned order, for terse assertions.
func roots(t *testing.T, dir string) []string {
	t.Helper()
	ms, err := Members("testdata/" + dir)
	if err != nil {
		t.Fatalf("Members(%s): %v", dir, err)
	}
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Root
	}
	return out
}

func assertEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// --- one test per declaration source (spec §2b's five) ---

func TestMembersGoWork(t *testing.T) {
	assertEqual(t, roots(t, "ws_gowork"), []string{"a", "b"})
}

func TestMembersPnpmWorkspace(t *testing.T) {
	assertEqual(t, roots(t, "ws_pnpm"), []string{"packages/one", "packages/two"})
}

func TestMembersComposerPathRepositories(t *testing.T) {
	// Only the "type": "path" entry is a member; the vcs entry is not.
	assertEqual(t, roots(t, "ws_composer"), []string{"libs/alpha"})
}

func TestMembersLerna(t *testing.T) {
	assertEqual(t, roots(t, "ws_lerna"), []string{"packages/core"})
}

func TestMembersPackageJSONWorkspacesArray(t *testing.T) {
	assertEqual(t, roots(t, "ws_pkgarray"), []string{"packages/x"})
}

func TestMembersPackageJSONWorkspacesObject(t *testing.T) {
	assertEqual(t, roots(t, "ws_pkgobject"), []string{"packages/y"})
}

// --- one fixture per over-collection guard ---

// Guard 1: a plain file matching the glob is not a member.
func TestMembersGuardDirectoriesOnly(t *testing.T) {
	assertEqual(t, roots(t, "ws_guard1file"), []string{"packages/real"})
}

// Guard 1: a candidate traversing a DefaultExcludeDirs basename is dropped
// even though the declaration explicitly globs it.
func TestMembersGuardExcludedDirs(t *testing.T) {
	assertEqual(t, roots(t, "ws_guard1excl"), []string{"packages/real"})
}

// Guard 2: a marker one level below the candidate does not qualify it.
func TestMembersGuardMarkerAtOwnTopLevel(t *testing.T) {
	assertEqual(t, roots(t, "ws_guard2"), []string{"packages/real"})
}

// Guard 3: a candidate whose namespaces are a subset of another's is dropped.
func TestMembersGuardSubsetSuppression(t *testing.T) {
	assertEqual(t, roots(t, "ws_guard3"), []string{"packages/foo"})
}

// Guard 3 must not fire on an empty namespace set. An empty set is a strict
// subset of every non-empty set, so an unnamed-but-declared member (here a
// private package.json with no "name") would otherwise be deleted with no
// diagnostic and no way for Merge to restore it. The re-exporting monorepo
// root guard 3 exists to drop always has a NON-EMPTY set, so nothing is lost.
func TestMembersGuardSubsetKeepsEmptyNamespaceMember(t *testing.T) {
	assertEqual(t, roots(t, "ws_guard3empty"), []string{"packages/named", "packages/unnamed"})
}

// --- id derivation ---

func TestMembersIDs(t *testing.T) {
	ms, err := Members("testdata/ws_ids")
	if err != nil {
		t.Fatalf("Members: %v", err)
	}
	got := map[string]string{}
	for _, m := range ms {
		got[m.Root] = m.ID
	}
	want := map[string]string{
		"x/lib":    "lib",
		"y/lib":    "lib-2",
		"My Pkg+1": "my-pkg-1",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for root, id := range want {
		if got[root] != id {
			t.Errorf("root %q: id = %q, want %q", root, got[root], id)
		}
	}
}

func TestMembersNamespacesPopulated(t *testing.T) {
	ms, err := Members("testdata/ws_lerna")
	if err != nil {
		t.Fatalf("Members: %v", err)
	}
	if len(ms) != 1 || len(ms[0].Namespaces) != 1 || ms[0].Namespaces[0] != "@nestjs/core" {
		t.Fatalf("namespaces not populated: %+v", ms)
	}
}

func TestMembersNoDeclarations(t *testing.T) {
	ms, err := Members("testdata/gomod")
	if err != nil {
		t.Fatalf("Members: %v", err)
	}
	if len(ms) != 0 {
		t.Fatalf("got %+v, want no members", ms)
	}
}
