package merkle

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	_ "codeindex/internal/adapter/golang" // register .go
	_ "codeindex/internal/adapter/tsjs"   // register .js/.ts
)

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWalkExcludesCompiledAndVendored(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"internal/graph/store.go":              "package graph\n",
		"pkg/dist/assets/app.js":               "var x=1\n",
		"external/node_modules/react/index.js": "module.exports={}\n",
		"static/vendor.min.js":                 "var y=2\n",
	})

	got, err := Walk(root)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, "internal/graph/store.go") {
		t.Errorf("expected store.go indexed; got %v", got)
	}
	for _, bad := range []string{
		"pkg/dist/assets/app.js",
		"external/node_modules/react/index.js",
		"static/vendor.min.js",
	} {
		if slices.Contains(got, bad) {
			t.Errorf("expected %q excluded; got %v", bad, got)
		}
	}
}

func TestWalkIncludeReadmitsExcludedFile(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"pkg/dist/assets/app.js": "var x=1\n",
		"pkg/dist/keep.js":       "export const keep=1\n",
		".codeindex.json":        `{"include":["pkg/dist/keep.js"]}`,
	})

	got, err := Walk(root)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, "pkg/dist/keep.js") {
		t.Errorf("expected included keep.js indexed; got %v", got)
	}
	if slices.Contains(got, "pkg/dist/assets/app.js") {
		t.Errorf("expected non-included dist sibling excluded; got %v", got)
	}
}
