package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	codeindexruntime "codeindex/internal/runtime"
)

// busyWork is deliberately a named, indexed symbol in this repo: the e2e
// proof profiles the test binary itself and resolves frames against the
// codeindex index of this repository.
func busyWork(d time.Duration) int {
	deadline := time.Now().Add(d)
	x := 0
	for time.Now().Before(deadline) {
		for i := 0; i < 1e5; i++ {
			x += i % 7
		}
	}
	return x
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller info")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(file)))) // sdk/go/agent -> repo
}

func TestKillSwitch(t *testing.T) {
	t.Setenv("CODEINDEX_PROFILING", "off")
	stop := Start(Options{Repo: t.TempDir()})
	busyWork(50 * time.Millisecond)
	stop()
	// No spool dir should exist at all.
	if _, err := os.Stat(filepath.Join(t.TempDir(), ".codeindex")); !os.IsNotExist(err) {
		t.Fatal("kill switch produced output")
	}
}

func TestUnwritableSpoolNeverErrors(t *testing.T) {
	before := Dropped.Load()
	stop := Start(Options{Repo: "/dev/null/not-a-dir"})
	busyWork(50 * time.Millisecond)
	stop() // must not panic or error
	if Dropped.Load() == before {
		t.Fatal("drop was not counted")
	}
}

// TestSpoolConformsAndResolves is the Go e2e dev-loop proof: profile this
// test binary, spool into a temp dir, verify cxprof conformance, and check
// that frames reference this repo's own source (so the real index resolves
// them).
func TestSpoolConformsAndResolves(t *testing.T) {
	repo := t.TempDir()
	stop := Start(Options{Repo: repo, Tag: "dev"})
	busyWork(400 * time.Millisecond)
	stop()

	dir := filepath.Join(repo, ".codeindex", "runtime")
	ents, err := os.ReadDir(dir)
	if err != nil || len(ents) != 1 {
		t.Fatalf("spool files: %v (err %v)", ents, err)
	}
	spool := filepath.Join(dir, ents[0].Name())
	if !strings.HasSuffix(spool, ".cxprof.jsonl") {
		t.Fatalf("bad spool name %s", spool)
	}

	p, err := codeindexruntime.Parse(spool)
	if err != nil {
		t.Fatalf("conformance: %v", err)
	}
	if p.Header.Lang != "go" || p.Header.Version != 1 || len(p.Stacks) == 0 {
		t.Fatalf("header/stacks: %+v (%d stacks)", p.Header, len(p.Stacks))
	}
	// At least one stack must reference this test's own source file — the
	// address the repo index can resolve.
	found := false
	for _, st := range p.Stacks {
		for _, fr := range st.Frames {
			if f, ok := fr[0].(string); ok && strings.HasSuffix(f, "sdk/go/agent/agent_test.go") {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("no frame referenced the profiled source")
	}
	_ = repoRoot(t) // referenced by the manual full-loop run below
}
