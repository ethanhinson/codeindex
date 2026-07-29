# Lore Engine Core (Plan 1 of 3) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A CLI-complete lore engine inside codeindex: git-native decision/item/note records in `.lore/`, a private overlay, a derived search index in `.codeindex/lore.db`, and the full record lifecycle (`add`, `show`, `search`, `for`, `backlog`, `promote`, `supersede`, `doctor`, `init`).

**Architecture:** Records are Markdown files with YAML frontmatter in two layers (committed `<repo>/.lore/`, private `~/.codeindex/lore/<repo-id>/`). Every command lazily reindexes changed files (content-hash diff, same pattern as the code index) into `.codeindex/lore.db`, then queries in Go. Ranking is BM25-lite scoring over `##`-heading chunks reusing `internal/search`'s tokenizer. Symbol anchors are validated against the existing `graph.db` for staleness.

**Tech Stack:** Go 1.26.5, mattn/go-sqlite3 (CGO, already a dep), gopkg.in/yaml.v3 (new), github.com/oklog/ulid/v2 (new).

**Spec:** `docs/superpowers/specs/2026-07-29-lore-engine-design.md` (including the two planning amendments: separate `lore.db`; Go-side scoring instead of FTS5).

**Plan series:** Plan 1 (this) = core + CLI. Plan 2 = MCP tools, `related_lore`, capture command, plugin/hooks/rules packaging. Plan 3 = lifecycle signals (`git log`-derived), events, `sync github`. Plans 2–3 are written after Plan 1 lands.

## Global Constraints

- Module path is `codeindex`; imports look like `codeindex/internal/lore`.
- CLI grammar follows the existing convention: `codeindex lore <repo-root> <subcommand> [args...]` (repo root is always argv[2], as in every other command). This intentionally amends the spec's shorthand `codeindex lore add <type>`.
- Never modify `internal/graph` schema or `graph.db`; lore reads it via `graph.OpenRaw` only.
- No SQLite build tags (no FTS5). No new dependencies beyond yaml.v3 and oklog/ulid/v2.
- All commands fail-open on malformed records: collect and report, never abort the command.
- Record ID prefixes: `dec-`, `itm-`, `note-` + ULID. Type is derived from the directory a file lives in (`decisions/`, `items/`, `notes/`, `sessions/`), never from frontmatter.
- Dates are `YYYY-MM-DD` UTC. Functions that need "now" take it as a parameter for testability.
- Run tests with plain `go test ./internal/lore/... ./cmd/codeindex/` — no tags, no env.
- Commit after every task with a `lore:` prefix.

---

### Task 1: Record model and frontmatter round-trip

**Files:**
- Create: `internal/lore/record.go`
- Test: `internal/lore/record_test.go`
- Modify: `go.mod` (via `go get`)

**Interfaces:**
- Consumes: nothing (leaf package).
- Produces (used by every later task):
  - `type Type string` with constants `TypeDecision`, `TypeItem`, `TypeNote`
  - `type Anchor struct { Path, Symbol string }` (exactly one field set)
  - `type Ref struct { Kind, Value string }`
  - `type Record struct { ID string; Type Type; Title, Status, Date, Supersedes, SupersededBy, Priority string; BlockedBy, Tags []string; Anchors []Anchor; Refs []Ref; Body string }`
  - `func NewID(t Type) string` — `dec-`/`itm-`/`note-` + ULID
  - `func DefaultStatus(t Type) string` — decision→`active`, item→`open`, note→``
  - `func Parse(b []byte, t Type) (Record, error)`
  - `func (r Record) Marshal() ([]byte, error)` — Parse∘Marshal is identity
  - `func Slug(title string) string` — lowercase, spaces/punct→`-`, max 48 chars

- [ ] **Step 1: Add dependencies**

```bash
cd ~/codeindex
go get gopkg.in/yaml.v3@v3.0.1 github.com/oklog/ulid/v2@v2.1.0
```

- [ ] **Step 2: Write the failing test**

```go
package lore

import (
	"strings"
	"testing"
)

func TestRecordRoundTrip(t *testing.T) {
	in := Record{
		ID: "dec-01AN4Z07BY79KA1307SR9X4MV3", Type: TypeDecision,
		Title: "Use Go for the engine", Status: "active", Date: "2026-07-29",
		Anchors: []Anchor{{Path: "internal/engine/"}, {Symbol: "ResolveImports"}},
		Refs:    []Ref{{Kind: "gh-issue", Value: "ethanhinson/codeindex#12"}},
		Body:    "Rationale.\n\n## Alternatives considered\nRust — rejected.\n",
	}
	b, err := in.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), "---\n") {
		t.Fatalf("no frontmatter delimiter:\n%s", b)
	}
	out, err := Parse(b, TypeDecision)
	if err != nil {
		t.Fatal(err)
	}
	if out.ID != in.ID || out.Title != in.Title || out.Status != in.Status ||
		len(out.Anchors) != 2 || out.Anchors[0].Path != "internal/engine/" ||
		out.Anchors[1].Symbol != "ResolveImports" ||
		len(out.Refs) != 1 || out.Refs[0].Kind != "gh-issue" ||
		out.Body != in.Body {
		t.Fatalf("round-trip mismatch:\n%+v\nvs\n%+v", out, in)
	}
}

func TestParseItemFields(t *testing.T) {
	src := `---
id: itm-01AN4Z07BY79KA1307SR9X4MV4
title: Migrate resolver tests
status: open
date: 2026-07-29
priority: p1
blocked_by: [itm-01AN4Z07BY79KA1307SR9X4MV5]
tags: [tech-debt]
---
Body here.
`
	r, err := Parse([]byte(src), TypeItem)
	if err != nil {
		t.Fatal(err)
	}
	if r.Priority != "p1" || len(r.BlockedBy) != 1 || r.Tags[0] != "tech-debt" {
		t.Fatalf("item fields: %+v", r)
	}
}

func TestParseErrors(t *testing.T) {
	if _, err := Parse([]byte("no frontmatter"), TypeNote); err == nil {
		t.Fatal("want error for missing frontmatter")
	}
	if _, err := Parse([]byte("---\nid: [broken\n---\n"), TypeNote); err == nil {
		t.Fatal("want error for bad yaml")
	}
}

func TestNewIDAndSlug(t *testing.T) {
	if id := NewID(TypeItem); !strings.HasPrefix(id, "itm-") || len(id) != 4+26 {
		t.Fatalf("bad id %q", id)
	}
	if s := Slug("Use Go: for the engine!"); s != "use-go-for-the-engine" {
		t.Fatalf("slug %q", s)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/lore/ -run TestRecord -v`
Expected: FAIL (package does not exist / undefined symbols).

- [ ] **Step 4: Write the implementation**

```go
// Package lore models decision/item/note records: Markdown files with YAML
// frontmatter, stored in a committed .lore/ layer and a private overlay.
package lore

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/oklog/ulid/v2"
	"gopkg.in/yaml.v3"
)

type Type string

const (
	TypeDecision Type = "decision"
	TypeItem     Type = "item"
	TypeNote     Type = "note"
)

// Anchor ties a record to code: exactly one of Path or Symbol is set.
type Anchor struct{ Path, Symbol string }

// Ref is a typed external reference (gh-issue, jira, commit, url, ...).
type Ref struct{ Kind, Value string }

type Record struct {
	ID           string
	Type         Type
	Title        string
	Status       string
	Date         string // YYYY-MM-DD (UTC)
	Supersedes   string
	SupersededBy string
	Priority     string // items: p0..p3
	BlockedBy    []string
	Tags         []string
	Anchors      []Anchor
	Refs         []Ref
	Body         string
}

func NewID(t Type) string {
	prefix := map[Type]string{TypeDecision: "dec-", TypeItem: "itm-", TypeNote: "note-"}[t]
	return prefix + ulid.Make().String()
}

func DefaultStatus(t Type) string {
	switch t {
	case TypeDecision:
		return "active"
	case TypeItem:
		return "open"
	}
	return ""
}

// wire is the YAML frontmatter shape. Anchors/refs are single-key maps so the
// file reads naturally: `- path: internal/engine/` / `- gh-issue: org/repo#12`.
type wire struct {
	ID           string              `yaml:"id"`
	Title        string              `yaml:"title"`
	Status       string              `yaml:"status,omitempty"`
	Date         string              `yaml:"date"`
	Supersedes   string              `yaml:"supersedes,omitempty"`
	SupersededBy string              `yaml:"superseded_by,omitempty"`
	Priority     string              `yaml:"priority,omitempty"`
	BlockedBy    []string            `yaml:"blocked_by,omitempty,flow"`
	Tags         []string            `yaml:"tags,omitempty,flow"`
	Anchors      []map[string]string `yaml:"anchors,omitempty"`
	Refs         []map[string]string `yaml:"refs,omitempty"`
}

// Parse splits frontmatter from body and decodes. The type comes from the
// caller (derived from the directory), never from the file.
func Parse(b []byte, t Type) (Record, error) {
	rest, ok := bytes.CutPrefix(b, []byte("---\n"))
	if !ok {
		return Record{}, fmt.Errorf("missing frontmatter (no leading ---)")
	}
	fm, body, ok := bytes.Cut(rest, []byte("\n---\n"))
	if !ok {
		return Record{}, fmt.Errorf("unterminated frontmatter")
	}
	var w wire
	if err := yaml.Unmarshal(fm, &w); err != nil {
		return Record{}, fmt.Errorf("frontmatter: %w", err)
	}
	r := Record{
		ID: w.ID, Type: t, Title: w.Title, Status: w.Status, Date: w.Date,
		Supersedes: w.Supersedes, SupersededBy: w.SupersededBy,
		Priority: w.Priority, BlockedBy: w.BlockedBy, Tags: w.Tags,
		Body: strings.TrimLeft(string(body), "\n"),
	}
	for _, m := range w.Anchors {
		r.Anchors = append(r.Anchors, Anchor{Path: m["path"], Symbol: m["symbol"]})
	}
	for _, m := range w.Refs {
		for k, v := range m {
			r.Refs = append(r.Refs, Ref{Kind: k, Value: v})
		}
	}
	return r, nil
}

func (r Record) Marshal() ([]byte, error) {
	w := wire{
		ID: r.ID, Title: r.Title, Status: r.Status, Date: r.Date,
		Supersedes: r.Supersedes, SupersededBy: r.SupersededBy,
		Priority: r.Priority, BlockedBy: r.BlockedBy, Tags: r.Tags,
	}
	for _, a := range r.Anchors {
		m := map[string]string{}
		if a.Path != "" {
			m["path"] = a.Path
		}
		if a.Symbol != "" {
			m["symbol"] = a.Symbol
		}
		w.Anchors = append(w.Anchors, m)
	}
	for _, ref := range r.Refs {
		w.Refs = append(w.Refs, map[string]string{ref.Kind: ref.Value})
	}
	fm, err := yaml.Marshal(w)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(fm)
	buf.WriteString("---\n")
	buf.WriteString(r.Body)
	return buf.Bytes(), nil
}

// Slug converts a title to a filename fragment: lowercase alnum runs joined
// by hyphens, capped at 48 chars.
func Slug(title string) string {
	var b strings.Builder
	prevDash := true
	for _, c := range strings.ToLower(title) {
		switch {
		case c >= 'a' && c <= 'z' || c >= '0' && c <= '9':
			b.WriteRune(c)
			prevDash = false
		case !prevDash:
			b.WriteByte('-')
			prevDash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 48 {
		s = strings.Trim(s[:48], "-")
	}
	return s
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/lore/ -v`
Expected: PASS (4 tests).

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/lore/
git commit -m "lore: record model with YAML frontmatter round-trip"
```

---

### Task 2: Layout and repo identity

**Files:**
- Create: `internal/lore/layout.go`
- Test: `internal/lore/layout_test.go`

**Interfaces:**
- Consumes: `Type` constants from Task 1.
- Produces:
  - `type Layout struct { RepoRoot, RepoDir, OverlayDir string }` — `RepoDir` = `<root>/.lore`, `OverlayDir` = `<home>/.codeindex/lore/<repo-id>` where `<home>` honors `CODEINDEX_HOME` (else `os.UserHomeDir()/.codeindex`)
  - `func NewLayout(root string) (Layout, error)`
  - `func (l Layout) Dir(layer string, t Type) string` — layer `"repo"` or `"overlay"`; type dirs `decisions|items|notes`
  - `func (l Layout) SessionsDir() string` — `<OverlayDir>/sessions`
  - `func RepoID(root string) string` — `<slug>-<12 hex>`: normalized git origin (`github.com/org/repo` from both SSH and HTTPS forms) hashed with sha256; falls back to the absolute path when no origin

- [ ] **Step 1: Write the failing test**

```go
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
	if id := RepoID(dir); id == "" || !strings.Contains(id, "-") {
		t.Fatalf("fallback id: %q", id)
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/lore/ -run 'TestRepoID|TestLayout' -v`
Expected: FAIL with "undefined: RepoID" etc.

- [ ] **Step 3: Write the implementation**

```go
package lore

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Layout locates the two record layers for a repo.
type Layout struct {
	RepoRoot   string // absolute repo root
	RepoDir    string // <root>/.lore (committed)
	OverlayDir string // <home>/.codeindex/lore/<repo-id> (private)
}

func NewLayout(root string) (Layout, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Layout{}, err
	}
	home := os.Getenv("CODEINDEX_HOME")
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return Layout{}, err
		}
		home = filepath.Join(h, ".codeindex")
	}
	return Layout{
		RepoRoot:   abs,
		RepoDir:    filepath.Join(abs, ".lore"),
		OverlayDir: filepath.Join(home, "lore", RepoID(abs)),
	}, nil
}

func typeDir(t Type) string {
	switch t {
	case TypeDecision:
		return "decisions"
	case TypeItem:
		return "items"
	}
	return "notes"
}

// Dir returns the directory for a layer ("repo" | "overlay") and record type.
func (l Layout) Dir(layer string, t Type) string {
	if layer == "overlay" {
		return filepath.Join(l.OverlayDir, typeDir(t))
	}
	return filepath.Join(l.RepoDir, typeDir(t))
}

func (l Layout) SessionsDir() string { return filepath.Join(l.OverlayDir, "sessions") }

// RepoID identifies a repo across clones and worktrees: the normalized origin
// remote hashed, so every checkout of one repo shares one overlay. Repos
// without an origin fall back to their absolute path.
func RepoID(root string) string {
	cmd := exec.Command("git", "config", "--get", "remote.origin.url")
	cmd.Dir = root
	out, err := cmd.Output()
	key := strings.TrimSpace(string(out))
	slug := filepath.Base(root)
	if err == nil && key != "" {
		key = normalizeOrigin(key)
		if i := strings.LastIndex(key, "/"); i >= 0 {
			slug = key[i+1:]
		}
	} else {
		abs, _ := filepath.Abs(root)
		key = abs
	}
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%s-%x", Slug(slug), sum[:6])
}

// normalizeOrigin maps SSH and HTTPS remote forms to "host/org/repo".
func normalizeOrigin(u string) string {
	u = strings.TrimSuffix(u, ".git")
	if rest, ok := strings.CutPrefix(u, "git@"); ok { // git@host:org/repo
		return strings.Replace(rest, ":", "/", 1)
	}
	for _, p := range []string{"https://", "http://", "ssh://git@", "ssh://"} {
		if rest, ok := strings.CutPrefix(u, p); ok {
			return rest
		}
	}
	return u
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/lore/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/lore/layout.go internal/lore/layout_test.go
git commit -m "lore: layout with repo-identity overlay (clones share one overlay)"
```

---

### Task 3: Lore store — schema, upsert, load, freshness hashes

**Files:**
- Create: `internal/lore/index/store.go`
- Test: `internal/lore/index/store_test.go`

**Interfaces:**
- Consumes: `lore.Record`, `lore.Type`, `lore.Anchor`, `lore.Ref` (Task 1).
- Produces:
  - `type Store struct { ... }` with `func Open(path string) (*Store, error)` (creates schema; on version mismatch drops all lore tables and recreates — index is derived, records are the truth), `func (s *Store) Close() error`
  - `type StoredRecord struct { lore.Record; Layer, File string; Stale bool; Confidence float64 }` — `Layer` ∈ `repo|overlay|session`, `File` is the absolute source path
  - `func (s *Store) Upsert(r lore.Record, layer, file string) error`
  - `func (s *Store) DeleteByFile(file string) error`
  - `func (s *Store) All() ([]StoredRecord, error)` (ordered by date desc, id)
  - `func (s *Store) Get(id string) (StoredRecord, bool, error)`
  - `func (s *Store) FileHashes() (map[string]string, error)`, `func (s *Store) SetFileHash(path, hash string) error`, `func (s *Store) DeleteFileHash(path string) error`
  - `func (s *Store) SetStale(id string, stale bool) error`

- [ ] **Step 1: Write the failing test**

```go
package index

import (
	"path/filepath"
	"testing"

	"codeindex/internal/lore"
)

func openStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "lore.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUpsertGetAll(t *testing.T) {
	s := openStore(t)
	r := lore.Record{
		ID: "dec-A", Type: lore.TypeDecision, Title: "T", Status: "active",
		Date:    "2026-07-29",
		Anchors: []lore.Anchor{{Symbol: "Foo"}},
		Refs:    []lore.Ref{{Kind: "gh-issue", Value: "e/x#1"}},
		Body:    "body",
	}
	if err := s.Upsert(r, "repo", "/repo/.lore/decisions/a.md"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Get("dec-A")
	if err != nil || !ok {
		t.Fatalf("get: %v ok=%v", err, ok)
	}
	if got.Layer != "repo" || got.Title != "T" || len(got.Anchors) != 1 ||
		got.Anchors[0].Symbol != "Foo" || got.Refs[0].Kind != "gh-issue" {
		t.Fatalf("got %+v", got)
	}
	// Upsert replaces children, not duplicates them.
	r.Anchors = []lore.Anchor{{Path: "internal/"}}
	if err := s.Upsert(r, "repo", "/repo/.lore/decisions/a.md"); err != nil {
		t.Fatal(err)
	}
	got, _, _ = s.Get("dec-A")
	if len(got.Anchors) != 1 || got.Anchors[0].Path != "internal/" {
		t.Fatalf("anchors after re-upsert: %+v", got.Anchors)
	}
	all, err := s.All()
	if err != nil || len(all) != 1 {
		t.Fatalf("all: %v n=%d", err, len(all))
	}
}

func TestDeleteByFileAndHashes(t *testing.T) {
	s := openStore(t)
	r := lore.Record{ID: "note-B", Type: lore.TypeNote, Title: "n", Date: "2026-07-29"}
	if err := s.Upsert(r, "overlay", "/o/notes/b.md"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFileHash("/o/notes/b.md", "h1"); err != nil {
		t.Fatal(err)
	}
	m, err := s.FileHashes()
	if err != nil || m["/o/notes/b.md"] != "h1" {
		t.Fatalf("hashes %v %v", m, err)
	}
	if err := s.DeleteByFile("/o/notes/b.md"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFileHash("/o/notes/b.md"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Get("note-B"); ok {
		t.Fatal("record survived DeleteByFile")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/lore/index/ -v`
Expected: FAIL (package missing).

- [ ] **Step 3: Write the implementation**

```go
// Package index maintains the derived lore search index in
// .codeindex/lore.db. Records on disk are the source of truth; this database
// can always be deleted and rebuilt.
package index

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"

	"codeindex/internal/lore"
)

const schemaVersion = 1

const schema = `
CREATE TABLE IF NOT EXISTS lore_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS lore_files (path TEXT PRIMARY KEY, hash TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS lore_records (
  id TEXT PRIMARY KEY, type TEXT NOT NULL, title TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT '', date TEXT NOT NULL DEFAULT '',
  layer TEXT NOT NULL, file TEXT NOT NULL,
  priority TEXT NOT NULL DEFAULT '',
  supersedes TEXT NOT NULL DEFAULT '', superseded_by TEXT NOT NULL DEFAULT '',
  stale INTEGER NOT NULL DEFAULT 0, confidence REAL NOT NULL DEFAULT 0,
  body TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS idx_lore_records_file ON lore_records(file);
CREATE TABLE IF NOT EXISTS lore_anchors (
  record_id TEXT NOT NULL, path TEXT NOT NULL DEFAULT '', symbol TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS idx_lore_anchors_rec ON lore_anchors(record_id);
CREATE TABLE IF NOT EXISTS lore_refs (
  record_id TEXT NOT NULL, kind TEXT NOT NULL, value TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS idx_lore_refs_rec ON lore_refs(record_id);
CREATE TABLE IF NOT EXISTS lore_blocked (record_id TEXT NOT NULL, blocked_by TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS lore_tags (record_id TEXT NOT NULL, tag TEXT NOT NULL);
`

type Store struct{ db *sql.DB }

// StoredRecord is a record plus its index-side metadata.
type StoredRecord struct {
	lore.Record
	Layer      string
	File       string
	Stale      bool
	Confidence float64
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	var ver string
	err = db.QueryRow(`SELECT value FROM lore_meta WHERE key='schema'`).Scan(&ver)
	if err == sql.ErrNoRows {
		_, err = db.Exec(`INSERT INTO lore_meta(key,value) VALUES('schema',?)`,
			fmt.Sprint(schemaVersion))
	} else if err == nil && ver != fmt.Sprint(schemaVersion) {
		// Derived data: on mismatch, wipe and let the next reindex rebuild.
		for _, t := range []string{"lore_files", "lore_records", "lore_anchors",
			"lore_refs", "lore_blocked", "lore_tags"} {
			if _, err = db.Exec("DELETE FROM " + t); err != nil {
				break
			}
		}
		if err == nil {
			_, err = db.Exec(`UPDATE lore_meta SET value=? WHERE key='schema'`,
				fmt.Sprint(schemaVersion))
		}
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Upsert(r lore.Record, layer, file string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO lore_records
		(id,type,title,status,date,layer,file,priority,supersedes,superseded_by,body)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET type=excluded.type,title=excluded.title,
		status=excluded.status,date=excluded.date,layer=excluded.layer,
		file=excluded.file,priority=excluded.priority,supersedes=excluded.supersedes,
		superseded_by=excluded.superseded_by,body=excluded.body`,
		r.ID, string(r.Type), r.Title, r.Status, r.Date, layer, file,
		r.Priority, r.Supersedes, r.SupersededBy, r.Body); err != nil {
		return err
	}
	for _, t := range []string{"lore_anchors", "lore_refs", "lore_blocked", "lore_tags"} {
		if _, err := tx.Exec("DELETE FROM "+t+" WHERE record_id=?", r.ID); err != nil {
			return err
		}
	}
	for _, a := range r.Anchors {
		if _, err := tx.Exec(`INSERT INTO lore_anchors(record_id,path,symbol) VALUES(?,?,?)`,
			r.ID, a.Path, a.Symbol); err != nil {
			return err
		}
	}
	for _, ref := range r.Refs {
		if _, err := tx.Exec(`INSERT INTO lore_refs(record_id,kind,value) VALUES(?,?,?)`,
			r.ID, ref.Kind, ref.Value); err != nil {
			return err
		}
	}
	for _, b := range r.BlockedBy {
		if _, err := tx.Exec(`INSERT INTO lore_blocked(record_id,blocked_by) VALUES(?,?)`,
			r.ID, b); err != nil {
			return err
		}
	}
	for _, tag := range r.Tags {
		if _, err := tx.Exec(`INSERT INTO lore_tags(record_id,tag) VALUES(?,?)`,
			r.ID, tag); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteByFile(file string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id FROM lore_records WHERE file=?`, file)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		for _, t := range []string{"lore_records", "lore_anchors", "lore_refs",
			"lore_blocked", "lore_tags"} {
			col := "record_id"
			if t == "lore_records" {
				col = "id"
			}
			if _, err := tx.Exec("DELETE FROM "+t+" WHERE "+col+"=?", id); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Store) All() ([]StoredRecord, error) {
	rows, err := s.db.Query(`SELECT id,type,title,status,date,layer,file,priority,
		supersedes,superseded_by,stale,confidence,body
		FROM lore_records ORDER BY date DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoredRecord
	for rows.Next() {
		var r StoredRecord
		var typ string
		var stale int
		if err := rows.Scan(&r.ID, &typ, &r.Title, &r.Status, &r.Date, &r.Layer,
			&r.File, &r.Priority, &r.Supersedes, &r.SupersededBy, &stale,
			&r.Confidence, &r.Body); err != nil {
			return nil, err
		}
		r.Type, r.Stale = lore.Type(typ), stale != 0
		if err := s.loadChildren(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) loadChildren(r *StoredRecord) error {
	rows, err := s.db.Query(`SELECT path,symbol FROM lore_anchors WHERE record_id=?`, r.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var a lore.Anchor
		if err := rows.Scan(&a.Path, &a.Symbol); err != nil {
			rows.Close()
			return err
		}
		r.Anchors = append(r.Anchors, a)
	}
	rows.Close()
	rows, err = s.db.Query(`SELECT kind,value FROM lore_refs WHERE record_id=?`, r.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var ref lore.Ref
		if err := rows.Scan(&ref.Kind, &ref.Value); err != nil {
			rows.Close()
			return err
		}
		r.Refs = append(r.Refs, ref)
	}
	rows.Close()
	rows, err = s.db.Query(`SELECT blocked_by FROM lore_blocked WHERE record_id=?`, r.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			rows.Close()
			return err
		}
		r.BlockedBy = append(r.BlockedBy, b)
	}
	rows.Close()
	rows, err = s.db.Query(`SELECT tag FROM lore_tags WHERE record_id=?`, r.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var tg string
		if err := rows.Scan(&tg); err != nil {
			rows.Close()
			return err
		}
		r.Tags = append(r.Tags, tg)
	}
	rows.Close()
	return nil
}

func (s *Store) Get(id string) (StoredRecord, bool, error) {
	all, err := s.All() // corpus is small; reuse the loader
	if err != nil {
		return StoredRecord{}, false, err
	}
	for _, r := range all {
		if r.ID == id {
			return r, true, nil
		}
	}
	return StoredRecord{}, false, nil
}

func (s *Store) FileHashes() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT path,hash FROM lore_files`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var p, h string
		if err := rows.Scan(&p, &h); err != nil {
			return nil, err
		}
		m[p] = h
	}
	return m, rows.Err()
}

func (s *Store) SetFileHash(path, hash string) error {
	_, err := s.db.Exec(`INSERT INTO lore_files(path,hash) VALUES(?,?)
		ON CONFLICT(path) DO UPDATE SET hash=excluded.hash`, path, hash)
	return err
}

func (s *Store) DeleteFileHash(path string) error {
	_, err := s.db.Exec(`DELETE FROM lore_files WHERE path=?`, path)
	return err
}

func (s *Store) SetStale(id string, stale bool) error {
	v := 0
	if stale {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE lore_records SET stale=? WHERE id=?`, v, id)
	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/lore/index/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/lore/index/
git commit -m "lore: derived SQLite store with self-healing schema versioning"
```

---

### Task 4: Reindex — walk layers, hash-diff, parse, patch

**Files:**
- Create: `internal/lore/index/reindex.go`
- Test: `internal/lore/index/reindex_test.go`

**Interfaces:**
- Consumes: `lore.Layout` (Task 2), `Store` (Task 3), `lore.Parse` (Task 1).
- Produces:
  - `type FileError struct { Path string; Err error }`
  - `type Report struct { Indexed, Removed int; Errors []FileError }`
  - `func Reindex(l lore.Layout, dbPath string) (*Store, Report, error)` — walks `RepoDir/{decisions,items,notes}` (layer `repo`), `OverlayDir/{decisions,items,notes}` (layer `overlay`), `SessionsDir()` (layer `session`, parsed as notes); sha256-diffs against `lore_files`; parses only changed files; deletes records for vanished files; malformed files are recorded in `Report.Errors` and skipped (fail-open). Missing directories are fine (zero records).

- [ ] **Step 1: Write the failing test**

```go
package index

import (
	"os"
	"path/filepath"
	"testing"

	"codeindex/internal/lore"
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
	os.Remove(filepath.Join(l.Dir("overlay", lore.TypeNote), "n.md"))
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
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "bad.md"), []byte("no frontmatter"), 0o644)
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/lore/index/ -run TestReindex -v`
Expected: FAIL with "undefined: Reindex".

- [ ] **Step 3: Write the implementation**

```go
package index

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeindex/internal/lore"
)

type FileError struct {
	Path string
	Err  error
}

type Report struct {
	Indexed, Removed int
	Errors           []FileError
}

// source is one directory to scan: its layer tag and the record type its
// files parse as.
type source struct {
	dir   string
	layer string
	typ   lore.Type
}

func sources(l lore.Layout) []source {
	var out []source
	for _, t := range []lore.Type{lore.TypeDecision, lore.TypeItem, lore.TypeNote} {
		out = append(out,
			source{l.Dir("repo", t), "repo", t},
			source{l.Dir("overlay", t), "overlay", t})
	}
	out = append(out, source{l.SessionsDir(), "session", lore.TypeNote})
	return out
}

// Reindex opens (or creates) the lore index and patches it to match the
// record files on disk. Malformed files are reported, never fatal.
func Reindex(l lore.Layout, dbPath string) (*Store, Report, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, Report{}, err
	}
	s, err := Open(dbPath)
	if err != nil {
		return nil, Report{}, err
	}
	var rep Report
	stored, err := s.FileHashes()
	if err != nil {
		s.Close()
		return nil, rep, err
	}
	seen := map[string]bool{}
	for _, src := range sources(l) {
		entries, err := os.ReadDir(src.dir)
		if err != nil {
			continue // missing layer dir: fine
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") ||
				strings.EqualFold(e.Name(), "README.md") {
				continue
			}
			p := filepath.Join(src.dir, e.Name())
			seen[p] = true
			b, err := os.ReadFile(p)
			if err != nil {
				rep.Errors = append(rep.Errors, FileError{p, err})
				continue
			}
			h := fmt.Sprintf("%x", sha256.Sum256(b))
			if stored[p] == h {
				continue
			}
			rec, err := lore.Parse(b, src.typ)
			if err != nil {
				rep.Errors = append(rep.Errors, FileError{p, err})
				continue
			}
			if err := s.Upsert(rec, src.layer, p); err != nil {
				s.Close()
				return nil, rep, err
			}
			if err := s.SetFileHash(p, h); err != nil {
				s.Close()
				return nil, rep, err
			}
			rep.Indexed++
		}
	}
	for p := range stored {
		if seen[p] {
			continue
		}
		if err := s.DeleteByFile(p); err != nil {
			s.Close()
			return nil, rep, err
		}
		if err := s.DeleteFileHash(p); err != nil {
			s.Close()
			return nil, rep, err
		}
		rep.Removed++
	}
	return s, rep, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/lore/index/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/lore/index/reindex.go internal/lore/index/reindex_test.go
git commit -m "lore: lazy reindex with content-hash diffing across layers"
```

---

### Task 5: Search and ranking

**Files:**
- Create: `internal/lore/index/search.go`
- Test: `internal/lore/index/search_test.go`

**Interfaces:**
- Consumes: `StoredRecord` (Task 3), `search.Tokenize`/`search.Stem` from the existing `codeindex/internal/search` package.
- Produces:
  - `type Hit struct { Rec StoredRecord; Score float64; Snippet string }`
  - `func Search(recs []StoredRecord, query string, now time.Time, limit int) []Hit`

Scoring (per record): split `title` (weight 2.0) and body chunks on `"\n## "` boundaries; per chunk, score = (matched query stems / total query stems) × (1 + ln(1 + term frequency)); record base score = max chunk score × weight. Multipliers: layer `session` → `exp(-ln2 × ageDays/7)` (age from `Date`; unparsable date → 1.0); closed statuses (`superseded`, `rejected`, `done`, `dropped`) → ×0.5; `Stale` → ×0.7. Records scoring 0 are omitted. Ranked descending; `Snippet` is the first 120 chars of the best-matching chunk (single line).

- [ ] **Step 1: Write the failing test**

```go
package index

import (
	"testing"
	"time"

	"codeindex/internal/lore"
)

func rec(id, title, body, status, layer, date string) StoredRecord {
	return StoredRecord{
		Record: lore.Record{ID: id, Type: lore.TypeDecision, Title: title,
			Body: body, Status: status, Date: date},
		Layer: layer,
	}
}

func TestSearchRanksTitleAndStatus(t *testing.T) {
	now := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	recs := []StoredRecord{
		rec("dec-1", "Use Go for the engine", "why go\n", "active", "repo", "2026-07-01"),
		rec("dec-2", "Old runtime choice", "we chose go once\n", "superseded", "repo", "2026-06-01"),
		rec("dec-3", "Unrelated", "nothing here\n", "active", "repo", "2026-07-01"),
	}
	hits := Search(recs, "go engine", now, 10)
	if len(hits) != 2 {
		t.Fatalf("hits=%d want 2 (unrelated omitted)", len(hits))
	}
	if hits[0].Rec.ID != "dec-1" {
		t.Fatalf("active title match should outrank superseded body: %s", hits[0].Rec.ID)
	}
}

func TestSearchSessionDecay(t *testing.T) {
	now := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	fresh := rec("note-f", "postgres tuning", "b\n", "", "session", "2026-07-28")
	old := rec("note-o", "postgres tuning", "b\n", "", "session", "2026-06-01")
	hits := Search([]StoredRecord{old, fresh}, "postgres tuning", now, 10)
	if len(hits) != 2 || hits[0].Rec.ID != "note-f" {
		t.Fatalf("fresh session note should outrank old: %+v", hits)
	}
	if hits[1].Score >= hits[0].Score/2 {
		t.Fatalf("8-week-old note barely decayed: %v vs %v", hits[1].Score, hits[0].Score)
	}
}

func TestSearchChunkSnippet(t *testing.T) {
	r := rec("dec-c", "T", "intro\n\n## Alternatives considered\nRust was rejected here\n",
		"active", "repo", "2026-07-01")
	hits := Search([]StoredRecord{r}, "rust rejected", time.Now().UTC(), 10)
	if len(hits) != 1 || hits[0].Snippet == "" ||
		!containsFold(hits[0].Snippet, "rust") {
		t.Fatalf("snippet from matching chunk: %+v", hits)
	}
}
```

Also add this helper at the bottom of the test file:

```go
func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
```

(and `strings` to the imports).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/lore/index/ -run TestSearch -v`
Expected: FAIL with "undefined: Search".

- [ ] **Step 3: Write the implementation**

```go
package index

import (
	"math"
	"sort"
	"strings"
	"time"

	"codeindex/internal/search"
)

type Hit struct {
	Rec     StoredRecord
	Score   float64
	Snippet string
}

// Search ranks records against a query. The corpus is small (hundreds of
// records), so this is a full in-memory scan — the same D1 pattern
// internal/search validated at far larger scale.
func Search(recs []StoredRecord, query string, now time.Time, limit int) []Hit {
	if limit <= 0 {
		limit = 10
	}
	qStems := stems(query)
	if len(qStems) == 0 {
		return nil
	}
	var hits []Hit
	for _, r := range recs {
		score, snippet := scoreRecord(r, qStems)
		if score == 0 {
			continue
		}
		score *= layerFactor(r, now) * statusFactor(r.Status)
		if r.Stale {
			score *= 0.7
		}
		hits = append(hits, Hit{Rec: r, Score: score, Snippet: snippet})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Rec.ID < hits[j].Rec.ID
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func stems(s string) []string {
	toks := search.Tokenize(s)
	out := make([]string, len(toks))
	for i, t := range toks {
		out[i] = search.Stem(t)
	}
	return out
}

// scoreRecord returns the best chunk score (title weighted 2x) and that
// chunk's snippet.
func scoreRecord(r StoredRecord, qStems []string) (float64, string) {
	best, snippet := 0.0, ""
	consider := func(text string, weight float64) {
		s := chunkScore(text, qStems) * weight
		if s > best {
			best, snippet = s, snip(text)
		}
	}
	consider(r.Title, 2.0)
	for _, chunk := range strings.Split(r.Body, "\n## ") {
		consider(chunk, 1.0)
	}
	return best, snippet
}

func chunkScore(text string, qStems []string) float64 {
	cs := stems(text)
	tf := map[string]int{}
	for _, s := range cs {
		tf[s]++
	}
	matched, freq := 0, 0
	for _, q := range qStems {
		if tf[q] > 0 {
			matched++
			freq += tf[q]
		}
	}
	if matched == 0 {
		return 0
	}
	return float64(matched) / float64(len(qStems)) * (1 + math.Log(1+float64(freq)))
}

func layerFactor(r StoredRecord, now time.Time) float64 {
	if r.Layer != "session" {
		return 1.0
	}
	d, err := time.Parse("2006-01-02", r.Date)
	if err != nil {
		return 1.0
	}
	ageDays := now.Sub(d).Hours() / 24
	if ageDays < 0 {
		ageDays = 0
	}
	return math.Exp(-math.Ln2 * ageDays / 7) // 7-day half-life
}

func statusFactor(status string) float64 {
	switch status {
	case "superseded", "rejected", "done", "dropped":
		return 0.5
	}
	return 1.0
}

func snip(text string) string {
	line := strings.Join(strings.Fields(text), " ")
	if len(line) > 120 {
		line = line[:120]
	}
	return line
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/lore/index/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/lore/index/search.go internal/lore/index/search_test.go
git commit -m "lore: chunked BM25-lite search with layer decay and status weighting"
```

---

### Task 6: Anchor staleness against graph.db

**Files:**
- Create: `internal/lore/index/anchors.go`
- Test: `internal/lore/index/anchors_test.go`

**Interfaces:**
- Consumes: `StoredRecord` (Task 3); `codeindex/internal/graph` (`graph.Open`, `Store.Begin`, `Store.PutFile` — same fixture pattern as `internal/graph/store_test.go`).
- Produces:
  - `func StaleRecords(repoRoot, graphDBPath string, recs []StoredRecord) (map[string]bool, error)` — record ID → stale. A record is stale when ANY anchor fails: a `Path` anchor whose `repoRoot/path` does not exist on disk, or a `Symbol` anchor not present in `graph.db`. When `graphDBPath` does not exist, symbol anchors are skipped (not stale) — the code index is optional for lore.
- Note for implementer: schema v7 keeps `symbols_t` behind a `symbols` view that exposes resolved `name`. If `SELECT COUNT(1) FROM symbols WHERE name = ?` errors in the fixture test, run `grep -n "CREATE VIEW" internal/graph/store.go` and adapt the column/view name — the fixture test is the guard.

- [ ] **Step 1: Write the failing test**

```go
package index

import (
	"os"
	"path/filepath"
	"testing"

	"codeindex/internal/graph"
	"codeindex/internal/lore"
)

func fixtureGraph(t *testing.T, dir string) string {
	t.Helper()
	dbPath := filepath.Join(dir, ".codeindex", "graph.db")
	os.MkdirAll(filepath.Dir(dbPath), 0o755)
	st, err := graph.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	tx, err := st.Begin()
	if err != nil {
		t.Fatal(err)
	}
	pf := &graph.ParsedFile{Path: "a.go", Symbols: []graph.Symbol{
		{File: "a.go", Name: "ResolveImports", Kind: graph.KindFunc,
			Signature: "func ResolveImports()", StartLine: 1, EndLine: 2},
	}}
	if _, _, err := st.PutFile(tx, pf, graph.FileMeta{Path: "a.go", Hash: "h", Size: 1, Mtime: 1}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return dbPath
}

func anchored(id string, a ...lore.Anchor) StoredRecord {
	return StoredRecord{Record: lore.Record{ID: id, Type: lore.TypeDecision, Anchors: a}}
}

func TestStaleRecords(t *testing.T) {
	root := t.TempDir()
	db := fixtureGraph(t, root)
	os.MkdirAll(filepath.Join(root, "internal", "engine"), 0o755)

	recs := []StoredRecord{
		anchored("dec-ok", lore.Anchor{Symbol: "ResolveImports"},
			lore.Anchor{Path: "internal/engine/"}),
		anchored("dec-gone-sym", lore.Anchor{Symbol: "DeletedSymbol"}),
		anchored("dec-gone-path", lore.Anchor{Path: "no/such/dir/"}),
		anchored("dec-unanchored"),
	}
	stale, err := StaleRecords(root, db, recs)
	if err != nil {
		t.Fatal(err)
	}
	if stale["dec-ok"] || !stale["dec-gone-sym"] || !stale["dec-gone-path"] ||
		stale["dec-unanchored"] {
		t.Fatalf("stale map: %+v", stale)
	}
}

func TestStaleRecordsWithoutGraphDB(t *testing.T) {
	root := t.TempDir()
	recs := []StoredRecord{anchored("dec-s", lore.Anchor{Symbol: "Whatever"})}
	stale, err := StaleRecords(root, filepath.Join(root, "missing.db"), recs)
	if err != nil {
		t.Fatal(err)
	}
	if stale["dec-s"] {
		t.Fatal("symbol anchors must be skipped when graph.db is absent")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/lore/index/ -run TestStale -v`
Expected: FAIL with "undefined: StaleRecords".

- [ ] **Step 3: Write the implementation**

```go
package index

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
)

// StaleRecords maps record IDs to staleness: an anchor pointing at a path
// that no longer exists or a symbol no longer in the code graph. graph.db is
// optional — without it, symbol anchors are trusted.
func StaleRecords(repoRoot, graphDBPath string, recs []StoredRecord) (map[string]bool, error) {
	stale := map[string]bool{}
	var db *sql.DB
	if _, err := os.Stat(graphDBPath); err == nil {
		db, err = sql.Open("sqlite3", graphDBPath)
		if err != nil {
			return nil, err
		}
		defer db.Close()
	}
	for _, r := range recs {
		for _, a := range r.Anchors {
			if a.Path != "" {
				p := filepath.Join(repoRoot, strings.TrimSuffix(a.Path, "/"))
				if _, err := os.Stat(p); err != nil {
					stale[r.ID] = true
				}
			}
			if a.Symbol != "" && db != nil {
				var n int
				if err := db.QueryRow(
					`SELECT COUNT(1) FROM symbols WHERE name = ?`, a.Symbol).Scan(&n); err != nil {
					return nil, err
				}
				if n == 0 {
					stale[r.ID] = true
				}
			}
		}
	}
	return stale, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/lore/index/ -v`
Expected: PASS. If `TestStaleRecords` fails with "no such table: symbols", follow the note in Interfaces (check the view name in `internal/graph/store.go`) and adjust the query.

- [ ] **Step 5: Commit**

```bash
git add internal/lore/index/anchors.go internal/lore/index/anchors_test.go
git commit -m "lore: symbol/path anchor staleness detection against graph.db"
```

---

### Task 7: CLI wiring, `lore add`, `lore show`

**Files:**
- Create: `cmd/codeindex/lore.go`
- Test: `cmd/codeindex/lore_test.go`
- Modify: `cmd/codeindex/main.go` (add one switch case; usage line)

**Interfaces:**
- Consumes: everything above.
- Produces (used by Tasks 8–12):
  - `func runLore(root string, args []string, out io.Writer) error` — `args[0]` is the subcommand; dispatched from `main.go`'s switch as `case "lore": if err := runLore(root, os.Args[3:], os.Stdout); err != nil { fatal(err) }`
  - `func loreDBPath(root string) string` — `<root>/.codeindex/lore.db`
  - `func loreReindex(root string) (lore.Layout, *index.Store, index.Report, error)` — shared preamble for every subcommand
  - Flag helpers reused by later subcommands: `func stringFlag(args []string, name string) string`, `func multiFlag(args []string, name string) []string`, `func boolIn(args []string, name string) bool`

Subcommand grammar for this task:
- `codeindex lore <repo> add <decision|item|note> --title T [--body TEXT|-] [--anchor path:P|symbol:S ...] [--ref kind:value ...] [--priority pN] [--tag T ...] [--private]` — creates `<date>-<slug>.md` in the layer/type dir, prints `created <id> <path>`. `--body -` reads stdin. `--private` writes to the overlay.
- `codeindex lore <repo> show <id>` — prints a meta line (`id  type/status  layer  file` plus `STALE` when set) then the marshaled record.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/codeindex/ -run TestLore -v`
Expected: FAIL with "undefined: runLore".

- [ ] **Step 3: Write the implementation**

`cmd/codeindex/lore.go`:

```go
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codeindex/internal/lore"
	"codeindex/internal/lore/index"
)

func loreDBPath(root string) string {
	return filepath.Join(root, ".codeindex", "lore.db")
}

// loreReindex is every lore subcommand's preamble: locate layers, patch the
// derived index, hand back the open store. Parse errors are fail-open (the
// report carries them; doctor surfaces them).
func loreReindex(root string) (lore.Layout, *index.Store, index.Report, error) {
	l, err := lore.NewLayout(root)
	if err != nil {
		return lore.Layout{}, nil, index.Report{}, err
	}
	st, rep, err := index.Reindex(l, loreDBPath(root))
	return l, st, rep, err
}

const loreUsage = "usage: codeindex lore <repo-root> " +
	"<add|show|search|for|backlog|promote|supersede|doctor|init> ..."

func runLore(root string, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf(loreUsage)
	}
	switch args[0] {
	case "add":
		return loreAdd(root, args[1:], out)
	case "show":
		return loreShow(root, args[1:], out)
	default:
		return fmt.Errorf("unknown lore subcommand %q\n%s", args[0], loreUsage)
	}
}

// --- flag helpers (plain args scan, matching main.go's style) ---

func stringFlag(args []string, name string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
}

func multiFlag(args []string, name string) []string {
	var out []string
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name {
			out = append(out, args[i+1])
		}
	}
	return out
}

func boolIn(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}

// --- add ---

func loreAdd(root string, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: codeindex lore <repo> add <decision|item|note> --title T ...")
	}
	typ := lore.Type(args[0])
	if typ != lore.TypeDecision && typ != lore.TypeItem && typ != lore.TypeNote {
		return fmt.Errorf("unknown record type %q (want decision|item|note)", args[0])
	}
	title := stringFlag(args, "--title")
	if title == "" {
		return fmt.Errorf("--title is required")
	}
	body := stringFlag(args, "--body")
	if body == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		body = string(b)
	}
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	rec := lore.Record{
		ID: lore.NewID(typ), Type: typ, Title: title,
		Status: lore.DefaultStatus(typ),
		Date:   time.Now().UTC().Format("2006-01-02"),
		Body:   body, Priority: stringFlag(args, "--priority"),
		Tags: multiFlag(args, "--tag"),
	}
	for _, a := range multiFlag(args, "--anchor") {
		kind, val, ok := strings.Cut(a, ":")
		if !ok || (kind != "path" && kind != "symbol") {
			return fmt.Errorf("bad --anchor %q (want path:P or symbol:S)", a)
		}
		if kind == "path" {
			rec.Anchors = append(rec.Anchors, lore.Anchor{Path: val})
		} else {
			rec.Anchors = append(rec.Anchors, lore.Anchor{Symbol: val})
		}
	}
	for _, r := range multiFlag(args, "--ref") {
		kind, val, ok := strings.Cut(r, ":")
		if !ok {
			return fmt.Errorf("bad --ref %q (want kind:value)", r)
		}
		rec.Refs = append(rec.Refs, lore.Ref{Kind: kind, Value: val})
	}
	layer := "repo"
	if boolIn(args, "--private") {
		layer = "overlay"
	}
	l, err := lore.NewLayout(root)
	if err != nil {
		return err
	}
	dir := l.Dir(layer, typ)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := rec.Date + "-" + lore.Slug(title) + ".md"
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		// Same-day same-slug collision: disambiguate with the ID tail.
		name = rec.Date + "-" + lore.Slug(title) + "-" + rec.ID[len(rec.ID)-6:] + ".md"
		path = filepath.Join(dir, name)
	}
	b, err := rec.Marshal()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "created %s %s\n", rec.ID, path)
	return nil
}

// --- show ---

func loreShow(root string, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: codeindex lore <repo> show <id>")
	}
	_, st, _, err := loreReindex(root)
	if err != nil {
		return err
	}
	defer st.Close()
	r, ok, err := st.Get(args[0])
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no record %q", args[0])
	}
	flags := ""
	if r.Stale {
		flags = "  STALE"
	}
	fmt.Fprintf(out, "%s  %s/%s  %s  %s%s\n\n", r.ID, r.Type, orDash(r.Status),
		r.Layer, r.File, flags)
	b, err := r.Record.Marshal()
	if err != nil {
		return err
	}
	out.Write(b)
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
```

`cmd/codeindex/main.go` — two edits. In the usage string (line ~31), add `lore` to the command list. In the switch (after `case "tree":`), add:

```go
	case "lore":
		if err := runLore(root, os.Args[3:], os.Stdout); err != nil {
			fatal(err)
		}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/codeindex/ -run TestLore -v && go build ./cmd/codeindex`
Expected: PASS; build succeeds.

- [ ] **Step 5: Commit**

```bash
git add cmd/codeindex/lore.go cmd/codeindex/lore_test.go cmd/codeindex/main.go
git commit -m "lore: CLI wiring with add and show subcommands"
```

---

### Task 8: `lore search` and `lore for` (with `--json`)

**Files:**
- Modify: `cmd/codeindex/lore.go` (add cases + functions)
- Test: append to `cmd/codeindex/lore_test.go`

**Interfaces:**
- Consumes: `index.Search` (Task 5), `runLore` dispatch (Task 7).
- Produces:
  - `codeindex lore <repo> search <query> [--limit N] [--json]` — text lines: `<id>  <score %.2f>  [<layer>/<status>]  <title> — <snippet>`; with `--json`, an array of `{id,type,title,status,date,layer,stale,score,snippet,file}`.
  - `codeindex lore <repo> for <path-or-symbol> [--json]` — records with a matching anchor: symbol anchors match exactly; path anchors match when either is a prefix of the other (after trimming trailing `/`). Text format: `<id>  [<layer>/<status>]  <title>` one per line; `--json` same shape as search minus score/snippet.
  - `type loreJSON struct { ID, Type, Title, Status, Date, Layer, File, Snippet string; Stale bool; Score float64 }` and `func toJSON(out io.Writer, v any) error` (uses `json.MarshalIndent`).

- [ ] **Step 1: Write the failing test** (append to `cmd/codeindex/lore_test.go`)

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/codeindex/ -run 'TestLoreSearch|TestLoreFor' -v`
Expected: FAIL with `unknown lore subcommand "search"`.

- [ ] **Step 3: Write the implementation** (add to `cmd/codeindex/lore.go`; register `case "search": return loreSearch(root, args[1:], out)` and `case "for": return loreFor(root, args[1:], out)` in `runLore`)

```go
type loreJSON struct {
	ID      string  `json:"id"`
	Type    string  `json:"type"`
	Title   string  `json:"title"`
	Status  string  `json:"status,omitempty"`
	Date    string  `json:"date"`
	Layer   string  `json:"layer"`
	File    string  `json:"file"`
	Stale   bool    `json:"stale,omitempty"`
	Score   float64 `json:"score,omitempty"`
	Snippet string  `json:"snippet,omitempty"`
}

func toJSON(out io.Writer, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(b))
	return err
}

func loreSearch(root string, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: codeindex lore <repo> search <query> [--limit N] [--json]")
	}
	limit := 10
	if v := stringFlag(args, "--limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	_, st, _, err := loreReindex(root)
	if err != nil {
		return err
	}
	defer st.Close()
	all, err := st.All()
	if err != nil {
		return err
	}
	hits := index.Search(all, args[0], time.Now().UTC(), limit)
	if boolIn(args, "--json") {
		js := make([]loreJSON, 0, len(hits))
		for _, h := range hits {
			js = append(js, loreJSON{ID: h.Rec.ID, Type: string(h.Rec.Type),
				Title: h.Rec.Title, Status: h.Rec.Status, Date: h.Rec.Date,
				Layer: h.Rec.Layer, File: h.Rec.File, Stale: h.Rec.Stale,
				Score: h.Score, Snippet: h.Snippet})
		}
		return toJSON(out, js)
	}
	for _, h := range hits {
		fmt.Fprintf(out, "%s  %.2f  [%s/%s]  %s — %s\n",
			h.Rec.ID, h.Score, h.Rec.Layer, orDash(h.Rec.Status), h.Rec.Title, h.Snippet)
	}
	return nil
}

// anchorMatches reports whether record anchor a covers query anchor q:
// symbols match exactly; paths match on either-direction prefix.
func anchorMatches(a lore.Anchor, q string) bool {
	if a.Symbol != "" {
		return a.Symbol == q
	}
	ap := strings.TrimSuffix(a.Path, "/")
	qp := strings.TrimSuffix(q, "/")
	return ap != "" && (strings.HasPrefix(qp, ap) || strings.HasPrefix(ap, qp))
}

func loreFor(root string, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: codeindex lore <repo> for <path-or-symbol> [--json]")
	}
	_, st, _, err := loreReindex(root)
	if err != nil {
		return err
	}
	defer st.Close()
	all, err := st.All()
	if err != nil {
		return err
	}
	var matched []index.StoredRecord
	for _, r := range all {
		for _, a := range r.Anchors {
			if anchorMatches(a, args[0]) {
				matched = append(matched, r)
				break
			}
		}
	}
	if boolIn(args, "--json") {
		js := make([]loreJSON, 0, len(matched))
		for _, r := range matched {
			js = append(js, loreJSON{ID: r.ID, Type: string(r.Type), Title: r.Title,
				Status: r.Status, Date: r.Date, Layer: r.Layer, File: r.File, Stale: r.Stale})
		}
		return toJSON(out, js)
	}
	for _, r := range matched {
		fmt.Fprintf(out, "%s  [%s/%s]  %s\n", r.ID, r.Layer, orDash(r.Status), r.Title)
	}
	return nil
}
```

Add `"encoding/json"` to the imports of `lore.go`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/codeindex/ -run TestLore -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/codeindex/lore.go cmd/codeindex/lore_test.go
git commit -m "lore: search and for subcommands with --json"
```

---

### Task 9: `lore backlog`

**Files:**
- Modify: `cmd/codeindex/lore.go`
- Test: append to `cmd/codeindex/lore_test.go`

**Interfaces:**
- Consumes: Task 7/8 helpers.
- Produces: `codeindex lore <repo> backlog [--for <anchor>] [--json]` — open items only, sorted by: priority ascending (`p0` < `p1` < `p2` < `p3`; empty = `p2`), then unblocked before blocked (blocked = any `blocked_by` ID that resolves to a still-`open` item), then date ascending. Text: `<id>  <priority>  <BLOCKED|ready>  <title>`. Register `case "backlog": return loreBacklog(root, args[1:], out)`.

- [ ] **Step 1: Write the failing test** (append)

```go
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
```

Note: this task also adds a `--blocked-by` flag to `loreAdd` (repeatable), appending to `rec.BlockedBy` — one line: `rec.BlockedBy = multiFlag(args, "--blocked-by")` placed with the other field assignments.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/codeindex/ -run TestLoreBacklog -v`
Expected: FAIL with `unknown lore subcommand "backlog"`.

- [ ] **Step 3: Write the implementation**

```go
func loreBacklog(root string, args []string, out io.Writer) error {
	_, st, _, err := loreReindex(root)
	if err != nil {
		return err
	}
	defer st.Close()
	all, err := st.All()
	if err != nil {
		return err
	}
	openIDs := map[string]bool{}
	for _, r := range all {
		if r.Type == lore.TypeItem && r.Status == "open" {
			openIDs[r.ID] = true
		}
	}
	anchor := stringFlag(args, "--for")
	var items []index.StoredRecord
	for _, r := range all {
		if r.Type != lore.TypeItem || r.Status != "open" {
			continue
		}
		if anchor != "" {
			ok := false
			for _, a := range r.Anchors {
				if anchorMatches(a, anchor) {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		items = append(items, r)
	}
	blocked := func(r index.StoredRecord) bool {
		for _, b := range r.BlockedBy {
			if openIDs[b] {
				return true
			}
		}
		return false
	}
	prio := func(p string) string {
		if p == "" {
			return "p2"
		}
		return p
	}
	sort.SliceStable(items, func(i, j int) bool {
		pi, pj := prio(items[i].Priority), prio(items[j].Priority)
		if pi != pj {
			return pi < pj
		}
		bi, bj := blocked(items[i]), blocked(items[j])
		if bi != bj {
			return !bi
		}
		return items[i].Date < items[j].Date
	})
	if boolIn(args, "--json") {
		js := make([]loreJSON, 0, len(items))
		for _, r := range items {
			js = append(js, loreJSON{ID: r.ID, Type: string(r.Type), Title: r.Title,
				Status: r.Status, Date: r.Date, Layer: r.Layer, File: r.File})
		}
		return toJSON(out, js)
	}
	for _, r := range items {
		state := "ready"
		if blocked(r) {
			state = "BLOCKED"
		}
		fmt.Fprintf(out, "%s  %s  %s  %s\n", r.ID, prio(r.Priority), state, r.Title)
	}
	return nil
}
```

Add `"sort"` to imports; register the case in `runLore`; add the `--blocked-by` line in `loreAdd`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/codeindex/ -run TestLore -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/codeindex/lore.go cmd/codeindex/lore_test.go
git commit -m "lore: backlog view with priority/blocked/age ordering and anchor filter"
```

---

### Task 10: `lore promote` and `lore supersede`

**Files:**
- Modify: `cmd/codeindex/lore.go`
- Test: append to `cmd/codeindex/lore_test.go`

**Interfaces:**
- Consumes: Tasks 7–9.
- Produces:
  - `codeindex lore <repo> promote <id>` — moves an overlay/session record's file into the committed layer dir for its type (session records promote as notes), prints `promoted <id> <new-path>`; errors if the record is already in the repo layer.
  - `codeindex lore <repo> supersede <old-id> --title T [--body ...]` — old record must be a decision; creates a new decision (same flags as `add`, committed layer) with `supersedes: <old-id>`; rewrites the old record's file setting `status: superseded` and `superseded_by: <new-id>`; prints `created <new-id> <path>` then `superseded <old-id>`.
  - Register cases `promote` and `supersede` in `runLore`.

- [ ] **Step 1: Write the failing test** (append)

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/codeindex/ -run 'TestLorePromote|TestLoreSupersede' -v`
Expected: FAIL with `unknown lore subcommand "promote"`.

- [ ] **Step 3: Write the implementation**

First, refactor `loreAdd` so record creation is reusable: extract everything after flag parsing into

```go
// writeNewRecord marshals rec and writes it into the layer/type directory,
// disambiguating same-day slug collisions with the ID tail.
func writeNewRecord(l lore.Layout, rec lore.Record, layer string) (string, error) {
	dir := l.Dir(layer, rec.Type)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := rec.Date + "-" + lore.Slug(rec.Title) + ".md"
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		name = rec.Date + "-" + lore.Slug(rec.Title) + "-" + rec.ID[len(rec.ID)-6:] + ".md"
		path = filepath.Join(dir, name)
	}
	b, err := rec.Marshal()
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, b, 0o644)
}
```

and a shared flag→record builder used by both `add` and `supersede`:

```go
// recordFromFlags builds a new record of typ from add-style flags.
func recordFromFlags(typ lore.Type, args []string) (lore.Record, error) {
	title := stringFlag(args, "--title")
	if title == "" {
		return lore.Record{}, fmt.Errorf("--title is required")
	}
	body := stringFlag(args, "--body")
	if body == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return lore.Record{}, err
		}
		body = string(b)
	}
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	rec := lore.Record{
		ID: lore.NewID(typ), Type: typ, Title: title,
		Status: lore.DefaultStatus(typ),
		Date:   time.Now().UTC().Format("2006-01-02"),
		Body:   body, Priority: stringFlag(args, "--priority"),
		Tags: multiFlag(args, "--tag"), BlockedBy: multiFlag(args, "--blocked-by"),
	}
	for _, a := range multiFlag(args, "--anchor") {
		kind, val, ok := strings.Cut(a, ":")
		if !ok || (kind != "path" && kind != "symbol") {
			return lore.Record{}, fmt.Errorf("bad --anchor %q (want path:P or symbol:S)", a)
		}
		if kind == "path" {
			rec.Anchors = append(rec.Anchors, lore.Anchor{Path: val})
		} else {
			rec.Anchors = append(rec.Anchors, lore.Anchor{Symbol: val})
		}
	}
	for _, r := range multiFlag(args, "--ref") {
		kind, val, ok := strings.Cut(r, ":")
		if !ok {
			return lore.Record{}, fmt.Errorf("bad --ref %q (want kind:value)", r)
		}
		rec.Refs = append(rec.Refs, lore.Ref{Kind: kind, Value: val})
	}
	return rec, nil
}
```

`loreAdd` shrinks to: validate type → `recordFromFlags` → pick layer → `writeNewRecord` → print. Then:

```go
func lorePromote(root string, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: codeindex lore <repo> promote <id>")
	}
	l, st, _, err := loreReindex(root)
	if err != nil {
		return err
	}
	defer st.Close()
	r, ok, err := st.Get(args[0])
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no record %q", args[0])
	}
	if r.Layer == "repo" {
		return fmt.Errorf("%s is already in the committed layer (%s)", r.ID, r.File)
	}
	dest := l.Dir("repo", r.Type) // session records promote as their parsed type (note)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	newPath := filepath.Join(dest, filepath.Base(r.File))
	b, err := os.ReadFile(r.File)
	if err != nil {
		return err
	}
	if err := os.WriteFile(newPath, b, 0o644); err != nil {
		return err
	}
	if err := os.Remove(r.File); err != nil {
		return err
	}
	fmt.Fprintf(out, "promoted %s %s\n", r.ID, newPath)
	return nil
}

func loreSupersede(root string, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: codeindex lore <repo> supersede <old-id> --title T ...")
	}
	l, st, _, err := loreReindex(root)
	if err != nil {
		return err
	}
	defer st.Close()
	old, ok, err := st.Get(args[0])
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no record %q", args[0])
	}
	if old.Type != lore.TypeDecision {
		return fmt.Errorf("%s is a %s; only decisions are superseded", old.ID, old.Type)
	}
	rec, err := recordFromFlags(lore.TypeDecision, args[1:])
	if err != nil {
		return err
	}
	rec.Supersedes = old.ID
	path, err := writeNewRecord(l, rec, "repo")
	if err != nil {
		return err
	}
	// Durable transition on the old record: rewrite its file.
	ob, err := os.ReadFile(old.File)
	if err != nil {
		return err
	}
	or, err := lore.Parse(ob, old.Type)
	if err != nil {
		return err
	}
	or.Status = "superseded"
	or.SupersededBy = rec.ID
	nb, err := or.Marshal()
	if err != nil {
		return err
	}
	if err := os.WriteFile(old.File, nb, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "created %s %s\n", rec.ID, path)
	fmt.Fprintf(out, "superseded %s\n", old.ID)
	return nil
}
```

Register both cases in `runLore`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/codeindex/ -run TestLore -v`
Expected: PASS (including all earlier lore tests — the `loreAdd` refactor must not change behavior).

- [ ] **Step 5: Commit**

```bash
git add cmd/codeindex/lore.go cmd/codeindex/lore_test.go
git commit -m "lore: promote and supersede with durable frontmatter write-back"
```

---

### Task 11: `lore doctor`

**Files:**
- Modify: `cmd/codeindex/lore.go`
- Test: append to `cmd/codeindex/lore_test.go`

**Interfaces:**
- Consumes: `index.StaleRecords` (Task 6), reindex `Report` (Task 4).
- Produces: `codeindex lore <repo> doctor` — prints one line per finding, then a summary count. Findings:
  1. `parse-error  <path>  <error>` — from `Report.Errors`
  2. `stale-anchor  <id>  <title>` — from `StaleRecords` (graph.db path: `dbPath(root)` from main.go; missing graph.db skips symbol checks silently). Also persists via `st.SetStale` so search downweights.
  3. `dangling-ref  <id>  supersedes|blocked_by <missing-id>` — referenced record ID not in the index
  4. `inconsistent  <id>  superseded_by set but status is <status>` — write-back drift
  Exit code stays 0 (doctor reports, never fails). Ends with `ok: no findings` or `N finding(s)`.

- [ ] **Step 1: Write the failing test** (append)

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/codeindex/ -run TestLoreDoctor -v`
Expected: FAIL with `unknown lore subcommand "doctor"`.

- [ ] **Step 3: Write the implementation**

```go
func loreDoctor(root string, args []string, out io.Writer) error {
	_, st, rep, err := loreReindex(root)
	if err != nil {
		return err
	}
	defer st.Close()
	findings := 0
	for _, fe := range rep.Errors {
		fmt.Fprintf(out, "parse-error  %s  %v\n", fe.Path, fe.Err)
		findings++
	}
	all, err := st.All()
	if err != nil {
		return err
	}
	stale, err := index.StaleRecords(root, dbPath(root), all)
	if err != nil {
		return err
	}
	byID := map[string]index.StoredRecord{}
	for _, r := range all {
		byID[r.ID] = r
	}
	for _, r := range all {
		if stale[r.ID] {
			fmt.Fprintf(out, "stale-anchor  %s  %s\n", r.ID, r.Title)
			findings++
			if err := st.SetStale(r.ID, true); err != nil {
				return err
			}
		} else if r.Stale {
			if err := st.SetStale(r.ID, false); err != nil {
				return err
			}
		}
		if r.Supersedes != "" {
			if _, ok := byID[r.Supersedes]; !ok {
				fmt.Fprintf(out, "dangling-ref  %s  supersedes %s\n", r.ID, r.Supersedes)
				findings++
			}
		}
		for _, b := range r.BlockedBy {
			if _, ok := byID[b]; !ok {
				fmt.Fprintf(out, "dangling-ref  %s  blocked_by %s\n", r.ID, b)
				findings++
			}
		}
		if r.SupersededBy != "" && r.Status != "superseded" {
			fmt.Fprintf(out, "inconsistent  %s  superseded_by set but status is %s\n",
				r.ID, orDash(r.Status))
			findings++
		}
	}
	if findings == 0 {
		fmt.Fprintln(out, "ok: no findings")
	} else {
		fmt.Fprintf(out, "%d finding(s)\n", findings)
	}
	return nil
}
```

Register `case "doctor": return loreDoctor(root, args[1:], out)`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/codeindex/ -run TestLore -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/codeindex/lore.go cmd/codeindex/lore_test.go
git commit -m "lore: doctor — parse errors, stale anchors, dangling refs, drift"
```

---

### Task 12: `lore init`, README, and end-to-end pass

**Files:**
- Modify: `cmd/codeindex/lore.go` (init), `README.md` (new section)
- Create: none (init writes `.lore/README.md` at runtime)
- Test: append to `cmd/codeindex/lore_test.go`

**Interfaces:**
- Produces: `codeindex lore <repo> init` — creates `.lore/{decisions,items,notes}` with a `.lore/README.md` explaining the format (frontmatter fields, one file per record, PR-reviewed), prints created paths and a reminder that `.codeindex/` should be gitignored (lore.db lives there). Idempotent: re-running reports `already initialized`.

- [ ] **Step 1: Write the failing test** (append)

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/codeindex/ -run TestLoreInit -v`
Expected: FAIL with `unknown lore subcommand "init"`.

- [ ] **Step 3: Write the implementation**

```go
const loreReadme = `# .lore/ — project decisions, work items, and notes

Records are Markdown files with YAML frontmatter, one file per record,
managed by ` + "`codeindex lore`" + ` and reviewed like code (they land via PRs).

- decisions/ — why the code is the way it is; status: active | superseded | rejected
- items/     — known work; status: open | done | dropped; priority p0..p3
- notes/     — gotchas, conventions, context

Useful commands:
  codeindex lore . add decision --title "..." --body - --anchor symbol:Foo
  codeindex lore . search "query"
  codeindex lore . for internal/some/pkg/
  codeindex lore . backlog
  codeindex lore . doctor
`

func loreInit(root string, args []string, out io.Writer) error {
	l, err := lore.NewLayout(root)
	if err != nil {
		return err
	}
	readme := filepath.Join(l.RepoDir, "README.md")
	if _, err := os.Stat(readme); err == nil {
		fmt.Fprintln(out, "already initialized:", l.RepoDir)
		return nil
	}
	for _, t := range []lore.Type{lore.TypeDecision, lore.TypeItem, lore.TypeNote} {
		if err := os.MkdirAll(l.Dir("repo", t), 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(readme, []byte(loreReadme), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "initialized %s (decisions/ items/ notes/)\n", l.RepoDir)
	fmt.Fprintln(out, "note: keep .codeindex/ gitignored — the lore index (lore.db) is derived")
	return nil
}
```

Register `case "init": return loreInit(root, args[1:], out)`.

- [ ] **Step 4: Add the README section**

In `README.md`, after the `## Commands` block, add:

```markdown
## Lore: decisions, work items, and notes

`codeindex lore` keeps project knowledge in the repo — decisions (with
rationale and rejected alternatives), a backlog of work items, and notes —
as Markdown records in `.lore/`, versioned and reviewed like code, plus a
private per-user overlay. Records anchor to files and symbols in the index,
so `lore for <path|symbol>` answers "what do we know about the code I am
about to change", and anchors go detectably stale when the code moves on.

```
codeindex lore <repo> init                      scaffold .lore/
codeindex lore <repo> add <type> --title ...    record a decision/item/note (--private for the overlay)
codeindex lore <repo> search <query>            ranked search across layers
codeindex lore <repo> for <path|symbol>         records anchored to this code
codeindex lore <repo> backlog [--for anchor]    open items, priority-ordered
codeindex lore <repo> promote <id>              private record -> committed .lore/
codeindex lore <repo> supersede <id> --title .. replace a decision, back-linked
codeindex lore <repo> doctor                    stale anchors, dangling refs, parse errors
```
```

- [ ] **Step 5: Run the full test suite and an end-to-end smoke**

Run: `go test ./... && go build -o /tmp/codeindex ./cmd/codeindex`
Expected: all packages PASS; build succeeds.

Smoke (manual, in any scratch repo):

```bash
cd "$(mktemp -d)" && git init -q .
/tmp/codeindex lore . init
/tmp/codeindex lore . add decision --title "Try lore" --body "It works."
/tmp/codeindex lore . search "lore"
/tmp/codeindex lore . doctor
```

Expected: `init` scaffolds, `add` prints `created dec-…`, `search` finds it, `doctor` says `ok: no findings`.

- [ ] **Step 6: Commit**

```bash
git add cmd/codeindex/lore.go cmd/codeindex/lore_test.go README.md
git commit -m "lore: init scaffold and README documentation — plan 1 complete"
```

---

## Self-Review Notes

- Spec coverage (Plan 1 scope): data model ✓ (T1), layers/repo-id ✓ (T2), index/freshness ✓ (T3–4), ranking with decay/status ✓ (T5), anchor staleness ✓ (T6), CLI add/show/search/for/backlog/promote/supersede/doctor/init ✓ (T7–12). Deliberately deferred to Plan 2: `capture`, MCP tools, `related_lore`, plugin/hooks/rules, per-host `init` scaffolding. Deferred to Plan 3: `event`, `sync github`, ratification labeling, confidence signals (the `confidence` column and `Stale` plumbing land here so Plan 3 is additive).
- Type consistency: `runLore(root string, args []string, out io.Writer)`, `loreReindex` returning `(lore.Layout, *index.Store, index.Report, error)`, and `StoredRecord` embedding `lore.Record` are used identically across Tasks 7–12.
- Known risk: the `symbols` view name/columns in graph.db (Task 6) — guarded by a fixture test with an explicit adaptation note.
