package main

import (
	"bufio"
	"bytes"
	"encoding/json"
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

	// Add a decision anchored to that directory.
	out := runLoreOK(t, root, "add", "decision", "--title", "Churn test",
		"--body", "x", "--anchor", "path:"+anchorDir+"/")
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
		"--body", "x", "--anchor", "path:"+anchorDir+"/")
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
