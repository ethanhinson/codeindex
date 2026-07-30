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

func TestLoreAddRecordWithPriorityAndBlockedBy(t *testing.T) {
	root := loreFixtureRepo(t)
	out, err := loreAddRecord(root, lore.TypeItem, "Test item", "body text", nil, nil, false, "p1", []string{"tag1", "tag2"}, []string{"itm-001"})
	if err != nil {
		t.Fatalf("loreAddRecord failed: %v", err)
	}
	if !strings.Contains(out, "created itm-") {
		t.Fatalf("unexpected output: %s", out)
	}

	// Extract the file path and verify the created item has the expected fields
	parts := strings.Fields(out)
	if len(parts) < 3 {
		t.Fatalf("malformed output: %s", out)
	}
	filePath := parts[2]

	// Read the file back and parse it
	b, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read created file: %v", err)
	}
	rec, err := lore.Parse(b, lore.TypeItem)
	if err != nil {
		t.Fatalf("failed to parse record: %v", err)
	}

	// Verify fields
	if rec.Priority != "p1" {
		t.Errorf("expected priority p1, got %q", rec.Priority)
	}
	if len(rec.Tags) != 2 || rec.Tags[0] != "tag1" || rec.Tags[1] != "tag2" {
		t.Errorf("expected tags [tag1 tag2], got %v", rec.Tags)
	}
	if len(rec.BlockedBy) != 1 || rec.BlockedBy[0] != "itm-001" {
		t.Errorf("expected blocked_by [itm-001], got %v", rec.BlockedBy)
	}
}
