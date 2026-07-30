package index

import (
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
