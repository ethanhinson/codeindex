package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeindex/internal/config"
)

// A root holding the workspace manifest is a workspace — classified by a stat,
// never a parse. The manifest here is deliberately malformed: reporting that is
// LoadWorkspace's job, and downgrading it to a single repo would be worse.
func TestDetectRootKindWorkspaceNoParse(t *testing.T) {
	dir := writeTree(t, map[string]string{
		config.WorkspaceFile: "{ this is not json",
	})
	kind, err := DetectRootKind(dir)
	if err != nil {
		t.Fatalf("workspace root: unexpected error: %v", err)
	}
	if kind != RootWorkspace {
		t.Fatalf("kind = %v, want RootWorkspace", kind)
	}
}

// os.Stat on <root>/.codeindex/workspace.json returns ENOTDIR (not
// ErrNotExist) when .codeindex exists as a regular file rather than a
// directory. That is absence, not a fault: an ordinary repo caught in this
// state must still classify as RootRepo, not error out.
func TestDetectRootKindCodeindexAsRegularFileIsRepo(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"README.md":  "# docs\n",
		"pkg/run.go": "package pkg\n\nfunc Run() {}\n",
	})
	if err := os.WriteFile(filepath.Join(dir, ".codeindex"), []byte("not a directory\n"), 0o644); err != nil {
		t.Fatalf("write .codeindex file: %v", err)
	}
	kind, err := DetectRootKind(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kind != RootRepo {
		t.Fatalf("kind = %v, want RootRepo", kind)
	}
}

// A root with at least one indexable source file and no manifest is a repo.
func TestDetectRootKindRepo(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"README.md":  "# docs\n",
		"pkg/run.go": "package pkg\n\nfunc Run() {}\n",
	})
	kind, err := DetectRootKind(dir)
	if err != nil {
		t.Fatalf("repo root: unexpected error: %v", err)
	}
	if kind != RootRepo {
		t.Fatalf("kind = %v, want RootRepo", kind)
	}
}

// Neither manifest nor indexable source: the error must name both
// possibilities, so the user is not left guessing which one they meant.
func TestDetectRootKindNeitherNamesBoth(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"README.md":   "# docs\n",
		"notes/a.txt": "plain text\n",
	})
	_, err := DetectRootKind(dir)
	if err == nil {
		t.Fatal("want an error for a root that is neither a repo nor a workspace")
	}
	msg := err.Error()
	for _, want := range []string{"not a code repository", "no indexable source", "not a workspace", config.WorkspaceFile} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not mention %q", msg, want)
		}
	}
}

// The helper stops at the first admitted file — it must not walk the whole
// tree — and it consults the filter both ways: an excluded directory and an
// excluded file are never offered to the predicate, even though both sort
// ahead of the first real source file (WalkDir is lexical, so the fixture
// names are load-bearing).
func TestHasIndexableSourceShortCircuits(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.min.js":    "// bundled\n",  // excluded by DefaultExcludeGlobs
		"dist/aaa.go": "package aaa\n", // excluded by DefaultExcludeDirs
		"z.go":        "package z\n",   // first admitted file
		"zz.go":       "package zz\n",  // must never be reached
	})
	filter, err := config.LoadFilter(dir)
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	ok, err := hasIndexableSource(dir, filter, func(rel string) bool {
		seen = append(seen, rel)
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("want ok=true")
	}
	if len(seen) != 1 || seen[0] != "z.go" {
		t.Fatalf("predicate calls = %v, want exactly [z.go] (short-circuit + filter)", seen)
	}
}

// Excluded directories are pruned, not merely filtered file-by-file: an
// unreadable node_modules must never be descended into. (SkipFile alone would
// hide this — it rejects everything under a default-skip dir anyway — so the
// only observable difference between pruning and descending is whether the
// walk touches the directory at all.)
func TestHasIndexableSourcePrunesExcludedDirs(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	dir := writeTree(t, map[string]string{
		"node_modules/dep/a.go": "package dep\n",
		"z.go":                  "package z\n",
	})
	unreadable := filepath.Join(dir, "node_modules")
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) }) // let TempDir clean up
	filter, err := config.LoadFilter(dir)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := hasIndexableSource(dir, filter, func(string) bool { return true })
	if err != nil {
		t.Fatalf("excluded dir should be pruned, not read: %v", err)
	}
	if !ok {
		t.Fatal("want ok=true")
	}
}

// With nothing the predicate claims, the helper reports false and no error
// after exhausting the tree.
func TestHasIndexableSourceNoHit(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.txt": "x\n", "sub/b.txt": "y\n"})
	filter, err := config.LoadFilter(dir)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := hasIndexableSource(dir, filter, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("want ok=false")
	}
}
