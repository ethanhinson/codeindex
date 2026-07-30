# Lore Lifecycle Signals (Plan 3 of 3) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Records respond to what happens to the code: ratification-by-merge labeling, `closes <item-id>` commit references driving durable item transitions, survival/churn confidence feeding search ranking, `lore event` CI ingestion, and `lore sync github` / `lore push` via the `gh` CLI. Also lands the two integrity items the earlier reviews sequenced ahead of this work: the open wire struct and duplicate-ID visibility.

**Architecture:** All git-derived signals are computed during the lazy reindex from `git log` output (shell-out via an injectable runner — no go-git dependency, matching `RepoID`'s existing pattern). The durable/derived split from the spec is strict: `closes` transitions and sync results rewrite record frontmatter (git-auditable); ratification and confidence live only in `lore.db` columns (recomputed, wipeable). No daemons, no webhooks.

**Tech Stack:** Go (existing deps only), `git` and `gh` CLIs at runtime (both optional — every signal degrades to no-op without them).

**Spec:** `docs/superpowers/specs/2026-07-29-lore-engine-design.md` (Lifecycle signals & evidence; Backlog & third-party integration). Backlog items: `itm-01KYR17XECP2YV5YBQ7VT87NQF` (this plan), `itm-01KYRS1YSW68DHC9QKNRD0DNX6` (wire struct, Task 1), `itm-01KYRS1YSWEV1CCEB1X8SGMDJ1` (dup-ID, Task 2).

## Global Constraints

- Module path `codeindex`. No new Go dependencies. No go-git: all git/gh access shells out through `internal/lore/gitinfo`'s injectable `Runner` so tests never spawn real processes (except gitinfo's own integration tests, which build throwaway repos with real `git` like `layout_test.go` does).
- Fail-open: a repo without git, a git without an origin, or a missing `gh` binary silently disables the corresponding signal — never an error to the user (doctor may *report* absence, commands still exit 0). `sync`/`push` are the exception: the user invoked them explicitly, so their failures are real errors.
- Durable vs derived: `closes`/sync write frontmatter via Parse→Marshal round-trip (safe after Task 1); ratification (`ratified` column) and `confidence` are index-only, recomputed each scan, and remain deliberately absent from `Upsert`'s conflict clause.
- Git scans are incremental: `lore_meta` key `last_scanned_commit` bounds every `git log` walk; unbounded first scan capped at 500 commits.
- Labels in text output: unratified repo-layer records get a trailing `  UNRATIFIED`; stale keeps `  STALE`; both may appear.
- Tests: `go test ./internal/lore/... ./internal/mcpserver/ ./cmd/codeindex/` — no tags. Commit per task with `lore:` prefix.

---

### Task 1: Open wire struct — carry unknown frontmatter keys

**Files:** Modify `internal/lore/record.go`; test in `internal/lore/record_test.go`.

**Interfaces:** `Record` gains `Extra map[string]any` (nil when none). `Parse` captures unknown frontmatter keys into `Extra`; `Marshal` re-emits them AFTER the known fields. Closes `itm-01KYRS1YSW68DHC9QKNRD0DNX6`.

- [ ] **Step 1 (RED):** Test: parse a decision whose frontmatter includes `hook: "future field"` and `claimed_at: 2026-08-01T00:00:00Z`; assert `Extra["hook"] == "future field"`; Marshal → Parse again; assert both keys survive byte-level (`strings.Contains(marshaled, "hook:")`) and in `Extra`. Also assert existing round-trip tests still pass (no `Extra` pollution with known keys).
- [ ] **Step 2:** Implement via a second unmarshal pass: after decoding into `wire`, unmarshal the frontmatter into `map[string]any`, delete every known key (keep a package-level `knownKeys` slice derived from the wire struct's yaml tags — write it literally and add a test that asserts `len(knownKeys)` matches the wire struct's field count so a future field can't be forgotten), and store the remainder as `Extra`. In `Marshal`, after `yaml.Marshal(w)`, append `yaml.Marshal(r.Extra)` when non-empty (yaml maps sort keys deterministically).
- [ ] **Step 3 (GREEN + commit):** `go test ./internal/lore/ && go vet ./internal/lore/` → `git commit -m "lore: carry unknown frontmatter keys through Parse/Marshal (open wire struct)"`. Then flip `itm-01KYRS1YSW68DHC9QKNRD0DNX6` to `status: done` with a one-line completion note and include it in the same commit.

---

### Task 2: Duplicate-ID visibility

**Files:** Modify `internal/lore/index/reindex.go` (Report gains `Duplicates []string` — "id: path1, path2"), `cmd/codeindex/lore.go` (doctor prints `duplicate-id  <id>  <paths>` findings). Tests in both packages.

- [ ] **Step 1 (RED):** Reindex test: two files (repo + overlay layer) carrying the same ID → `Report.Duplicates` has one entry naming both paths; the LAST-upserted file owns the index row (existing behavior, now asserted). Doctor CLI test: same setup surfaces a `duplicate-id` finding line and counts it.
- [ ] **Step 2:** Implement: `Reindex` tracks `idToPath map[string]string` across ALL scanned files (including unchanged ones — the map must be complete, so populate it from parsing changed files AND from `lore_records`' file column for unchanged ones via one query). On collision append to `rep.Duplicates`. Doctor iterates `rep.Duplicates`.
- [ ] **Step 3 (GREEN + commit):** full lore+cmd tests → `git commit -m "lore: surface duplicate-ID records in reindex report and doctor"`. Flip `itm-01KYRS1YSWEV1CCEB1X8SGMDJ1` to done in the same commit.

---

### Task 3: gitinfo — injectable git access

**Files:** Create `internal/lore/gitinfo/gitinfo.go`, `gitinfo_test.go`.

**Interfaces (consumed by Tasks 4–6):**

```go
type Runner func(dir string, args ...string) (string, error) // default: exec git
func New(repoRoot string) *Git                                // uses default runner
func NewWithRunner(repoRoot string, r Runner) *Git            // tests
func (g *Git) Available() bool                                // git binary + .git present
func (g *Git) DefaultBranch() string                          // origin/HEAD → "main" fallback
func (g *Git) FileOnBranch(branch, relPath string) bool       // git cat-file -e <branch>:<path>
func (g *Git) Head() (string, error)
// CommitsSince returns commits newer than sinceSHA (all reachable capped at
// limit when sinceSHA == ""), oldest first, with per-file added/deleted line
// counts. Parses: git log --reverse --numstat --format=%H%x00%s <since>..HEAD
type Commit struct { SHA, Subject string; Files map[string][2]int }
func (g *Git) CommitsSince(sinceSHA string, limit int) ([]Commit, error)
```

- [ ] **Step 1 (RED):** Unit tests with a fake Runner returning canned `git log --numstat` output (two commits, multi-file, binary-file `-` numstat lines skipped) asserting parse correctness; `DefaultBranch` fallback when the symbolic-ref call errors; `FileOnBranch` true/false mapping from runner error. One integration test building a real temp git repo (init, two commits with a `closes` subject) asserting `CommitsSince("", 10)` order and file stats — skip with `t.Skip` if `git` absent.
- [ ] **Step 2:** Implement. Numstat parsing: records start at `%H\x00%s` lines; numstat lines are `added\tdeleted\tpath` (`-` for binary → skip). `%x00` separator avoids subject-line ambiguity.
- [ ] **Step 3 (GREEN + commit):** `lore: gitinfo — injectable git log/branch access for signals`.

---

### Task 4: Ratification labeling

**Files:** Modify `internal/lore/index/store.go` (schema: `ratified INTEGER NOT NULL DEFAULT 1` on lore_records — default ratified so non-git repos never flag; bump `schemaVersion` to 2, the self-healing wipe rebuilds), `reindex.go` (after the file walk, when `gitinfo.Available()`: for each repo-layer record, `ratified = FileOnBranch("origin/"+DefaultBranch(), relPath)`; overlay/session always ratified), `StoredRecord` gains `Ratified bool`. Label plumbing: CLI `search`/`for`/`backlog` text lines and `loreJSON` (`unratified,omitempty` inverted field: emit `"unratified": true`), MCP `formatRecords`, both appending `  UNRATIFIED` when `!Ratified`.

- [ ] **Step 1 (RED):** Reindex test with fake runner: a record file reported absent on the default branch → `Ratified == false`; overlay record with same runner → true; no-git repo (Available false) → true. CLI test: unratified record's search line contains `UNRATIFIED`. (Wire a test seam: `Reindex` uses `gitinfo.New` by default — add package var `newGit = gitinfo.New` in `index` and override in tests, matching Go stdlib test-seam convention.)
- [ ] **Step 2:** Implement; keep the git pass AFTER hash-diff upserts (labels apply to all records each scan — cheap: one FileOnBranch per repo-layer record, only when git is available and origin exists).
- [ ] **Step 3 (GREEN + commit):** all packages + `go vet ./...` → `lore: ratification-by-merge labeling (UNRATIFIED on branch-only records)`.

---

### Task 5: `closes <item-id>` transitions + incremental scan state

**Files:** Modify `internal/lore/index/reindex.go` (new `scanSignals` step), `store.go` (lore_meta get/set helpers `Meta(key)`, `SetMeta(key, val)`).

**Behavior:** After the file walk, when git is available: `CommitsSince(Meta("last_scanned_commit"), 500)`. For each commit whose subject matches `(?i)\bcloses\s+(itm-[0-9A-Z]{26})\b` (compile once; multiple matches per subject allowed): if that item exists and `Status == "open"`, rewrite its FILE durably — Parse, set `Status = "done"`, append `Ref{Kind: "commit", Value: shortSHA}`, Marshal, WriteFile — then re-upsert. Skipped silently when the item is missing or not open (idempotent on rescan). Finally `SetMeta("last_scanned_commit", Head())`. Report gains `Closed []string` (item IDs); doctor prints nothing for these (they're normal operations), but `Reindex` callers already surface counts via existing patterns — add `closed <id> by <sha>` lines to CLI `doctor` output? No — keep doctor for problems; the transition is visible in `git diff` of the record. YAGNI.

- [ ] **Step 1 (RED):** With fake runner returning a commit `closes itm-XXXX...`: open item file flips to `status: done` on disk with the commit ref appended (Parse the rewritten file to assert), index row updated, `last_scanned_commit` advanced; second Reindex with runner returning no new commits → no rewrite (mtime/hash unchanged assertion). Non-open item and unknown ID cases: untouched.
- [ ] **Step 2:** Implement inside `index` (it owns reindex); the file rewrite goes through `lore.Parse`/`Marshal` (Task 1 makes this future-proof).
- [ ] **Step 3 (GREEN + commit):** `lore: closes <item-id> commit transitions with incremental scan state`.

---

### Task 6: Survival/churn confidence

**Files:** Modify `internal/lore/index/reindex.go` (same signals pass), `search.go` (confidence multiplier), `cmd/codeindex/lore.go` (`show` prints a confidence line; doctor gains `churn-suspect` finding).

**Behavior (concrete v1 formulas):** For each record with path anchors (symbol-anchor churn deferred — path anchors cover the dogfood corpus): from the SAME `CommitsSince` batch as Task 5, accumulate per-record `survived += 1` for each commit touching an anchored path while the record was not itself modified in that commit, and `churnLines += added+deleted` for anchored paths. Persist running totals in two new lore_records columns (`survived INTEGER DEFAULT 0`, `churn_lines INTEGER DEFAULT 0` — same schema bump as Task 4 if sequenced together; Tasks 4–6 share `schemaVersion = 2`). Derived values: `confidence = ln(1+survived)/ln(21)` capped at 1.0 (survived 20 merges → full confidence); `churn-suspect` when `churnLines > 3× the current total line count` of anchored files (cheap `wc`-equivalent count at scan time; 3× because added+deleted double-counts rewrites). Search: multiply score by `0.8 + 0.4×confidence` (range 0.8–1.2 — evidence nudges, never dominates). `show` meta gains `confidence: 0.43 (survived 9 commits)` when survived > 0.

- [ ] **Step 1 (RED):** Fake-runner tests: commits touching anchored path increment survived and churn; commit that also touches the record's own file does NOT increment survived; churn-suspect doctor finding fires past threshold; search ordering flips between two otherwise-equal records when one has high confidence.
- [ ] **Step 2:** Implement; single git batch shared with Task 5's scan (one `CommitsSince` call per reindex).
- [ ] **Step 3 (GREEN + commit):** `lore: survival/churn confidence feeding ranking and doctor`.

---

### Task 7: `lore event` ingestion

**Files:** Modify `cmd/codeindex/lore.go` (subcommand), `internal/lore/index/store.go` (lore_events table: sha, type, status, detail, created), `internal/lore/index/reindex.go` (events load), `show` display.

**Behavior:** `codeindex lore <repo> event --type deploy --status ok|failed [--commit <sha>] [--detail <text>]` appends one JSON line to `<OverlayDir>/events.jsonl` (durable, survives db wipe) and exits 0 silently on any storage failure (CI must not break). Reindex ingests events.jsonl into lore_events (hash-diffed like record files). `show <id>` lists events whose sha prefix-matches any of the record's `commit` refs: `event: deploy ok (a1b2c3d)`. No confidence coupling in v1 (spec's "attaches evidence" satisfied by display; boost deferred until events exist in practice). `--commit` defaults to `gitinfo.Head()` when available.

- [ ] **Step 1 (RED):** CLI test writes an event, asserts the JSONL line; reindex ingests it; `show` on a record with the matching commit ref displays it; a second identical reindex is a no-op.
- [ ] **Step 2 + 3:** Implement → `lore: event ingestion (deploy/CI evidence via events.jsonl)`.

---

### Task 8: `lore sync github` and `lore push` + docs + close-out

**Files:** Modify `cmd/codeindex/lore.go`, create `internal/lore/ghsync/ghsync.go` + test (same injectable Runner pattern as gitinfo, wrapping `gh`), README Lore section.

**Behavior:**
- `lore sync github`: for every item with a `gh-issue` ref (`owner/repo#N` or bare `#N` against the origin repo): `gh issue view N --repo owner/repo --json state,stateReason`. Closed issue + open item → durable write-back `status: done` (+ existing ref untouched), print `synced <id> done (issue #N closed)`. Open issue: no-op. Missing `gh` or auth failure → real error (explicit command).
- `lore push <id>`: item must have NO existing gh-issue ref; `gh issue create --title <title> --body <body+backlink line "lore: <id>">` → parse the returned issue URL → append `gh-issue` ref durably, print `pushed <id> <url>`.
- README "Third-party sync" subsection documenting both + the zero-integration tier (skills instruct agents to record refs when they file tickets via host tools).

- [ ] **Step 1 (RED):** ghsync unit tests with fake runner: issue-view JSON parsing (closed/open/malformed), issue-create URL extraction; CLI tests with the seam: sync flips a done item durably (Parse the file), push appends the ref and refuses when a ref exists.
- [ ] **Step 2:** Implement (`newGH` seam var in cmd, mirroring Task 4's pattern).
- [ ] **Step 3 (GREEN + full close-out):** `go test ./... && go vet ./... && go build -o /tmp/codeindex-p3 ./cmd/codeindex`. Dogfood smoke on this repo: `lore . backlog` (labels sane), `lore . show <a decision>` (confidence line appears after some scans), `lore . doctor` (clean or explainable), and a real `closes` rehearsal in a scratch clone (commit "closes <one seed item id>" → reindex → file flipped). README updated → `lore: gh sync/push and third-party docs — plan 3 complete`.

---

## Self-Review Notes

- Spec coverage: ratification ✓ (T4), closes-transitions ✓ (T5), survival/churn confidence ✓ (T6, with the concrete formulas the spec left open), events ✓ (T7, display-only evidence per YAGNI), gh reconcile/push ✓ (T8). Integrity items sequenced first ✓ (T1–T2 close their backlog items in-commit — the workflow's first in-commit item closures).
- Deviations, all narrowing: symbol-anchor churn deferred (path anchors suffice for dogfood); events don't feed confidence yet; `closes` scans commit subjects only (not bodies) — each is one sentence in the record when someone hits the limit.
- Schema: Tasks 4 and 6 share one `schemaVersion` bump to 2; the self-healing wipe makes migration free.
- Test seams (`newGit`, `newGH` package vars) keep every unit test process-free; only gitinfo's integration test spawns real git, skip-guarded.
