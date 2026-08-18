package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeindex/internal/config"
)

// benchShapeRoot builds the D1 dedicated-workspace-directory shape: an EMPTY
// workspace directory whose members all live at "../" roots. This is the bench
// skeleton's own shape, and pass 1 legitimately discovers zero members here.
func benchShapeRoot(t *testing.T) string {
	t.Helper()
	base := t.TempDir()

	gomod := filepath.Join(base, "gosvc")
	mustMkdir(t, gomod)
	mustWrite(t, filepath.Join(gomod, "go.mod"), "module example.com/gosvc\n\ngo 1.22\n")

	node := filepath.Join(base, "web")
	mustMkdir(t, node)
	mustWrite(t, filepath.Join(node, "package.json"), `{"name":"@acme/web"}`)

	ws := filepath.Join(base, "oss-ws")
	mustMkdir(t, ws)
	return ws
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func findMember(t *testing.T, ms []config.Member, id string) config.Member {
	t.Helper()
	for _, m := range ms {
		if m.ID == id {
			return m
		}
	}
	t.Fatalf("member %q not in %v", id, ms)
	return config.Member{}
}

// The namespace pass runs over AUTHORED roots, not only over what pass 1
// discovered. On the bench shape pass 1 finds zero, and the whole result comes
// from the authored roots.
func TestScanFillsAuthoredRoots(t *testing.T) {
	ws := benchShapeRoot(t)
	authored := []config.Member{
		{ID: "gosvc", Root: "../gosvc", Namespaces: []string{}},
		{ID: "web", Root: "../web", Namespaces: []string{}, Deps: []string{"gosvc"}},
	}

	got, err := Scan(ws, authored)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d members, want 2: %v", len(got), got)
	}
	assertEqual(t, findMember(t, got, "gosvc").Namespaces, []string{"example.com/gosvc"})
	assertEqual(t, findMember(t, got, "web").Namespaces, []string{"@acme/web"})

	// Scan returns scan results only — it never echoes authored deps.
	if d := findMember(t, got, "web").Deps; len(d) != 0 {
		t.Fatalf("Scan echoed authored deps: %v", d)
	}
}

// Pass 1 still runs: a monorepo declaration at the workspace root contributes
// discovered members alongside the authored ones.
func TestScanDiscoversMembersAtWorkspaceRoot(t *testing.T) {
	base := t.TempDir()
	mustWrite(t, filepath.Join(base, "pnpm-workspace.yaml"), "packages:\n  - packages/*\n")
	pkg := filepath.Join(base, "packages", "core")
	mustMkdir(t, pkg)
	mustWrite(t, filepath.Join(pkg, "package.json"), `{"name":"@acme/core"}`)

	got, err := Scan(base, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %v, want one discovered member", got)
	}
	if got[0].Root != "packages/core" {
		t.Fatalf("root = %q", got[0].Root)
	}
	assertEqual(t, got[0].Namespaces, []string{"@acme/core"})
}

// Assumption 16: a member root that is not on disk is not an error; it just
// contributes no namespaces.
func TestScanNonexistentMemberRootContributesNothing(t *testing.T) {
	ws := benchShapeRoot(t)
	authored := []config.Member{
		{ID: "gone", Root: "../not-here", Namespaces: []string{}},
		{ID: "gosvc", Root: "../gosvc"},
	}

	got, err := Scan(ws, authored)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if ns := findMember(t, got, "gone").Namespaces; len(ns) != 0 {
		t.Fatalf("nonexistent root contributed %v", ns)
	}
	assertEqual(t, findMember(t, got, "gosvc").Namespaces, []string{"example.com/gosvc"})
}

// The empty-result error fires only with no authored members AND zero
// discovered — and it names the five declaration sources.
func TestScanEmptyResultError(t *testing.T) {
	ws := benchShapeRoot(t)

	_, err := Scan(ws, nil)
	if err == nil {
		t.Fatal("want an error when there are no authored members and nothing discovered")
	}
	for _, want := range []string{"go.work", "pnpm-workspace.yaml", "composer.json", "lerna.json", "package.json"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name %q: %v", want, err)
		}
	}
}

// When declarations DO exist but every one of them resolves outside the
// workspace root, the empty-result error must say so rather than claim nothing
// was declared, and must name an escaping path so the user can act on it.
func TestScanEmptyResultErrorNamesEscapingDeclarations(t *testing.T) {
	base := t.TempDir()

	shared := filepath.Join(base, "shared")
	mustMkdir(t, shared)
	mustWrite(t, filepath.Join(shared, "go.mod"), "module example.com/shared\n\ngo 1.22\n")

	ws := filepath.Join(base, "oss-ws")
	mustMkdir(t, ws)
	mustWrite(t, filepath.Join(ws, "go.work"), "go 1.22\n\nuse ../shared\n")

	_, err := Scan(ws, nil)
	if err == nil {
		t.Fatal("want an error when every declared member resolves outside the workspace root")
	}
	msg := err.Error()
	if strings.Contains(msg, "no members are declared") {
		t.Errorf("error falsely claims nothing was declared: %v", err)
	}
	if !strings.Contains(msg, "outside the workspace root") {
		t.Errorf("error does not explain the out-of-root omission: %v", err)
	}
	if !strings.Contains(msg, "../shared") {
		t.Errorf("error does not name the escaping path: %v", err)
	}
}

// ...and it must NOT fire merely because pass 1 discovered zero, even when
// every authored root is missing from disk.
func TestScanEmptyResultErrorDoesNotFireWithAuthoredMembers(t *testing.T) {
	ws := benchShapeRoot(t)
	if _, err := Scan(ws, []config.Member{{ID: "gone", Root: "../not-here"}}); err != nil {
		t.Fatalf("Scan must not error when authored members exist: %v", err)
	}
}

// --- Merge ---

func TestMergeMatchesByID(t *testing.T) {
	existing := []config.Member{{ID: "a", Root: "../a"}}
	scanned := []config.Member{{ID: "a", Root: "elsewhere", Namespaces: []string{"ns/a"}}}

	got := Merge(existing, scanned)
	if len(got) != 1 {
		t.Fatalf("got %v, want one member", got)
	}
	if got[0].Root != "../a" {
		t.Fatalf("scan overwrote root: %q", got[0].Root)
	}
	assertEqual(t, got[0].Namespaces, []string{"ns/a"})
}

func TestMergeMatchesByCleanedRootWhenIDDiffers(t *testing.T) {
	existing := []config.Member{{ID: "authored-id", Root: "../a/./"}}
	scanned := []config.Member{{ID: "a", Root: "../a", Namespaces: []string{"ns/a"}}}

	got := Merge(existing, scanned)
	if len(got) != 1 {
		t.Fatalf("got %v, want one member (matched by cleaned root)", got)
	}
	if got[0].ID != "authored-id" {
		t.Fatalf("scan overwrote authored id: %q", got[0].ID)
	}
	assertEqual(t, got[0].Namespaces, []string{"ns/a"})
}

func TestMergeAuthoredNonEmptyNamespacesWin(t *testing.T) {
	existing := []config.Member{{ID: "a", Root: "../a", Namespaces: []string{"authored/ns"}}}
	scanned := []config.Member{{ID: "a", Root: "../a", Namespaces: []string{"scanned/ns"}}}

	got := Merge(existing, scanned)
	assertEqual(t, got[0].Namespaces, []string{"authored/ns"})
}

func TestMergeNeverOverwritesRootOrDeps(t *testing.T) {
	existing := []config.Member{{ID: "a", Root: "../a", Deps: []string{"b"}}}
	scanned := []config.Member{{ID: "a", Root: "scanned/a", Namespaces: []string{"ns/a"}, Deps: []string{"c"}}}

	got := Merge(existing, scanned)
	if got[0].Root != "../a" {
		t.Fatalf("root = %q", got[0].Root)
	}
	assertEqual(t, got[0].Deps, []string{"b"})
}

// Existing members keep manifest order; newly discovered members are appended
// sorted by id.
func TestMergeOrdering(t *testing.T) {
	existing := []config.Member{
		{ID: "zeta", Root: "../zeta"},
		{ID: "alpha", Root: "../alpha"},
	}
	scanned := []config.Member{
		{ID: "zeta", Root: "../zeta", Namespaces: []string{"ns/z"}},
		{ID: "new-c", Root: "c"},
		{ID: "new-a", Root: "a"},
		{ID: "new-b", Root: "b"},
	}

	got := Merge(existing, scanned)
	ids := make([]string, len(got))
	for i, m := range got {
		ids[i] = m.ID
	}
	assertEqual(t, ids, []string{"zeta", "alpha", "new-a", "new-b", "new-c"})
}
