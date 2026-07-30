package index

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeindex/internal/lore"
	"codeindex/internal/lore/gitinfo"
)

func writeRec(t *testing.T, dir, name, id, title string, typ lore.Type) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	r := lore.Record{ID: id, Type: typ, Title: title, Date: "2026-07-29",
		Status: lore.DefaultStatus(typ), Body: "b\n"}
	b, err := r.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func testLayout(t *testing.T) lore.Layout {
	t.Helper()
	t.Setenv("CODEINDEX_HOME", t.TempDir())
	l, err := lore.NewLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestReindexAddChangeRemove(t *testing.T) {
	l := testLayout(t)
	db := filepath.Join(t.TempDir(), "lore.db")
	p := writeRec(t, l.Dir("repo", lore.TypeDecision), "a.md", "dec-A", "First", lore.TypeDecision)
	writeRec(t, l.Dir("overlay", lore.TypeNote), "n.md", "note-N", "Private", lore.TypeNote)

	s, rep, err := Reindex(l, db)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Indexed != 2 || len(rep.Errors) != 0 {
		t.Fatalf("report %+v", rep)
	}
	all, _ := s.All()
	if len(all) != 2 {
		t.Fatalf("n=%d", len(all))
	}
	s.Close()

	// Unchanged files are not re-parsed (Indexed == 0 on second run).
	s, rep, err = Reindex(l, db)
	if err != nil || rep.Indexed != 0 {
		t.Fatalf("second run: %+v %v", rep, err)
	}
	s.Close()

	// Change + remove are picked up.
	writeRec(t, l.Dir("repo", lore.TypeDecision), "a.md", "dec-A", "Renamed", lore.TypeDecision)
	if err := os.Remove(filepath.Join(l.Dir("overlay", lore.TypeNote), "n.md")); err != nil {
		t.Fatal(err)
	}
	s, rep, err = Reindex(l, db)
	if err != nil || rep.Indexed != 1 || rep.Removed != 1 {
		t.Fatalf("third run: %+v %v", rep, err)
	}
	defer s.Close()
	got, ok, _ := s.Get("dec-A")
	if !ok || got.Title != "Renamed" || got.File != p {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
	if _, ok, _ := s.Get("note-N"); ok {
		t.Fatal("removed record still indexed")
	}
}

func TestReindexFailOpenOnMalformed(t *testing.T) {
	l := testLayout(t)
	dir := l.Dir("repo", lore.TypeNote)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.md"), []byte("no frontmatter"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRec(t, dir, "good.md", "note-G", "Good", lore.TypeNote)

	s, rep, err := Reindex(l, filepath.Join(t.TempDir(), "lore.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if len(rep.Errors) != 1 || rep.Indexed != 1 {
		t.Fatalf("report %+v", rep)
	}
}

func TestSessionsIndexAsSessionLayer(t *testing.T) {
	l := testLayout(t)
	writeRec(t, l.SessionsDir(), "s.md", "note-S", "Session note", lore.TypeNote)
	s, _, err := Reindex(l, filepath.Join(t.TempDir(), "lore.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, ok, _ := s.Get("note-S")
	if !ok || got.Layer != "session" {
		t.Fatalf("session layer: %+v ok=%v", got, ok)
	}
}

// TestReindexDuplicateIDReported checks that when the same ID exists in two
// different layer files, Report.Duplicates has exactly one entry naming both
// paths, and the last-upserted (overlay) file wins the index row.
func TestReindexDuplicateIDReported(t *testing.T) {
	l := testLayout(t)
	db := filepath.Join(t.TempDir(), "lore.db")
	const dupID = "dec-DUP"
	repoPath := writeRec(t, l.Dir("repo", lore.TypeDecision), "dup.md",
		dupID, "Repo copy", lore.TypeDecision)
	overlayPath := writeRec(t, l.Dir("overlay", lore.TypeDecision), "dup.md",
		dupID, "Overlay copy", lore.TypeDecision)

	s, rep, err := Reindex(l, db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Exactly one duplicate entry.
	if len(rep.Duplicates) != 1 {
		t.Fatalf("want 1 duplicate, got %d: %v", len(rep.Duplicates), rep.Duplicates)
	}
	entry := rep.Duplicates[0]
	if !strings.Contains(entry, dupID) {
		t.Errorf("duplicate entry missing id %q: %q", dupID, entry)
	}
	if !strings.Contains(entry, repoPath) {
		t.Errorf("duplicate entry missing repo path %q: %q", repoPath, entry)
	}
	if !strings.Contains(entry, overlayPath) {
		t.Errorf("duplicate entry missing overlay path %q: %q", overlayPath, entry)
	}

	// Last-writer-wins: overlay is processed after repo, so it should win.
	got, ok, _ := s.Get(dupID)
	if !ok {
		t.Fatal("duplicate id not in index at all")
	}
	if got.File != overlayPath {
		t.Errorf("expected overlay copy to win, got file=%q", got.File)
	}
}

// TestReindexDuplicateIDThreeWay checks that three files sharing one ID yield
// a single Duplicates entry naming all three paths.
func TestReindexDuplicateIDThreeWay(t *testing.T) {
	l := testLayout(t)
	const dupID = "dec-TRI"
	p1 := writeRec(t, l.Dir("repo", lore.TypeDecision), "a.md", dupID, "A", lore.TypeDecision)
	p2 := writeRec(t, l.Dir("repo", lore.TypeDecision), "b.md", dupID, "B", lore.TypeDecision)
	p3 := writeRec(t, l.Dir("overlay", lore.TypeDecision), "c.md", dupID, "C", lore.TypeDecision)

	s, rep, err := Reindex(l, filepath.Join(t.TempDir(), "lore.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if len(rep.Duplicates) != 1 {
		t.Fatalf("want 1 duplicate entry, got %d: %v", len(rep.Duplicates), rep.Duplicates)
	}
	for _, p := range []string{p1, p2, p3} {
		if !strings.Contains(rep.Duplicates[0], p) {
			t.Errorf("entry missing path %q: %q", p, rep.Duplicates[0])
		}
	}
}

// TestReindexDuplicateIDOnSecondRunUnchanged checks that unchanged files still
// surface duplicates on a second reindex run: all files are parsed for ID
// tracking even when their hash is unchanged.
func TestReindexDuplicateIDOnSecondRunUnchanged(t *testing.T) {
	l := testLayout(t)
	db := filepath.Join(t.TempDir(), "lore.db")
	const dupID = "dec-DUP2"
	writeRec(t, l.Dir("repo", lore.TypeDecision), "dup2.md",
		dupID, "Repo copy", lore.TypeDecision)
	writeRec(t, l.Dir("overlay", lore.TypeDecision), "dup2.md",
		dupID, "Overlay copy", lore.TypeDecision)

	// First run populates the DB.
	s, _, err := Reindex(l, db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Second run: nothing changed on disk, but duplicates must still be reported.
	s2, rep2, err := Reindex(l, db)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	if len(rep2.Duplicates) != 1 {
		t.Fatalf("second run: want 1 duplicate, got %d: %v", len(rep2.Duplicates), rep2.Duplicates)
	}
}

// --- Ratification tests ---

// makeGitDotDir creates a fake .git directory so gitinfo.Available() returns true.
func makeGitDotDir(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestRatificationRepoLayerAbsentOnBranch: a repo-layer record whose file is
// not on origin/main → Ratified == false after Reindex.
func TestRatificationRepoLayerAbsentOnBranch(t *testing.T) {
	l := testLayout(t)
	db := filepath.Join(t.TempDir(), "lore.db")
	makeGitDotDir(t, l.RepoRoot)

	writeRec(t, l.Dir("repo", lore.TypeDecision), "a.md", "dec-A", "A", lore.TypeDecision)

	// Fake runner: origin/HEAD ref exists, but cat-file returns error (file absent on branch).
	orig := newGit
	t.Cleanup(func() { newGit = orig })
	newGit = func(root string) *gitinfo.Git {
		return gitinfo.NewWithRunner(root, func(dir string, args ...string) (string, error) {
			if len(args) == 0 {
				return "", nil
			}
			switch args[0] {
			case "rev-parse":
				// HasRef("refs/remotes/origin/HEAD") succeeds → origin exists.
				return "", nil
			case "symbolic-ref":
				return "refs/remotes/origin/main\n", nil
			case "cat-file":
				// File absent on branch → error.
				return "", os.ErrNotExist
			}
			return "", nil
		})
	}

	s, _, err := Reindex(l, db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	r, ok, err := s.Get("dec-A")
	if err != nil || !ok {
		t.Fatalf("get dec-A: %v ok=%v", err, ok)
	}
	if r.Ratified {
		t.Fatal("repo-layer record absent on branch must be Ratified=false")
	}
}

// TestRatificationRepoLayerPresentOnBranch: a repo-layer record whose file IS
// on origin/main → Ratified == true.
func TestRatificationRepoLayerPresentOnBranch(t *testing.T) {
	l := testLayout(t)
	db := filepath.Join(t.TempDir(), "lore.db")
	makeGitDotDir(t, l.RepoRoot)

	writeRec(t, l.Dir("repo", lore.TypeDecision), "b.md", "dec-B", "B", lore.TypeDecision)

	orig := newGit
	t.Cleanup(func() { newGit = orig })
	newGit = func(root string) *gitinfo.Git {
		return gitinfo.NewWithRunner(root, func(dir string, args ...string) (string, error) {
			if len(args) == 0 {
				return "", nil
			}
			switch args[0] {
			case "rev-parse":
				return "", nil // HasRef succeeds
			case "symbolic-ref":
				return "refs/remotes/origin/main\n", nil
			case "cat-file":
				return "", nil // file present on branch → no error
			}
			return "", nil
		})
	}

	s, _, err := Reindex(l, db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	r, ok, err := s.Get("dec-B")
	if err != nil || !ok {
		t.Fatalf("get dec-B: %v ok=%v", err, ok)
	}
	if !r.Ratified {
		t.Fatal("repo-layer record present on branch must be Ratified=true")
	}
}

// TestRatificationOverlayAlwaysRatified: an overlay record is always Ratified
// regardless of git state.
func TestRatificationOverlayAlwaysRatified(t *testing.T) {
	l := testLayout(t)
	db := filepath.Join(t.TempDir(), "lore.db")
	makeGitDotDir(t, l.RepoRoot)

	writeRec(t, l.Dir("overlay", lore.TypeNote), "o.md", "note-O", "Overlay", lore.TypeNote)

	orig := newGit
	t.Cleanup(func() { newGit = orig })
	newGit = func(root string) *gitinfo.Git {
		return gitinfo.NewWithRunner(root, func(dir string, args ...string) (string, error) {
			if len(args) == 0 {
				return "", nil
			}
			switch args[0] {
			case "rev-parse":
				return "", nil
			case "symbolic-ref":
				return "refs/remotes/origin/main\n", nil
			case "cat-file":
				return "", os.ErrNotExist // absent on branch
			}
			return "", nil
		})
	}

	s, _, err := Reindex(l, db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	r, ok, err := s.Get("note-O")
	if err != nil || !ok {
		t.Fatalf("get note-O: %v ok=%v", err, ok)
	}
	if !r.Ratified {
		t.Fatal("overlay record must always be Ratified=true")
	}
}

// TestRatificationNoOriginRefKeepsEverythingRatified: when there is no
// origin ref (HasRef returns false), no ratification pass runs and all
// records stay at their default Ratified=true.
func TestRatificationNoOriginRefKeepsEverythingRatified(t *testing.T) {
	l := testLayout(t)
	db := filepath.Join(t.TempDir(), "lore.db")
	makeGitDotDir(t, l.RepoRoot)

	writeRec(t, l.Dir("repo", lore.TypeDecision), "c.md", "dec-C", "C", lore.TypeDecision)

	orig := newGit
	t.Cleanup(func() { newGit = orig })
	newGit = func(root string) *gitinfo.Git {
		return gitinfo.NewWithRunner(root, func(dir string, args ...string) (string, error) {
			if len(args) == 0 {
				return "", nil
			}
			// rev-parse fails → HasRef returns false → no origin → skip ratification.
			return "", os.ErrNotExist
		})
	}

	s, _, err := Reindex(l, db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	r, ok, err := s.Get("dec-C")
	if err != nil || !ok {
		t.Fatalf("get dec-C: %v ok=%v", err, ok)
	}
	if !r.Ratified {
		t.Fatal("no-origin guard: repo record must stay Ratified=true when origin ref absent")
	}
}

// TestUpsertPreservesRatified: re-upserting a record must not reset Ratified
// (it is derived state like stale/confidence).
func TestUpsertPreservesRatified(t *testing.T) {
	s := openStore(t)
	r := lore.Record{ID: "dec-R", Type: lore.TypeDecision, Title: "t", Date: "2026-07-29"}
	if err := s.Upsert(r, "repo", "/r.md"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRatified("dec-R", false); err != nil {
		t.Fatal(err)
	}
	// Re-upsert must not reset ratified back to true.
	if err := s.Upsert(r, "repo", "/r.md"); err != nil {
		t.Fatal(err)
	}
	got, _, _ := s.Get("dec-R")
	if got.Ratified {
		t.Fatal("re-upsert must not reset ratified; it is derived state")
	}
}

// --- Closes-transition tests ---

// fakeHeadSHA is the full SHA returned by the fake git runner as HEAD.
const fakeHeadSHA = "abc1234567890000000000000000000000000000"

// writeOpenItem writes an open item record file in the repo items dir and
// returns its path.
func writeOpenItem(t *testing.T, l lore.Layout, filename, id, title string) string {
	t.Helper()
	dir := l.Dir("repo", lore.TypeItem)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	r := lore.Record{
		ID:     id,
		Type:   lore.TypeItem,
		Title:  title,
		Date:   "2026-07-29",
		Status: "open",
		Body:   "item body\n",
	}
	b, err := r.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, filename)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// makeClosesRunner returns a fake git runner that:
// - Available: .git dir must exist in root (checked by Available() itself)
// - Head() succeeds with fakeHeadSHA
// - CommitsSince(sinceSHA, 500) returns commits built from commitSubjects
//   (sinceSHA is captured into receivedSince for assertion)
// - ratification HasRef returns false (no origin), so ratification is skipped.
func makeClosesRunner(commitSubjects []string, receivedSince *string) func(string) *gitinfo.Git {
	return func(root string) *gitinfo.Git {
		return gitinfo.NewWithRunner(root, func(dir string, args ...string) (string, error) {
			if len(args) == 0 {
				return "", nil
			}
			switch args[0] {
			case "rev-parse":
				// Head() call: rev-parse HEAD → return fake SHA
				// HasRef call: rev-parse --verify --quiet <ref> → fail (no origin)
				for _, a := range args {
					if a == "--verify" {
						return "", os.ErrNotExist // HasRef → false → skip ratification
					}
				}
				return fakeHeadSHA + "\n", nil
			case "log":
				// CommitsSince call. Capture the since SHA from the range arg.
				if receivedSince != nil {
					for _, a := range args {
						if strings.Contains(a, "..HEAD") {
							*receivedSince = strings.TrimSuffix(a, "..HEAD")
							break
						}
					}
				}
				// Build fake log output in numstat format.
				// Format: <sha>\x00<subject>\n\n
				var sb strings.Builder
				for i, subj := range commitSubjects {
					sha := fakeHeadSHA[:40]
					// Give each commit a distinct SHA based on index.
					sha = strings.Repeat("0", 39-i%39) + strings.Repeat("a", 1+i%39)
					if len(sha) > 40 {
						sha = sha[:40]
					}
					sb.WriteString(sha + "\x00" + subj + "\n\n")
				}
				return sb.String(), nil
			}
			return "", nil
		})
	}
}

// TestClosesTransitionOpenItemFlipsToDone: a commit with subject
// "closes <item-id>" causes the item file on disk to flip to status: done,
// appends a commit ref, updates the index row, advances last_scanned_commit,
// and populates Report.Closed.
func TestClosesTransitionOpenItemFlipsToDone(t *testing.T) {
	l := testLayout(t)
	makeGitDotDir(t, l.RepoRoot)
	db := filepath.Join(t.TempDir(), "lore.db")

	const itemID = "itm-01AN4Z07BY79KA1307SR9X4MV4"
	itemPath := writeOpenItem(t, l, "item.md", itemID, "My item")

	orig := newGit
	t.Cleanup(func() { newGit = orig })
	newGit = makeClosesRunner([]string{"closes " + itemID}, nil)

	s, rep, err := Reindex(l, db)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	defer s.Close()

	// Report.Closed must contain the item ID.
	if len(rep.Closed) != 1 || rep.Closed[0] != itemID {
		t.Fatalf("Report.Closed: %v", rep.Closed)
	}

	// File on disk must now have status: done.
	data, err := os.ReadFile(itemPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	parsed, err := lore.Parse(data, lore.TypeItem)
	if err != nil {
		t.Fatalf("Parse rewritten file: %v", err)
	}
	if parsed.Status != "done" {
		t.Fatalf("status on disk: want done, got %q", parsed.Status)
	}

	// A commit ref must have been appended.
	var hasCommitRef bool
	for _, ref := range parsed.Refs {
		if ref.Kind == "commit" {
			hasCommitRef = true
		}
	}
	if !hasCommitRef {
		t.Fatalf("no commit ref appended; refs=%v", parsed.Refs)
	}

	// Index row must also show done.
	row, ok, err := s.Get(itemID)
	if err != nil || !ok {
		t.Fatalf("Get %s: %v ok=%v", itemID, err, ok)
	}
	if row.Status != "done" {
		t.Fatalf("index status: want done, got %q", row.Status)
	}

	// last_scanned_commit must be set to HEAD.
	meta, err := s.Meta("last_scanned_commit")
	if err != nil {
		t.Fatalf("Meta: %v", err)
	}
	if meta == "" {
		t.Fatal("last_scanned_commit not set after Reindex")
	}
}

// TestClosesTransitionSecondReindexNoNewCommits: second Reindex with no new
// commits (CommitsSince called with stored SHA) causes no rewrites.
func TestClosesTransitionSecondReindexNoNewCommits(t *testing.T) {
	l := testLayout(t)
	makeGitDotDir(t, l.RepoRoot)
	db := filepath.Join(t.TempDir(), "lore.db")

	const itemID = "itm-01AN4Z07BY79KA1307SR9X4MV4"
	writeOpenItem(t, l, "item.md", itemID, "My item")

	orig := newGit
	t.Cleanup(func() { newGit = orig })

	// First run: commit closes item.
	newGit = makeClosesRunner([]string{"closes " + itemID}, nil)
	s, rep, err := Reindex(l, db)
	if err != nil {
		t.Fatalf("first Reindex: %v", err)
	}
	if len(rep.Closed) != 1 {
		t.Fatalf("first run: want 1 closed, got %v", rep.Closed)
	}
	storedSince, _ := s.Meta("last_scanned_commit")
	s.Close()

	// Read file content after first reindex (it has been rewritten to status: done).
	itemPath := filepath.Join(l.Dir("repo", lore.TypeItem), "item.md")
	data1, err := os.ReadFile(itemPath)
	if err != nil {
		t.Fatalf("ReadFile after first run: %v", err)
	}

	// Second run: no new commits, runner returns empty. Capture since arg.
	var receivedSince string
	newGit = makeClosesRunner(nil, &receivedSince)
	s2, rep2, err := Reindex(l, db)
	if err != nil {
		t.Fatalf("second Reindex: %v", err)
	}
	defer s2.Close()

	// Must have passed the stored SHA to CommitsSince.
	if receivedSince != storedSince {
		t.Fatalf("CommitsSince since arg: want %q, got %q", storedSince, receivedSince)
	}

	// No rewrites: Report.Closed empty.
	if len(rep2.Closed) != 0 {
		t.Fatalf("second run: Report.Closed should be empty, got %v", rep2.Closed)
	}

	// File content unchanged (content-based check, more reliable than mtime on
	// filesystems with coarse time resolution or after a copy-on-write COW).
	data2, err := os.ReadFile(itemPath)
	if err != nil {
		t.Fatalf("ReadFile after second run: %v", err)
	}
	if string(data1) != string(data2) {
		t.Fatal("second run rewrote item file; it must be unchanged when no new commits")
	}
}

// TestClosesTransitionNonOpenOrMissingSkipped: closing a decision ID or an
// already-done item leaves the records untouched.
func TestClosesTransitionNonOpenOrMissingSkipped(t *testing.T) {
	l := testLayout(t)
	makeGitDotDir(t, l.RepoRoot)
	db := filepath.Join(t.TempDir(), "lore.db")

	// Write a decision with ID prefix "dec-".
	const decID = "dec-01AN4Z07BY79KA1307SR9X4MV4"
	decDir := l.Dir("repo", lore.TypeDecision)
	if err := os.MkdirAll(decDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dec := lore.Record{ID: decID, Type: lore.TypeDecision, Title: "Decision",
		Date: "2026-07-29", Status: "active", Body: "b\n"}
	decBytes, _ := dec.Marshal()
	if err := os.WriteFile(filepath.Join(decDir, "dec.md"), decBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	// Write an already-done item.
	const doneItemID = "itm-01AN4Z07BY79KA1307SR9X4MV5"
	itemDir := l.Dir("repo", lore.TypeItem)
	if err := os.MkdirAll(itemDir, 0o755); err != nil {
		t.Fatal(err)
	}
	doneItem := lore.Record{ID: doneItemID, Type: lore.TypeItem, Title: "Done item",
		Date: "2026-07-29", Status: "done", Body: "b\n"}
	doneBytes, _ := doneItem.Marshal()
	if err := os.WriteFile(filepath.Join(itemDir, "done.md"), doneBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	orig := newGit
	t.Cleanup(func() { newGit = orig })

	// Commit closes: decision ID, already-done item, and unknown ID.
	subjects := []string{
		"closes " + decID,
		"closes " + doneItemID,
		"closes itm-99999999999999999999999999",
	}
	newGit = makeClosesRunner(subjects, nil)

	s, rep, err := Reindex(l, db)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	defer s.Close()

	// None should appear in Report.Closed.
	if len(rep.Closed) != 0 {
		t.Fatalf("want no closed items, got %v", rep.Closed)
	}

	// Decision status unchanged.
	dr, ok, _ := s.Get(decID)
	if !ok || dr.Status != "active" {
		t.Fatalf("decision status changed: ok=%v status=%q", ok, dr.Status)
	}

	// Already-done item status unchanged.
	ir, ok, _ := s.Get(doneItemID)
	if !ok || ir.Status != "done" {
		t.Fatalf("done item status changed: ok=%v status=%q", ok, ir.Status)
	}
}

// TestClosesTransitionTwoItemsOneCommit: one commit subject matching two item
// IDs causes both items to flip to done.
func TestClosesTransitionTwoItemsOneCommit(t *testing.T) {
	l := testLayout(t)
	makeGitDotDir(t, l.RepoRoot)
	db := filepath.Join(t.TempDir(), "lore.db")

	const itemID1 = "itm-01AN4Z07BY79KA1307SR9X4MV4"
	const itemID2 = "itm-01AN4Z07BY79KA1307SR9X4MV6"
	writeOpenItem(t, l, "item1.md", itemID1, "Item one")
	writeOpenItem(t, l, "item2.md", itemID2, "Item two")

	orig := newGit
	t.Cleanup(func() { newGit = orig })
	// Subject closes both IDs.
	newGit = makeClosesRunner([]string{"closes " + itemID1 + " closes " + itemID2}, nil)

	s, rep, err := Reindex(l, db)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	defer s.Close()

	if len(rep.Closed) != 2 {
		t.Fatalf("want 2 closed, got %v", rep.Closed)
	}
	for _, id := range []string{itemID1, itemID2} {
		row, ok, _ := s.Get(id)
		if !ok || row.Status != "done" {
			t.Fatalf("item %s: ok=%v status=%q", id, ok, row.Status)
		}
	}
}

// --- Survival / churn signals tests ---

// survivalCommit describes a fake git commit for survival signal tests.
type survivalCommit struct {
	subject string
	files   map[string][2]int // path → [added, deleted]
}

// makeSurvivalRunner returns a fake git runner that emits structured commits
// with numstat file data. It counts how many times CommitsSince (log) is called
// so tests can assert exactly-one-call semantics.
func makeSurvivalRunner(commits []survivalCommit, logCallCount *int) func(string) *gitinfo.Git {
	return func(root string) *gitinfo.Git {
		return gitinfo.NewWithRunner(root, func(dir string, args ...string) (string, error) {
			if len(args) == 0 {
				return "", nil
			}
			switch args[0] {
			case "rev-parse":
				for _, a := range args {
					if a == "--verify" {
						return "", os.ErrNotExist // HasRef → false → skip ratification
					}
				}
				return fakeHeadSHA + "\n", nil
			case "log":
				if logCallCount != nil {
					*logCallCount++
				}
				var sb strings.Builder
				for i, c := range commits {
					sha := fmt.Sprintf("%040d", i+1)
					sb.WriteString(sha + "\x00" + c.subject + "\n\n")
					for path, stats := range c.files {
						sb.WriteString(fmt.Sprintf("%d\t%d\t%s\n", stats[0], stats[1], path))
					}
					sb.WriteString("\n")
				}
				return sb.String(), nil
			}
			return "", nil
		})
	}
}

// writeRecWithAnchor writes a lore record with a path anchor into the repo layer
// and returns the record path and id.
func writeRecWithAnchor(t *testing.T, l lore.Layout, filename, id, title, anchorPath string) string {
	t.Helper()
	dir := l.Dir("repo", lore.TypeDecision)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	r := lore.Record{
		ID:      id,
		Type:    lore.TypeDecision,
		Title:   title,
		Date:    "2026-07-29",
		Status:  "active",
		Body:    "body\n",
		Anchors: []lore.Anchor{{Path: anchorPath}},
	}
	b, err := r.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, filename)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestSurvivalSignalsAnchoredPathTouched: a commit that touches files under an
// anchored path (but NOT the record's own file) must increment survived by 1
// and accumulate churn lines.
func TestSurvivalSignalsAnchoredPathTouched(t *testing.T) {
	l := testLayout(t)
	makeGitDotDir(t, l.RepoRoot)
	db := filepath.Join(t.TempDir(), "lore.db")

	const recID = "dec-01AN4Z07BY79KA1307SR9X4MV4"
	writeRecWithAnchor(t, l, "rec.md", recID, "Engine decision", "internal/engine/")

	orig := newGit
	t.Cleanup(func() { newGit = orig })

	var logCalls int
	newGit = makeSurvivalRunner([]survivalCommit{
		{
			subject: "refactor engine",
			files: map[string][2]int{
				"internal/engine/core.go": {5, 3}, // 5 added, 3 deleted
			},
		},
	}, &logCalls)

	s, _, err := Reindex(l, db)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	defer s.Close()

	// Exactly one CommitsSince call (shared batch).
	if logCalls != 1 {
		t.Fatalf("CommitsSince called %d times; want exactly 1", logCalls)
	}

	r, ok, err := s.Get(recID)
	if err != nil || !ok {
		t.Fatalf("Get: %v ok=%v", err, ok)
	}
	if r.Survived != 1 {
		t.Fatalf("survived: want 1, got %d", r.Survived)
	}
	if r.ChurnLines != 8 { // 5 added + 3 deleted
		t.Fatalf("churn_lines: want 8, got %d", r.ChurnLines)
	}
	// confidence = ln(2)/ln(21)
	wantConf := math.Log(2) / math.Log(21)
	if math.Abs(r.Confidence-wantConf) > 1e-9 {
		t.Fatalf("confidence: want %.10f, got %.10f", wantConf, r.Confidence)
	}
}

// TestSurvivalSignalsOwnFileTouchedNoIncrement: when a commit touches BOTH the
// anchored path AND the record's own file, survived must NOT increment.
func TestSurvivalSignalsOwnFileTouchedNoIncrement(t *testing.T) {
	l := testLayout(t)
	makeGitDotDir(t, l.RepoRoot)
	db := filepath.Join(t.TempDir(), "lore.db")

	const recID = "dec-01AN4Z07BY79KA1307SR9X4MV5"
	recPath := writeRecWithAnchor(t, l, "rec2.md", recID, "Engine decision 2", "internal/engine/")

	// Make the record's own file relative to repo root.
	recRelPath, _ := filepath.Rel(l.RepoRoot, recPath)

	orig := newGit
	t.Cleanup(func() { newGit = orig })

	newGit = makeSurvivalRunner([]survivalCommit{
		{
			subject: "update record and engine",
			files: map[string][2]int{
				"internal/engine/core.go": {5, 3},
				recRelPath:                {1, 1}, // record's own file also changed
			},
		},
	}, nil)

	s, _, err := Reindex(l, db)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	defer s.Close()

	r, ok, err := s.Get(recID)
	if err != nil || !ok {
		t.Fatalf("Get: %v ok=%v", err, ok)
	}
	if r.Survived != 0 {
		t.Fatalf("survived: want 0 (own file touched), got %d", r.Survived)
	}
	if r.ChurnLines != 0 {
		t.Fatalf("churn_lines: want 0 (own file touched), got %d", r.ChurnLines)
	}
}

// TestSurvivalSignalsNoPathAnchorNoSignals: a record with only a symbol anchor
// must NOT accumulate survival signals.
func TestSurvivalSignalsNoPathAnchorNoSignals(t *testing.T) {
	l := testLayout(t)
	makeGitDotDir(t, l.RepoRoot)
	db := filepath.Join(t.TempDir(), "lore.db")

	const recID = "dec-01AN4Z07BY79KA1307SR9X4MV6"
	dir := l.Dir("repo", lore.TypeDecision)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	r := lore.Record{
		ID:      recID,
		Type:    lore.TypeDecision,
		Title:   "Symbol anchored",
		Date:    "2026-07-29",
		Status:  "active",
		Body:    "body\n",
		Anchors: []lore.Anchor{{Symbol: "MyFunc"}}, // symbol only
	}
	b, _ := r.Marshal()
	os.WriteFile(filepath.Join(dir, "sym.md"), b, 0o644)

	orig := newGit
	t.Cleanup(func() { newGit = orig })

	newGit = makeSurvivalRunner([]survivalCommit{
		{
			subject: "changes near MyFunc",
			files:   map[string][2]int{"internal/engine/core.go": {10, 5}},
		},
	}, nil)

	s, _, err := Reindex(l, db)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	defer s.Close()

	got, ok, _ := s.Get(recID)
	if !ok {
		t.Fatal("record not found")
	}
	if got.Survived != 0 || got.ChurnLines != 0 {
		t.Fatalf("symbol-only anchor should get no signals: survived=%d churn=%d",
			got.Survived, got.ChurnLines)
	}
}

// TestSurvivalSignalsMultipleCommitsAccumulate: two commits touching the anchored
// path (without touching the record's file) must accumulate survived=2 and
// combined churn.
func TestSurvivalSignalsMultipleCommitsAccumulate(t *testing.T) {
	l := testLayout(t)
	makeGitDotDir(t, l.RepoRoot)
	db := filepath.Join(t.TempDir(), "lore.db")

	const recID = "dec-01AN4Z07BY79KA1307SR9X4MV7"
	writeRecWithAnchor(t, l, "rec3.md", recID, "Engine decision 3", "internal/engine/")

	orig := newGit
	t.Cleanup(func() { newGit = orig })

	var logCalls int
	newGit = makeSurvivalRunner([]survivalCommit{
		{
			subject: "first engine change",
			files:   map[string][2]int{"internal/engine/a.go": {4, 2}},
		},
		{
			subject: "second engine change",
			files:   map[string][2]int{"internal/engine/b.go": {7, 3}},
		},
	}, &logCalls)

	s, _, err := Reindex(l, db)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	defer s.Close()

	// Exactly one CommitsSince call.
	if logCalls != 1 {
		t.Fatalf("CommitsSince called %d times; want exactly 1", logCalls)
	}

	r, ok, err := s.Get(recID)
	if err != nil || !ok {
		t.Fatalf("Get: %v ok=%v", err, ok)
	}
	if r.Survived != 2 {
		t.Fatalf("survived: want 2, got %d", r.Survived)
	}
	if r.ChurnLines != 16 { // (4+2)+(7+3) = 6+10 = 16
		t.Fatalf("churn_lines: want 16, got %d", r.ChurnLines)
	}
}
