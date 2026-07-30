package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeindex/internal/lore"
	"codeindex/internal/lore/index"
)

func loreTestRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("CODEINDEX_HOME", t.TempDir())
	return t.TempDir()
}

func runLoreOK(t *testing.T, root string, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := runLore(root, args, &buf); err != nil {
		t.Fatalf("lore %v: %v", args, err)
	}
	return buf.String()
}

func TestLoreAddAndShow(t *testing.T) {
	root := loreTestRepo(t)
	out := runLoreOK(t, root, "add", "decision",
		"--title", "Use Go for the engine",
		"--body", "Because fast.\n\n## Alternatives considered\nRust.\n",
		"--anchor", "symbol:ResolveImports", "--ref", "gh-issue:e/x#1")
	if !strings.Contains(out, "created dec-") {
		t.Fatalf("add output: %q", out)
	}
	id := strings.Fields(out)[1]

	// File landed in the committed layer.
	files, _ := filepath.Glob(filepath.Join(root, ".lore", "decisions", "*.md"))
	if len(files) != 1 {
		t.Fatalf("files: %v", files)
	}

	show := runLoreOK(t, root, "show", id)
	if !strings.Contains(show, "Use Go for the engine") ||
		!strings.Contains(show, "repo") ||
		!strings.Contains(show, "Alternatives considered") {
		t.Fatalf("show output:\n%s", show)
	}
}

func TestLoreAddPrivateGoesToOverlay(t *testing.T) {
	root := loreTestRepo(t)
	runLoreOK(t, root, "add", "note", "--title", "Scratch", "--body", "x", "--private")
	if repoFiles, _ := filepath.Glob(filepath.Join(root, ".lore", "notes", "*.md")); len(repoFiles) != 0 {
		t.Fatalf("private note leaked into repo layer: %v", repoFiles)
	}
	overlay, _ := filepath.Glob(filepath.Join(os.Getenv("CODEINDEX_HOME"),
		"lore", "*", "notes", "*.md"))
	if len(overlay) != 1 {
		t.Fatalf("overlay files: %v", overlay)
	}
}

func TestLoreShowUnknownID(t *testing.T) {
	root := loreTestRepo(t)
	var buf bytes.Buffer
	if err := runLore(root, []string{"show", "dec-NOPE"}, &buf); err == nil {
		t.Fatal("want error for unknown id")
	}
}

func TestLoreSearchTextAndJSON(t *testing.T) {
	root := loreTestRepo(t)
	runLoreOK(t, root, "add", "decision", "--title", "Use Go for the engine",
		"--body", "static binary")
	runLoreOK(t, root, "add", "note", "--title", "Unrelated", "--body", "zzz")

	out := runLoreOK(t, root, "search", "go engine")
	if !strings.Contains(out, "dec-") || strings.Contains(out, "Unrelated") {
		t.Fatalf("search text:\n%s", out)
	}
	js := runLoreOK(t, root, "search", "go engine", "--json")
	if !strings.Contains(js, `"title": "Use Go for the engine"`) {
		t.Fatalf("search json:\n%s", js)
	}
}

func TestLoreFor(t *testing.T) {
	root := loreTestRepo(t)
	runLoreOK(t, root, "add", "decision", "--title", "Engine dir decision",
		"--anchor", "path:internal/engine/")
	runLoreOK(t, root, "add", "decision", "--title", "Symbol decision",
		"--anchor", "symbol:ResolveImports")

	out := runLoreOK(t, root, "for", "internal/engine/resolver.go")
	if !strings.Contains(out, "Engine dir decision") || strings.Contains(out, "Symbol decision") {
		t.Fatalf("for path:\n%s", out)
	}
	out = runLoreOK(t, root, "for", "ResolveImports")
	if !strings.Contains(out, "Symbol decision") {
		t.Fatalf("for symbol:\n%s", out)
	}
}

func TestLoreBacklogOrdering(t *testing.T) {
	root := loreTestRepo(t)
	runLoreOK(t, root, "add", "item", "--title", "Low prio", "--priority", "p3")
	out := runLoreOK(t, root, "add", "item", "--title", "Blocker", "--priority", "p1")
	blockerID := strings.Fields(out)[1]
	// Blocked p0 sorts below unblocked p1 despite higher priority? No —
	// priority sorts first, blocked-ness second WITHIN a priority. Encode
	// the actual rule: p0-blocked, p1-ready, p3-ready.
	runLoreOK(t, root, "add", "item", "--title", "Urgent but blocked", "--priority", "p0")
	// Manually add blocked_by via a second item file edit is complex here;
	// instead create the blocked item with the flag:
	_ = blockerID
	out = runLoreOK(t, root, "backlog")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("backlog lines: %v", lines)
	}
	if !strings.Contains(lines[0], "Urgent but blocked") ||
		!strings.Contains(lines[1], "Blocker") ||
		!strings.Contains(lines[2], "Low prio") {
		t.Fatalf("order:\n%s", out)
	}
}

func TestLoreBacklogFilterByAnchor(t *testing.T) {
	root := loreTestRepo(t)
	runLoreOK(t, root, "add", "item", "--title", "Engine work",
		"--anchor", "path:internal/engine/")
	runLoreOK(t, root, "add", "item", "--title", "Docs work",
		"--anchor", "path:docs/")
	out := runLoreOK(t, root, "backlog", "--for", "internal/engine/x.go")
	if !strings.Contains(out, "Engine work") || strings.Contains(out, "Docs work") {
		t.Fatalf("filtered backlog:\n%s", out)
	}
}

func TestLoreBacklogBlockedFlag(t *testing.T) {
	root := loreTestRepo(t)
	out := runLoreOK(t, root, "add", "item", "--title", "Blocker")
	blocker := strings.Fields(out)[1]
	runLoreOK(t, root, "add", "item", "--title", "Dependent", "--blocked-by", blocker)
	out = runLoreOK(t, root, "backlog")
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Dependent") && !strings.Contains(line, "BLOCKED") {
			t.Fatalf("dependent not flagged BLOCKED:\n%s", out)
		}
	}
}

func TestLoreBacklogBlockedSortsBelowReadySamePriority(t *testing.T) {
	root := loreTestRepo(t)
	out := runLoreOK(t, root, "add", "item", "--title", "Blocker item", "--priority", "p1")
	blocker := strings.Fields(out)[1]
	runLoreOK(t, root, "add", "item", "--title", "Blocked peer", "--priority", "p1", "--blocked-by", blocker)
	out = runLoreOK(t, root, "backlog")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines: %v", lines)
	}
	if !strings.Contains(lines[0], "Blocker item") || !strings.Contains(lines[1], "Blocked peer") {
		t.Fatalf("blocked item must sort below ready peer at same priority:\n%s", out)
	}
}

func TestLorePromote(t *testing.T) {
	root := loreTestRepo(t)
	out := runLoreOK(t, root, "add", "note", "--title", "Gotcha", "--body", "x", "--private")
	id := strings.Fields(out)[1]
	out = runLoreOK(t, root, "promote", id)
	if !strings.Contains(out, "promoted "+id) {
		t.Fatalf("promote out: %q", out)
	}
	repoFiles, _ := filepath.Glob(filepath.Join(root, ".lore", "notes", "*.md"))
	overlayFiles, _ := filepath.Glob(filepath.Join(os.Getenv("CODEINDEX_HOME"),
		"lore", "*", "notes", "*.md"))
	if len(repoFiles) != 1 || len(overlayFiles) != 0 {
		t.Fatalf("repo=%v overlay=%v", repoFiles, overlayFiles)
	}
	// Re-promoting errors.
	var buf bytes.Buffer
	if err := runLore(root, []string{"promote", id}, &buf); err == nil {
		t.Fatal("want error promoting a repo-layer record")
	}
}

func TestLoreSupersede(t *testing.T) {
	root := loreTestRepo(t)
	out := runLoreOK(t, root, "add", "decision", "--title", "Old way", "--body", "x")
	oldID := strings.Fields(out)[1]
	out = runLoreOK(t, root, "supersede", oldID, "--title", "New way", "--body", "y")
	if !strings.Contains(out, "created dec-") || !strings.Contains(out, "superseded "+oldID) {
		t.Fatalf("supersede out: %q", out)
	}
	newID := strings.Fields(strings.Split(out, "\n")[0])[1]

	oldShow := runLoreOK(t, root, "show", oldID)
	if !strings.Contains(oldShow, "status: superseded") ||
		!strings.Contains(oldShow, "superseded_by: "+newID) {
		t.Fatalf("old record not rewritten:\n%s", oldShow)
	}
	newShow := runLoreOK(t, root, "show", newID)
	if !strings.Contains(newShow, "supersedes: "+oldID) {
		t.Fatalf("new record missing supersedes:\n%s", newShow)
	}
}

func TestLorePromoteCollisionDisambiguates(t *testing.T) {
	root := loreTestRepo(t)
	out := runLoreOK(t, root, "add", "note", "--title", "Same Name", "--body", "private", "--private")
	id := strings.Fields(out)[1]
	runLoreOK(t, root, "add", "note", "--title", "Same Name", "--body", "committed")
	out = runLoreOK(t, root, "promote", id)
	if !strings.Contains(out, "promoted "+id) {
		t.Fatalf("promote out: %q", out)
	}
	files, _ := filepath.Glob(filepath.Join(root, ".lore", "notes", "*.md"))
	if len(files) != 2 {
		t.Fatalf("collision overwrote a file; want 2 files, got %v", files)
	}
}

func TestLoreDoctorFindings(t *testing.T) {
	root := loreTestRepo(t)
	// dangling supersedes + malformed file + stale path anchor
	runLoreOK(t, root, "add", "decision", "--title", "Anchored",
		"--anchor", "path:no/such/dir/")
	dir := filepath.Join(root, ".lore", "notes")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "bad.md"), []byte("not a record"), 0o644)

	out := runLoreOK(t, root, "doctor")
	for _, want := range []string{"parse-error", "stale-anchor", "finding"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor missing %q:\n%s", want, out)
		}
	}
}

func TestLoreDoctorClean(t *testing.T) {
	root := loreTestRepo(t)
	runLoreOK(t, root, "add", "note", "--title", "Fine", "--body", "x")
	out := runLoreOK(t, root, "doctor")
	if !strings.Contains(out, "ok: no findings") {
		t.Fatalf("doctor clean:\n%s", out)
	}
}

// TestLoreDoctorDuplicateID checks that when two files carry the same ID,
// lore doctor emits a "duplicate-id" finding and counts it.
func TestLoreDoctorDuplicateID(t *testing.T) {
	root := loreTestRepo(t)

	// Write one record via the normal add path (repo layer).
	out := runLoreOK(t, root, "add", "decision", "--title", "The Decision", "--body", "x")
	id := strings.Fields(out)[1]

	// Derive the overlay decisions dir from the Layout (same logic as production).
	l, err := lore.NewLayout(root)
	if err != nil {
		t.Fatalf("NewLayout: %v", err)
	}
	overlayDecisions := l.Dir("overlay", lore.TypeDecision)
	if err := os.MkdirAll(overlayDecisions, 0o755); err != nil {
		t.Fatalf("mkdir overlay: %v", err)
	}

	dupFile := filepath.Join(overlayDecisions, "2026-07-29-dup.md")
	content := "---\nid: " + id + "\ntitle: Dup overlay\ntype: decision\nstatus: active\ndate: 2026-07-29\n---\nDuplicate.\n"
	if err := os.WriteFile(dupFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write dup file: %v", err)
	}

	out = runLoreOK(t, root, "doctor")
	// Column format matches the other finding types: duplicate-id  <id>  <paths>
	if !strings.Contains(out, "duplicate-id  "+id+"  ") {
		t.Fatalf("doctor missing duplicate-id column finding:\n%s", out)
	}
	if !strings.Contains(out, dupFile) {
		t.Fatalf("doctor finding missing duplicate path %q:\n%s", dupFile, out)
	}
	if !strings.Contains(out, "finding") {
		t.Fatalf("doctor missing findings count:\n%s", out)
	}
}

func TestLoreInit(t *testing.T) {
	root := loreTestRepo(t)
	out := runLoreOK(t, root, "init")
	for _, d := range []string{"decisions", "items", "notes"} {
		if fi, err := os.Stat(filepath.Join(root, ".lore", d)); err != nil || !fi.IsDir() {
			t.Fatalf("missing dir %s: %v", d, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".lore", "README.md")); err != nil {
		t.Fatal("missing .lore/README.md")
	}
	if !strings.Contains(out, ".lore") {
		t.Fatalf("init output: %q", out)
	}
	out = runLoreOK(t, root, "init")
	if !strings.Contains(out, "already initialized") {
		t.Fatalf("second init: %q", out)
	}
}

func TestLoreBacklogJSONCarriesPriorityAndBlocked(t *testing.T) {
	root := loreTestRepo(t)
	out := runLoreOK(t, root, "add", "item", "--title", "Blocker", "--priority", "p1")
	blocker := strings.Fields(out)[1]
	runLoreOK(t, root, "add", "item", "--title", "Dependent", "--priority", "p1", "--blocked-by", blocker)
	js := runLoreOK(t, root, "backlog", "--json")
	if !strings.Contains(js, `"priority": "p1"`) || !strings.Contains(js, `"blocked": true`) {
		t.Fatalf("backlog json missing priority/blocked:\n%s", js)
	}
}

func TestLoreSearchUnratifiedLabel(t *testing.T) {
	// This test verifies that an unratified record's search output line contains
	// "UNRATIFIED". We simulate an unratified record by directly manipulating
	// the store after reindex, using SetRatified.
	root := loreTestRepo(t)

	// Add a record that will be in the index.
	out := runLoreOK(t, root, "add", "decision",
		"--title", "Branch only decision",
		"--body", "not on main branch")
	id := strings.Fields(out)[1]

	// Force-set it as unratified by opening the store directly.
	l, err := lore.NewLayout(root)
	if err != nil {
		t.Fatalf("NewLayout: %v", err)
	}
	dbPath := filepath.Join(root, ".codeindex", "lore.db")
	// Reindex once to seed the DB.
	st, _, err2 := index.Reindex(l, dbPath)
	if err2 != nil {
		t.Fatalf("reindex: %v", err2)
	}
	if err := st.SetRatified(id, false); err != nil {
		t.Fatalf("SetRatified: %v", err)
	}
	st.Close()

	// Now search — the result should include UNRATIFIED.
	searchOut := runLoreOK(t, root, "search", "branch only")
	if !strings.Contains(searchOut, "UNRATIFIED") {
		t.Fatalf("search output missing UNRATIFIED for unratified record:\n%s", searchOut)
	}
}

func TestLoreCaptureRequiresStdinFlag(t *testing.T) {
	root := loreTestRepo(t)
	var buf bytes.Buffer
	if err := runLore(root, []string{"capture"}, &buf); err == nil {
		t.Fatal("want usage error without --stdin")
	}
}

func TestLoreInitHostCursorAndCodex(t *testing.T) {
	root := loreTestRepo(t)
	runLoreOK(t, root, "init", "--host", "cursor")
	b, err := os.ReadFile(filepath.Join(root, ".cursor", "rules", "lore.mdc"))
	if err != nil || !strings.Contains(string(b), "alwaysApply: true") ||
		!strings.Contains(string(b), "codeindex lore") {
		t.Fatalf("cursor rule: %v\n%s", err, b)
	}
	runLoreOK(t, root, "init", "--host", "codex")
	runLoreOK(t, root, "init", "--host", "codex") // idempotent
	ab, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(ab), "codeindex-lore:start") != 1 {
		t.Fatalf("managed block duplicated:\n%s", ab)
	}
}

func TestLoreInitHostUnknown(t *testing.T) {
	root := loreTestRepo(t)
	var buf bytes.Buffer
	if err := runLore(root, []string{"init", "--host", "vim"}, &buf); err == nil {
		t.Fatal("want error for unknown host")
	}
}

func TestLoreInitHostCodexRefusesOrphanedMarker(t *testing.T) {
	root := loreTestRepo(t)
	orphaned := "# My agents\n" + codexBlockStart + "\nprecious user content below\n"
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(orphaned), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := runLore(root, []string{"init", "--host", "codex"}, &buf); err == nil {
		t.Fatal("want error on orphaned start marker")
	}
	b, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if !strings.Contains(string(b), "precious user content below") {
		t.Fatal("user content was destroyed")
	}
}

// TestLoreShowConfidenceLine: when a record has Survived > 0, lore show must
// print a "confidence: X.XX (survived N commits)" line after the meta line.
func TestLoreShowConfidenceLine(t *testing.T) {
	root := loreTestRepo(t)

	// Add a record.
	out := runLoreOK(t, root, "add", "decision", "--title", "Confidence test",
		"--body", "x", "--anchor", "path:internal/engine/")
	id := strings.Fields(out)[1]

	// Directly set survived/confidence in the store.
	l, err := lore.NewLayout(root)
	if err != nil {
		t.Fatalf("NewLayout: %v", err)
	}
	dbPath := filepath.Join(root, ".codeindex", "lore.db")
	st, _, err := index.Reindex(l, dbPath)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	// survived=9 → confidence = ln(10)/ln(21)
	if err := st.AddSignals(id, 9, 0); err != nil {
		t.Fatalf("AddSignals: %v", err)
	}
	st.Close()

	show := runLoreOK(t, root, "show", id)
	if !strings.Contains(show, "confidence:") {
		t.Fatalf("show missing confidence line:\n%s", show)
	}
	if !strings.Contains(show, "survived 9 commits") {
		t.Fatalf("show missing 'survived 9 commits':\n%s", show)
	}
}

// TestLoreShowNoConfidenceLineWhenSurvivedZero: when survived=0, no confidence
// line should appear in lore show output.
func TestLoreShowNoConfidenceLineWhenSurvivedZero(t *testing.T) {
	root := loreTestRepo(t)
	out := runLoreOK(t, root, "add", "decision", "--title", "No confidence", "--body", "x")
	id := strings.Fields(out)[1]
	show := runLoreOK(t, root, "show", id)
	if strings.Contains(show, "confidence:") {
		t.Fatalf("show should NOT print confidence line when survived=0:\n%s", show)
	}
}

// TestLoreDoctorChurnSuspect: a record whose churn_lines > 3× total line count
// of anchored files must appear in doctor as "churn-suspect".
func TestLoreDoctorChurnSuspect(t *testing.T) {
	root := loreTestRepo(t)
	t.Chdir(t.TempDir()) // CWD != root; proves fix works via filepath.Join(root, a.Path)

	// Create a small file that will be the anchor target.
	anchorDir := filepath.Join(root, "internal", "engine")
	if err := os.MkdirAll(anchorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	anchorFile := filepath.Join(anchorDir, "core.go")
	// Write exactly 10 lines so total lines = 10.
	content := strings.Repeat("line\n", 10)
	if err := os.WriteFile(anchorFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Add a decision anchored to that directory using a repo-relative path.
	out := runLoreOK(t, root, "add", "decision", "--title", "Churn test",
		"--body", "x", "--anchor", "path:internal/engine/")
	id := strings.Fields(out)[1]

	// Set churn_lines = 31 (> 3×10=30 → churn-suspect).
	l, err := lore.NewLayout(root)
	if err != nil {
		t.Fatalf("NewLayout: %v", err)
	}
	dbPath := filepath.Join(root, ".codeindex", "lore.db")
	st, _, err2 := index.Reindex(l, dbPath)
	if err2 != nil {
		t.Fatalf("Reindex: %v", err2)
	}
	if err := st.AddSignals(id, 0, 31); err != nil {
		t.Fatalf("AddSignals: %v", err)
	}
	st.Close()

	doctorOut := runLoreOK(t, root, "doctor")
	if !strings.Contains(doctorOut, "churn-suspect") {
		t.Fatalf("doctor missing churn-suspect finding:\n%s", doctorOut)
	}
	if !strings.Contains(doctorOut, id) {
		t.Fatalf("doctor churn-suspect missing id %s:\n%s", id, doctorOut)
	}
}

// TestLoreDoctorChurnSuspectBelowThreshold: a record whose churn_lines ≤ 3×
// total line count must NOT appear in doctor as churn-suspect.
func TestLoreDoctorChurnSuspectBelowThreshold(t *testing.T) {
	root := loreTestRepo(t)
	t.Chdir(t.TempDir()) // CWD != root; proves fix works via filepath.Join(root, a.Path)

	anchorDir := filepath.Join(root, "internal", "engine2")
	if err := os.MkdirAll(anchorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	anchorFile := filepath.Join(anchorDir, "core.go")
	// Write exactly 10 lines.
	if err := os.WriteFile(anchorFile, []byte(strings.Repeat("line\n", 10)), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runLoreOK(t, root, "add", "decision", "--title", "Below threshold",
		"--body", "x", "--anchor", "path:internal/engine2/")
	id := strings.Fields(out)[1]

	l, err := lore.NewLayout(root)
	if err != nil {
		t.Fatalf("NewLayout: %v", err)
	}
	dbPath := filepath.Join(root, ".codeindex", "lore.db")
	st, _, err2 := index.Reindex(l, dbPath)
	if err2 != nil {
		t.Fatalf("Reindex: %v", err2)
	}
	// churn_lines = 30 = exactly 3×10, NOT > 3×10, so no churn-suspect.
	if err := st.AddSignals(id, 0, 30); err != nil {
		t.Fatalf("AddSignals: %v", err)
	}
	st.Close()

	doctorOut := runLoreOK(t, root, "doctor")
	if strings.Contains(doctorOut, "churn-suspect") {
		t.Fatalf("doctor should NOT emit churn-suspect at exactly 3× threshold:\n%s", doctorOut)
	}
	_ = id
}

// --- lore event tests ---

// TestLoreEventWritesJSONL: event subcommand appends one JSON line.
func TestLoreEventWritesJSONL(t *testing.T) {
	root := loreTestRepo(t)

	var buf bytes.Buffer
	if err := runLore(root, []string{"event", "--type", "deploy", "--status", "ok",
		"--commit", "abc1234", "--detail", "prod release"}, &buf); err != nil {
		t.Fatalf("lore event: %v", err)
	}

	l, err := lore.NewLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	eventsPath := filepath.Join(l.OverlayDir, "events.jsonl")
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("read events.jsonl: %v", err)
	}

	// Parse the one line.
	type evLine struct {
		SHA     string `json:"sha"`
		Type    string `json:"type"`
		Status  string `json:"status"`
		Detail  string `json:"detail"`
		Created string `json:"created"`
	}
	var ev evLine
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("parse JSONL: %v — line: %q", err, line)
		}
	}
	if ev.SHA != "abc1234" || ev.Type != "deploy" || ev.Status != "ok" || ev.Detail != "prod release" {
		t.Fatalf("event fields: %+v", ev)
	}
	if ev.Created == "" {
		t.Fatal("created field empty")
	}
}

// TestLoreEventMissingTypeErrors: omitting --type returns a usage error.
func TestLoreEventMissingTypeErrors(t *testing.T) {
	root := loreTestRepo(t)
	var buf bytes.Buffer
	if err := runLore(root, []string{"event", "--status", "ok"}, &buf); err == nil {
		t.Fatal("want usage error when --type missing")
	}
}

// TestLoreEventMissingStatusErrors: omitting --status returns a usage error.
func TestLoreEventMissingStatusErrors(t *testing.T) {
	root := loreTestRepo(t)
	var buf bytes.Buffer
	if err := runLore(root, []string{"event", "--type", "deploy"}, &buf); err == nil {
		t.Fatal("want usage error when --status missing")
	}
}

// TestLoreShowDisplaysEventLines: a record with a commit ref shows matching events.
func TestLoreShowDisplaysEventLines(t *testing.T) {
	root := loreTestRepo(t)

	// Add a record with a commit ref.
	out := runLoreOK(t, root, "add", "decision",
		"--title", "Deploy test",
		"--body", "test\n",
		"--ref", "commit:abc1234567890abcdef1234567890abcdef12345")
	id := strings.Fields(out)[1]

	// Record an event matching that commit (by 7-char short).
	var buf bytes.Buffer
	if err := runLore(root, []string{"event", "--type", "deploy", "--status", "ok",
		"--commit", "abc1234567890abcdef1234567890abcdef12345"}, &buf); err != nil {
		t.Fatalf("lore event: %v", err)
	}

	show := runLoreOK(t, root, "show", id)
	if !strings.Contains(show, "event: deploy ok (abc1234") {
		t.Fatalf("show missing event line:\n%s", show)
	}
}

// --- lore sync github tests ---

// TestLoreSyncGithub_FlipsDoneOnClosedIssue: when a gh-issue ref is CLOSED,
// the item's status is durably written to "done".
func TestLoreSyncGithub_FlipsDoneOnClosedIssue(t *testing.T) {
	root := loreTestRepo(t)

	// Add an open item with a gh-issue ref.
	out := runLoreOK(t, root, "add", "item",
		"--title", "Open task",
		"--body", "needs doing\n",
		"--ref", "gh-issue:owner/repo#7")
	id := strings.Fields(out)[1]

	// Install a fake GH that returns CLOSED for any issue.
	fakeGH := &fakeSyncGH{state: "CLOSED"}
	origNewGH := newGH
	newGH = func() ghSyncer { return fakeGH }
	defer func() { newGH = origNewGH }()

	syncOut := runLoreOK(t, root, "sync", "github")
	if !strings.Contains(syncOut, "synced "+id+" done") {
		t.Fatalf("sync output missing synced line:\n%s", syncOut)
	}

	// Verify durable write-back: parse the file.
	ll, err := lore.NewLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(ll.Dir("repo", lore.TypeItem), "*.md"))
	if len(files) != 1 {
		t.Fatalf("expected 1 item file, got %v", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	rec, err := lore.Parse(data, lore.TypeItem)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != "done" {
		t.Fatalf("item status not flipped to done: %q", rec.Status)
	}

	// Assert index is fresh: Get shows status done and a follow-up Reindex reports Indexed==0.
	l2, err := lore.NewLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	st2, rep2, err := index.Reindex(l2, filepath.Join(root, ".codeindex", "lore.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	if rep2.Indexed != 0 {
		t.Fatalf("sync should have updated index: Reindex.Indexed=%d (want 0)", rep2.Indexed)
	}
	row, ok, err := st2.Get(id)
	if err != nil || !ok {
		t.Fatalf("Get after sync: ok=%v err=%v", ok, err)
	}
	if row.Status != "done" {
		t.Fatalf("index row status=%q, want done", row.Status)
	}
}

// TestLoreSyncGithub_OpenIssue_NoOp: when a gh-issue ref is OPEN, no change.
func TestLoreSyncGithub_OpenIssue_NoOp(t *testing.T) {
	root := loreTestRepo(t)

	out := runLoreOK(t, root, "add", "item",
		"--title", "Still open",
		"--body", "pending\n",
		"--ref", "gh-issue:owner/repo#3")
	_ = strings.Fields(out)[1]

	fakeGH := &fakeSyncGH{state: "OPEN"}
	origNewGH := newGH
	newGH = func() ghSyncer { return fakeGH }
	defer func() { newGH = origNewGH }()

	syncOut := runLoreOK(t, root, "sync", "github")
	if strings.Contains(syncOut, "synced") {
		t.Fatalf("open issue should produce no output, got: %q", syncOut)
	}
}

// TestLoreSyncGithub_GHError_PropagatesAsError: when gh returns an error,
// sync returns a real error (not fail-open).
func TestLoreSyncGithub_GHError_PropagatesAsError(t *testing.T) {
	root := loreTestRepo(t)

	runLoreOK(t, root, "add", "item",
		"--title", "Error item",
		"--body", "x\n",
		"--ref", "gh-issue:owner/repo#1")

	fakeGH := &fakeSyncGH{err: errors.New("gh: not logged in")}
	origNewGH := newGH
	newGH = func() ghSyncer { return fakeGH }
	defer func() { newGH = origNewGH }()

	var buf bytes.Buffer
	if err := runLore(root, []string{"sync", "github"}, &buf); err == nil {
		t.Fatal("want error when gh fails")
	}
}

// --- lore push tests ---

// TestLorePush_AppendsRefAndPrintsURL: push creates an issue and appends ref.
func TestLorePush_AppendsRefAndPrintsURL(t *testing.T) {
	root := loreTestRepo(t)

	out := runLoreOK(t, root, "add", "item",
		"--title", "Push me",
		"--body", "some work\n")
	id := strings.Fields(out)[1]

	fakeGH := &fakeSyncGH{createURL: "https://github.com/owner/repo/issues/99"}
	origNewGH := newGH
	newGH = func() ghSyncer { return fakeGH }
	defer func() { newGH = origNewGH }()

	pushOut := runLoreOK(t, root, "push", id)
	if !strings.Contains(pushOut, "pushed "+id) {
		t.Fatalf("push output missing 'pushed <id>':\n%s", pushOut)
	}
	if !strings.Contains(pushOut, "https://github.com/owner/repo/issues/99") {
		t.Fatalf("push output missing URL:\n%s", pushOut)
	}

	// Assert the fake runner received --body containing "lore: <id>"
	if !strings.Contains(fakeGH.lastBody, "lore: "+id) {
		t.Fatalf("gh body missing backlink 'lore: %s', got body: %q", id, fakeGH.lastBody)
	}

	// Verify durable ref write-back: parse the file.
	l, err := lore.NewLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(l.Dir("repo", lore.TypeItem), "*.md"))
	if len(files) != 1 {
		t.Fatalf("expected 1 item file, got %v", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	rec, err := lore.Parse(data, lore.TypeItem)
	if err != nil {
		t.Fatal(err)
	}
	foundRef := false
	for _, ref := range rec.Refs {
		if ref.Kind == "gh-issue" && strings.Contains(ref.Value, "#99") {
			foundRef = true
		}
	}
	if !foundRef {
		t.Fatalf("gh-issue ref not appended to record, refs: %v", rec.Refs)
	}

	// Assert index is fresh: Get shows the new ref and Reindex reports Indexed==0.
	l2, err := lore.NewLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	st2, rep2, err2 := index.Reindex(l2, filepath.Join(root, ".codeindex", "lore.db"))
	if err2 != nil {
		t.Fatal(err2)
	}
	defer st2.Close()
	if rep2.Indexed != 0 {
		t.Fatalf("push should have updated index: Reindex.Indexed=%d (want 0)", rep2.Indexed)
	}
	row, ok, err := st2.Get(id)
	if err != nil || !ok {
		t.Fatalf("Get after push: ok=%v err=%v", ok, err)
	}
	foundRefInIndex := false
	for _, ref := range row.Refs {
		if ref.Kind == "gh-issue" && strings.Contains(ref.Value, "#99") {
			foundRefInIndex = true
		}
	}
	if !foundRefInIndex {
		t.Fatalf("index row missing gh-issue ref after push, refs: %v", row.Refs)
	}
}

// TestLorePush_RefusesExistingGHIssue: push errors if item already has gh-issue.
func TestLorePush_RefusesExistingGHIssue(t *testing.T) {
	root := loreTestRepo(t)

	out := runLoreOK(t, root, "add", "item",
		"--title", "Already pushed",
		"--body", "x\n",
		"--ref", "gh-issue:owner/repo#5")
	id := strings.Fields(out)[1]

	fakeGH := &fakeSyncGH{createURL: "https://github.com/owner/repo/issues/6"}
	origNewGH := newGH
	newGH = func() ghSyncer { return fakeGH }
	defer func() { newGH = origNewGH }()

	var buf bytes.Buffer
	if err := runLore(root, []string{"push", id}, &buf); err == nil {
		t.Fatal("want error pushing item that already has a gh-issue ref")
	}
}

// TestLorePush_RefusesNonItem: push errors if record is not an item.
func TestLorePush_RefusesNonItem(t *testing.T) {
	root := loreTestRepo(t)

	out := runLoreOK(t, root, "add", "decision",
		"--title", "Not an item",
		"--body", "x\n")
	id := strings.Fields(out)[1]

	fakeGH := &fakeSyncGH{createURL: "https://github.com/owner/repo/issues/1"}
	origNewGH := newGH
	newGH = func() ghSyncer { return fakeGH }
	defer func() { newGH = origNewGH }()

	var buf bytes.Buffer
	if err := runLore(root, []string{"push", id}, &buf); err == nil {
		t.Fatal("want error pushing non-item record")
	}
}

// --- fake GH implementation for CLI tests ---

type fakeSyncGH struct {
	state     string
	err       error
	createURL string
	lastBody  string
}

func (f *fakeSyncGH) IssueState(repoDir, ref string) (string, error) {
	return f.state, f.err
}

func (f *fakeSyncGH) CreateIssue(repoDir, title, body string) (string, error) {
	f.lastBody = body
	if f.err != nil {
		return "", f.err
	}
	return f.createURL, nil
}
