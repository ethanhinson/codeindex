package lore

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRepo(t *testing.T, origin string) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"}, {"remote", "add", "origin", origin},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestRepoIDNormalizesOriginForms(t *testing.T) {
	ssh := RepoID(gitRepo(t, "git@github.com:ethanhinson/codeindex.git"))
	https := RepoID(gitRepo(t, "https://github.com/ethanhinson/codeindex"))
	if ssh != https {
		t.Fatalf("ssh %q != https %q", ssh, https)
	}
	if !strings.HasPrefix(ssh, "codeindex-") || len(ssh) != len("codeindex-")+12 {
		t.Fatalf("id shape: %q", ssh)
	}
}

func TestRepoIDFallsBackToPath(t *testing.T) {
	dir := t.TempDir() // not a git repo
	id := RepoID(dir)
	if id == "" || !strings.Contains(id, "-") {
		t.Fatalf("fallback id: %q", id)
	}
	// Assert the ID ends with a 12-hex-char suffix.
	parts := strings.Split(id, "-")
	suffix := parts[len(parts)-1]
	if len(suffix) != 12 {
		t.Fatalf("suffix %q not 12 hex chars", suffix)
	}
	// Assert relative path gives same ID as absolute form.
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	os.Chdir(filepath.Dir(dir))
	rel := filepath.Base(dir)
	// Both paths resolve through filepath.Abs which handles symlinks on macOS.
	if RepoID(rel) != RepoID(dir) {
		t.Fatalf("relative and absolute RepoID differ")
	}
}

func TestLayoutDirs(t *testing.T) {
	t.Setenv("CODEINDEX_HOME", t.TempDir())
	root := gitRepo(t, "git@github.com:e/x.git")
	l, err := NewLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	if l.RepoDir != filepath.Join(root, ".lore") {
		t.Fatalf("RepoDir %q", l.RepoDir)
	}
	if got := l.Dir("repo", TypeDecision); got != filepath.Join(root, ".lore", "decisions") {
		t.Fatalf("Dir repo/decision %q", got)
	}
	if got := l.Dir("overlay", TypeItem); !strings.HasPrefix(got, os.Getenv("CODEINDEX_HOME")) ||
		!strings.HasSuffix(got, "items") {
		t.Fatalf("Dir overlay/item %q", got)
	}
	if !strings.HasSuffix(l.SessionsDir(), "sessions") {
		t.Fatalf("SessionsDir %q", l.SessionsDir())
	}
}
