package gitinfo_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"codeindex/internal/lore/gitinfo"
)

// fakeRunner returns canned output for specific git subcommands.
type fakeRunner struct {
	outputs map[string]string // key: first arg after "git"
	errors  map[string]error
}

func (f *fakeRunner) run(dir string, args ...string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	key := args[0]
	if err, ok := f.errors[key]; ok {
		return "", err
	}
	return f.outputs[key], nil
}

// --- Unit tests (fake runner) ---

func TestAvailable_TrueWhenGitAndDotGitPresent(t *testing.T) {
	dir := t.TempDir()
	// create a .git directory
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	fr := &fakeRunner{
		outputs: map[string]string{"rev-parse": ""},
	}
	g := gitinfo.NewWithRunner(dir, fr.run)
	if !g.Available() {
		t.Fatal("expected Available=true when .git exists and git is on PATH")
	}
}

func TestAvailable_FalseWhenNoDotGit(t *testing.T) {
	dir := t.TempDir() // no .git
	fr := &fakeRunner{outputs: map[string]string{}}
	g := gitinfo.NewWithRunner(dir, fr.run)
	if g.Available() {
		t.Fatal("expected Available=false when .git missing")
	}
}

func TestAvailable_WorktreeFileIsAccepted(t *testing.T) {
	// In git worktrees, .git is a file, not a directory — os.Stat (not IsDir) should handle it.
	dir := t.TempDir()
	// create a .git file (simulating a worktree)
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: ../some/path"), 0o644); err != nil {
		t.Fatal(err)
	}
	fr := &fakeRunner{outputs: map[string]string{}}
	g := gitinfo.NewWithRunner(dir, fr.run)
	if !g.Available() {
		t.Fatal("expected Available=true when .git is a file (worktree)")
	}
}

func TestDefaultBranch_ParsesSymbolicRef(t *testing.T) {
	dir := t.TempDir()
	fr := &fakeRunner{
		outputs: map[string]string{
			"symbolic-ref": "refs/remotes/origin/HEAD\n",
		},
	}
	g := gitinfo.NewWithRunner(dir, fr.run)
	// Note: Available() won't matter here — we just test DefaultBranch parsing
	branch := g.DefaultBranch()
	// symbolic-ref output "refs/remotes/origin/HEAD" → strip "refs/remotes/origin/" → "HEAD"
	// but normally the output is "refs/remotes/origin/main" → "main"
	if branch != "HEAD" {
		t.Fatalf("expected 'HEAD', got %q", branch)
	}
}

func TestDefaultBranch_ParsesMainBranch(t *testing.T) {
	dir := t.TempDir()
	fr := &fakeRunner{
		outputs: map[string]string{
			"symbolic-ref": "refs/remotes/origin/main\n",
		},
	}
	g := gitinfo.NewWithRunner(dir, fr.run)
	branch := g.DefaultBranch()
	if branch != "main" {
		t.Fatalf("expected 'main', got %q", branch)
	}
}

func TestDefaultBranch_FallsBackToMainOnError(t *testing.T) {
	dir := t.TempDir()
	fr := &fakeRunner{
		outputs: map[string]string{},
		errors:  map[string]error{"symbolic-ref": exec.ErrNotFound},
	}
	g := gitinfo.NewWithRunner(dir, fr.run)
	branch := g.DefaultBranch()
	if branch != "main" {
		t.Fatalf("expected fallback 'main', got %q", branch)
	}
}

func TestFileOnBranch_TrueWhenNoError(t *testing.T) {
	dir := t.TempDir()
	fr := &fakeRunner{
		outputs: map[string]string{"cat-file": ""},
	}
	g := gitinfo.NewWithRunner(dir, fr.run)
	if !g.FileOnBranch("main", "some/file.go") {
		t.Fatal("expected FileOnBranch=true when runner returns no error")
	}
}

func TestFileOnBranch_FalseWhenError(t *testing.T) {
	dir := t.TempDir()
	fr := &fakeRunner{
		outputs: map[string]string{},
		errors:  map[string]error{"cat-file": exec.ErrNotFound},
	}
	g := gitinfo.NewWithRunner(dir, fr.run)
	if g.FileOnBranch("main", "missing/file.go") {
		t.Fatal("expected FileOnBranch=false when runner returns error")
	}
}

func TestCommitsSince_ParsesMultipleCommitsWithNumstat(t *testing.T) {
	// Two commits, multi-file, binary-file lines should be skipped
	logOutput := "abc123\x00first commit\n" +
		"10\t2\tfoo/bar.go\n" +
		"-\t-\tfoo/image.png\n" + // binary — must be skipped
		"5\t0\tfoo/baz.go\n" +
		"\n" +
		"def456\x00second commit\n" +
		"3\t1\tfoo/bar.go\n" +
		"\n"

	dir := t.TempDir()
	fr := &fakeRunner{
		outputs: map[string]string{"log": logOutput},
	}
	g := gitinfo.NewWithRunner(dir, fr.run)
	commits, err := g.CommitsSince("", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}

	// First commit
	c0 := commits[0]
	if c0.SHA != "abc123" {
		t.Fatalf("commit[0].SHA = %q, want 'abc123'", c0.SHA)
	}
	if c0.Subject != "first commit" {
		t.Fatalf("commit[0].Subject = %q, want 'first commit'", c0.Subject)
	}
	if len(c0.Files) != 2 {
		t.Fatalf("commit[0] should have 2 files (binary skipped), got %d", len(c0.Files))
	}
	if c0.Files["foo/bar.go"] != [2]int{10, 2} {
		t.Fatalf("foo/bar.go stats: %v", c0.Files["foo/bar.go"])
	}
	if c0.Files["foo/baz.go"] != [2]int{5, 0} {
		t.Fatalf("foo/baz.go stats: %v", c0.Files["foo/baz.go"])
	}

	// Second commit
	c1 := commits[1]
	if c1.SHA != "def456" {
		t.Fatalf("commit[1].SHA = %q, want 'def456'", c1.SHA)
	}
	if c1.Subject != "second commit" {
		t.Fatalf("commit[1].Subject = %q, want 'second commit'", c1.Subject)
	}
	if c1.Files["foo/bar.go"] != [2]int{3, 1} {
		t.Fatalf("commit[1] foo/bar.go stats: %v", c1.Files["foo/bar.go"])
	}
}

func TestCommitsSince_EmptyOutputReturnsEmptySlice(t *testing.T) {
	dir := t.TempDir()
	fr := &fakeRunner{
		outputs: map[string]string{"log": ""},
	}
	g := gitinfo.NewWithRunner(dir, fr.run)
	commits, err := g.CommitsSince("", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits) != 0 {
		t.Fatalf("expected 0 commits, got %d", len(commits))
	}
}

// --- Integration test ---

func TestIntegration_CommitsSince(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH, skipping integration test")
	}

	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Init the repo with an explicit branch name and CI-safe identity.
	run("-c", "init.defaultBranch=main", "init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")

	// First commit: add file1.go
	if err := os.WriteFile(filepath.Join(dir, "file1.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "file1.go")
	run("commit", "-m", "initial commit")

	// Second commit: add file2.go and modify file1.go, with a "closes" subject
	if err := os.WriteFile(filepath.Join(dir, "file1.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file2.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "file1.go", "file2.go")
	run("commit", "-m", "closes itm-TEST")

	g := gitinfo.New(dir)
	if !g.Available() {
		t.Fatal("Available() should be true for a real git repo")
	}

	commits, err := g.CommitsSince("", 10)
	if err != nil {
		t.Fatalf("CommitsSince: %v", err)
	}

	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d: %v", len(commits), commits)
	}

	// --reverse means oldest first
	if commits[0].Subject != "initial commit" {
		t.Fatalf("expected first commit subject 'initial commit', got %q", commits[0].Subject)
	}
	if commits[1].Subject != "closes itm-TEST" {
		t.Fatalf("expected second commit subject 'closes itm-TEST', got %q", commits[1].Subject)
	}

	// Second commit (closes itm-TEST) touched file1.go and file2.go
	c1 := commits[1]
	if _, ok := c1.Files["file1.go"]; !ok {
		t.Fatal("second commit should include file1.go in Files")
	}
	if _, ok := c1.Files["file2.go"]; !ok {
		t.Fatal("second commit should include file2.go in Files")
	}

	// file2.go was newly added: 1 line added, 0 deleted
	if c1.Files["file2.go"] != [2]int{1, 0} {
		t.Fatalf("file2.go stats: %v, want [1 0]", c1.Files["file2.go"])
	}

	// Test CommitsSince with sinceSHA — only the second commit should be returned
	firstSHA := commits[0].SHA
	since, err := g.CommitsSince(firstSHA, 10)
	if err != nil {
		t.Fatalf("CommitsSince(sha): %v", err)
	}
	if len(since) != 1 {
		t.Fatalf("expected 1 commit since first SHA, got %d", len(since))
	}
	if since[0].Subject != "closes itm-TEST" {
		t.Fatalf("expected 'closes itm-TEST', got %q", since[0].Subject)
	}

	// Test Head
	head, err := g.Head()
	if err != nil {
		t.Fatalf("Head(): %v", err)
	}
	if !strings.HasPrefix(head, commits[1].SHA) && head != commits[1].SHA {
		t.Fatalf("Head() = %q, want %q", head, commits[1].SHA)
	}

	// Test DefaultBranch (no remote in this temp repo, should fall back to "main")
	branch := g.DefaultBranch()
	if branch != "main" {
		t.Fatalf("DefaultBranch() = %q, want 'main'", branch)
	}

	// Test FileOnBranch
	if !g.FileOnBranch("main", "file1.go") {
		t.Fatal("file1.go should be on main branch")
	}
	if g.FileOnBranch("main", "nonexistent.go") {
		t.Fatal("nonexistent.go should not be on main branch")
	}
}
