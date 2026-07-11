package depmap_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeindex/internal/depmap"
	"codeindex/internal/engine"
	"codeindex/internal/graph"
	"codeindex/internal/query"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// End-to-end: generate a map from a fake dep, attach into a project that calls
// into it, verify tier priority + provenance + the hacked-dep overlay.
func TestDepmapAttachResolveOverlay(t *testing.T) {
	// Fake dep source: defines Logf and a name that collides with the project.
	dep := t.TempDir()
	write(t, dep, "log.go", `package fakelog
func Logf(f string) {}
func Shared() int { return 1 }
`)
	mapPath := filepath.Join(t.TempDir(), "fakelog.db")
	if _, _, err := depmap.Generate(dep, "example.com/fakelog", "v1.0.0", mapPath); err != nil {
		t.Fatal(err)
	}

	// Project: calls Logf (dep-only) and Shared (defined in BOTH tiers).
	repo := t.TempDir()
	write(t, repo, "main.go", `package p
func Shared() int { return 2 }
func Run() {
	Logf("x")
	Shared()
}
`)
	// Vendor the dep in-tree so the overlay path is exercised.
	write(t, repo, "vendor/example.com/fakelog/log.go", `package fakelog
func Logf(f string) {}
func Shared() int { return 1 }
`)
	db := filepath.Join(repo, ".codeindex", "graph.db")
	os.MkdirAll(filepath.Dir(db), 0o755)
	if _, err := engine.Build(repo, db); err != nil {
		t.Fatal(err)
	}
	if _, _, err := depmap.Attach(db, mapPath, "vendor/example.com/fakelog"); err != nil {
		t.Fatal(err)
	}

	// 1. Dep-only call resolves into the map with provenance.
	out, err := query.CalleesText(repo, "Run", 20)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Logf") || !strings.Contains(out, "[dep example.com/fakelog@v1.0.0]") {
		t.Errorf("Logf should resolve into the dep with provenance:\n%s", out)
	}

	// 2. Project beats dep on collision: Shared resolves to main.go, unambiguous.
	st, err := graph.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	callees, err := st.Callees("Run", "")
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	for _, c := range callees {
		if c.Name == "Shared" {
			if c.DefFile != "main.go" || c.Conf != graph.ConfUnambiguous {
				t.Errorf("Shared should resolve to project main.go unambiguously; got %+v", c)
			}
		}
	}

	// 3. Hacked-dep overlay: add a function to the vendored file.
	write(t, repo, "vendor/example.com/fakelog/log.go", `package fakelog
func Logf(f string) {}
func Shared() int { return 1 }
func Hacked() int { return 42 }
`)
	out, err = query.CallersText(repo, "Hacked", 5) // Fresh triggers VerifyOverlay
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "vendor/example.com/fakelog/log.go") {
		t.Errorf("hacked symbol should be indexed:\n%s", out)
	}
	out, _ = query.CalleesText(repo, "Run", 20)
	if !strings.Contains(out, "modified]") {
		t.Errorf("provenance should show modified:\n%s", out)
	}

	// 4. Restore: content back to map hash -> hacked symbol gone, clean marker.
	write(t, repo, "vendor/example.com/fakelog/log.go", `package fakelog
func Logf(f string) {}
func Shared() int { return 1 }
`)
	out, _ = query.CallersText(repo, "Hacked", 5)
	if !strings.Contains(out, "not found") {
		t.Errorf("restored dep should drop the hacked symbol:\n%s", out)
	}
	out, _ = query.CalleesText(repo, "Run", 20)
	if strings.Contains(out, "modified]") {
		t.Errorf("provenance should be clean after restore:\n%s", out)
	}
}
