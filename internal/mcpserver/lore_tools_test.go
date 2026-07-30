package mcpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeindex/internal/lore"
)

func loreFixtureRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("CODEINDEX_HOME", t.TempDir())
	root := t.TempDir()
	dir := filepath.Join(root, ".lore", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := lore.Record{
		ID: lore.NewID(lore.TypeDecision), Type: lore.TypeDecision,
		Title: "Use Go for the engine", Status: "active", Date: "2026-07-29",
		Anchors: []lore.Anchor{{Symbol: "ResolveImports"}},
		Body:    "static binary\n",
	}
	b, err := rec.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "d.md"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestLoreSearchTool(t *testing.T) {
	root := loreFixtureRepo(t)
	out, err := loreSearchText(root, "go engine", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Use Go for the engine") {
		t.Fatalf("search text:\n%s", out)
	}
}

func TestLoreForSymbolTool(t *testing.T) {
	root := loreFixtureRepo(t)
	out, err := loreForText(root, "ResolveImports")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Use Go for the engine") {
		t.Fatalf("for text:\n%s", out)
	}
	out, err = loreForText(root, "NoSuchSymbol")
	if err != nil || strings.TrimSpace(out) != "no lore records anchored to \"NoSuchSymbol\"" {
		t.Fatalf("miss text: %q %v", out, err)
	}
}
