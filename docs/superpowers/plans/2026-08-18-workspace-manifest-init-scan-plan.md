<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0009 — Workspace manifest load/validate + init-workspace --scan](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0009-workspace-manifest-init-scan.md)**
<!-- docket:backlink:end -->

# Plan: Workspace manifest load/validate + `init-workspace --scan`

Change 0009 `workspace-manifest-init-scan` · openspec task §3.1 ·
Spec: `docs/superpowers/specs/2026-08-18-workspace-manifest-init-scan-design.md`
(on the `docket` branch — the spec is authoritative for every design question;
this plan only sequences the work).

> **Plan-role degradation:** the configured plan skill
> (`superpowers:writing-plans`) is not available in this harness, so this plan
> file was authored directly under the docket Skill-layer `auto` fallback.

## Ground rules for every task

- **TDD.** Write the focused test first, watch it fail for the right reason,
  then implement. Each task ends in exactly one commit.
- **Additive only.** The single exception is Task 6 (`main.go` switch case +
  usage string) and Task 8 (`.gitignore`). No existing behavior changes; the
  regression bar is the **whole single-repo golden suite** —
  `go test ./...` green, and `TestCallersTextGolden`, `TestCalleesTextGolden`,
  `TestNavTextGolden` (`internal/query/query_test.go:43,113,140`)
  byte-identical.
- **The spec's Assumptions table is binding**, as are the owner decisions in
  the change's `## Groom context`. Do not re-litigate either; if a task looks
  like it needs a design decision, the answer is already in the spec.
- Fixture trees live under the package's own `testdata/`. The 10 bench corpus
  checkouts are **not** in the repo — never write a `go test` case that reads
  `bench/repos/`.

---

## Task 1 — Manifest types + `LoadWorkspace` validation

**Files:** `internal/config/workspace.go` (new),
`internal/config/workspace_test.go` (new).

Implement `WorkspaceFile`, `Workspace`, `Member`, and `LoadWorkspace` exactly
as spec §1 declares them.

Validation rules — every row of spec §1's table is an error, and every row
gets its own test case:

| Case | Expectation |
|---|---|
| `version` absent (`0`) | error; its **own** message, not fused with a parse failure |
| `version: 2` | `"unsupported workspace version %d"` (mirrors `internal/runtime/runtime.go:70`) |
| `members` empty | error |
| `id` empty, or a rune outside `[A-Za-z0-9._-]` | error |
| duplicate `id` | error |
| `root` empty, or `filepath.IsAbs(root)` | error |
| duplicate `root` after `filepath.Clean` | error |
| `root` containing `../` | **accepted** (D1 dedicated-workspace-directory shape) |

Two further tests, both load-bearing:

- **`LoadWorkspace` never stats a member root** — load a manifest whose
  members all point at nonexistent paths and assert **success**. This keeps
  the frozen coverage-clause scenario (`spec.md:117-122`) reachable.
- **Missing manifest** ⇒ `errors.Is(err, fs.ErrNotExist)` holds.

Unknown JSON fields are ignored (plain `encoding/json`, no
`DisallowUnknownFields`) — matches `Config`'s posture.

**Done when:** `go test ./internal/config/...` green.

---

## Task 2 — `SaveWorkspace` + `Resolve`

**Files:** same two as Task 1.

- `SaveWorkspace(wsRoot, ws)` — writes the slice **in the order given**,
  `json.MarshalIndent` two-space, trailing newline, creating `.codeindex/` if
  absent. **It carries no ordering policy** — assert this by saving a
  deliberately unsorted slice and reading back the same order.
- `Resolve(wsRoot)` — stats each member root; returns present/missing. Test it
  against the Task-1 nonexistent-roots manifest: `LoadWorkspace` succeeds and
  `Resolve` names exactly those members as missing.
- Round-trip test: load → save → load is stable.

`Resolve` ships with **no call site** in this slice (spec Assumption 13); that
is intentional, not dead code to delete.

**Done when:** `go test ./internal/config/...` green.

---

## Task 3 — New module dependencies

**Files:** `go.mod`, `go.sum`.

Add `golang.org/x/mod` (go.mod/go.work parsing) and `gopkg.in/yaml.v3`
(pnpm-workspace.yaml) as **direct** requires; run `go mod tidy`; commit
`go.sum`. Both are pure Go, so ADR-0003 (single static binary) is unaffected
and no new ADR is warranted.

**Done when:** `go build ./...` succeeds with `CGO` settings unchanged and
`go mod tidy` leaves the tree clean.

---

## Task 4 — `Namespaces` per-language discovery

**Files:** `internal/workspace/namespaces.go` (new),
`internal/workspace/namespaces_test.go` (new), fixtures under
`internal/workspace/testdata/`.

`func Namespaces(memberRoot string) ([]string, error)` runs every probe and
returns the concatenation, **deduplicated and sorted**. An absent marker file
contributes nothing and is **not** an error.

- **Go** — `go.mod` `module` path via `golang.org/x/mod/modfile.Parse`. Not a
  hand-rolled line scan: `module` may be quoted or inside a block.
- **TS/JS** — `package.json` `name`.
- **PHP** — composer `name` **plus every `autoload.psr-4` key**. The psr-4
  value must decode as **string-or-array** (`map[string]json.RawMessage` and
  branch on the first byte, or `map[string]any` and type-switch). A
  `map[string]string` decode fails outright on laravel-framework, where
  `Illuminate\Support\` maps to three paths — the fixture **must** include a
  multi-path array value.
- **Python** — top-level importable modules, probing **`src/` first** with a
  root fallback. A candidate is a directory containing `__init__.py`, or a
  top-level `*.py` (module name = basename sans extension). Skip
  `test`/`tests`/`docs`/`examples` and dot-prefixed entries. Both corpus
  Python members are src-layout; a root-only rule silently zeroes 12 of the 65
  bench tasks, so the src-first fixture is mandatory.

**Done when:** one fixture tree per language passes, including the psr-4
multi-path array and the Python src-layout case.

---

## Task 5 — `Members` monorepo discovery + the three guards

**Files:** `internal/workspace/members.go` (new),
`internal/workspace/members_test.go` (new), fixtures under `testdata/`.

`func Members(root string) ([]config.Member, error)` reads **five** sources
(spec §2b): `go.work` (`modfile.ParseWork`), `pnpm-workspace.yaml`
(`yaml.v3`), `composer.json` `repositories[]` with `"type":"path"` → `url`,
`lerna.json` `packages`, and `package.json` `workspaces` — **both** the array
form and the object form `{"packages": [...]}`.

`lerna.json` + `package.json workspaces` are the round-2 owner amendment to
D1; the corpus's only monorepo (nest) declares members **solely** via
`lerna.json`.

Expand patterns with `filepath.Glob` relative to `root`, then apply the three
over-collection guards **in order**:

1. **Directories only** — drop non-directory hits, and drop any candidate
   whose path traverses a `config.DefaultExcludeDirs` basename
   (`node_modules`, `vendor`, `dist`, …). A glob is not the repo filter, so
   the exclusion is applied explicitly.
2. **Marker at the candidate's own top level** — the candidate must itself
   contain `go.mod` / `package.json` / `composer.json` / a Python marker. One
   level down does not qualify it.
3. **Subset suppression** — drop a candidate whose discovered namespace set is
   a subset of an already-declared member's. This is what removes a monorepo's
   own root package when it re-exports its children.

Discovered **id** = candidate basename, sanitized to `[A-Za-z0-9._-]` (every
other rune → `-`), lowercased; `-2`, `-3`, … on collision in expansion order.
Ids are derived **only for members the scan introduces**.

Tests: one per declaration source (five), plus **one fixture per guard** — a
file matching the glob, a `node_modules` hit, a marker one level below the
candidate, and a subset-namespace root package.

> Spec §2b's scope note applies: this path gets **no bench coverage** in this
> slice; these fixture tests are its only coverage, so do not thin them.

**Done when:** all five sources and all three guards are covered and green.

---

## Task 6 — `Scan` + `Merge`

**Files:** `internal/workspace/scan.go` (new), `internal/workspace/scan_test.go`
(new).

### `func Scan(wsRoot string, authored []config.Member) ([]config.Member, error)`

**Two independent passes** (spec §3a — this is the load-bearing detail):

1. **Member discovery** — `Members(wsRoot)` at the workspace root. On a
   dedicated workspace directory this legitimately returns **zero**.
2. **Namespace pass** — runs over **every authored member root** *and* every
   member pass 1 discovered, calling
   `Namespaces(filepath.Join(wsRoot, m.Root))`.

Pass 2 is what fills the bench skeleton; without it `Namespaces` would have no
caller in the whole slice.

Return contract: **scan results only** — for each member touched, the
`id`/`root` identifying it plus discovered `namespaces`. Never echo authored
`deps`; never invent fields.

- A member root that does not exist is **not** an error — it contributes no
  namespaces (spec Assumption 16).
- **Empty-result rule:** the "found nothing" error fires **only** when there
  are no authored members *and* zero discovered, and it names the five
  declaration sources. It must **not** fire merely because pass 1 found zero.

### `func Merge(existing, scanned []config.Member) []config.Member`

- Match by `id` first, then by cleaned `root`.
- A **non-empty authored field is an override** and wins; an **empty
  `namespaces` is a gap the scan fills**. This asymmetry is the mechanism the
  bench bootstrap rides on.
- `root` and `deps` are never overwritten by the scan.
- **Order preservation lives here, not in `SaveWorkspace`**: existing members
  in manifest order, then newly discovered members sorted by id.

Tests: the authored-roots pass asserted by a workspace root that is an **empty
directory** with authored members at `../` roots (mirroring the bench shape);
a nonexistent member root contributing nothing; the empty-result error firing
only in the no-authored-and-zero-discovered case; and each `Merge` rule
including the order rule.

**Done when:** `go test ./internal/workspace/...` green.

---

## Task 7 — `init-workspace` CLI verb

**Files:** `cmd/codeindex/main.go` (edit), plus a CLI-level test if the
package's existing test style supports one.

- New `case "init-workspace"` in the `switch cmd` at `main.go:43`.
- Add `init-workspace` to the single-line usage verb list at `main.go:40`.
- `<verb> <root>` already satisfies the `len(os.Args) < 3` guard at
  `main.go:38-42` — leave that guard alone.
- `--scan` is **required**; without it, usage error via `fatal()` → **exit 1**.
- Existing manifest without `--force` → refuse, exit 1, message pointing at
  `--force`.
- Otherwise: `Scan` → `Merge` → `SaveWorkspace`.
- **Exit 2 remains exclusive** to the `main.go:38-42` arg-count guard.

**Done when:** the verb runs end-to-end against a temp workspace and every
usage error exits 1.

---

## Task 8 — Root-kind detection groundwork

**Files:** `internal/engine/rootkind.go` (new),
`internal/engine/rootkind_test.go` (new).

Per spec §5:

```go
type RootKind int
const (
    RootRepo RootKind = iota
    RootWorkspace
)
func DetectRootKind(root string) (RootKind, error)
func hasIndexableSource(root string, filter *config.Filter, indexable func(rel string) bool) (bool, error)
```

- `DetectRootKind` **stats** `<root>/.codeindex/workspace.json`; present ⇒
  `RootWorkspace` with **no parse** (a malformed manifest is `LoadWorkspace`'s
  error to report; misclassifying it as a repo is worse).
- Otherwise load the real filter and pass it, with the real predicate,
  **explicitly**:

  ```go
  filter, err := config.LoadFilter(root)
  if err != nil { return RootRepo, err }
  ok, err := hasIndexableSource(root, filter, adapter.Indexable)
  ```

  Never a hard-coded extension list. **Do not call `adapter.SetAssociations`**
  — it replaces process-global routes and would corrupt a live MCP server's
  serving repo.
- `hasIndexableSource` uses `filepath.WalkDir` with `filter.SkipDir` /
  `filter.SkipFile` and returns a **sentinel error on the first admitted
  file** — O(files-until-first-hit). `merkle.WalkWith` is deliberately not
  reused: its signature returns the complete path slice with no early-exit
  hook.
- The "neither" error names **both** possibilities, e.g.
  `%s: not a code repository (no indexable source) and not a workspace (no .codeindex/workspace.json)`.

**`DetectRootKind` has no non-test call site in this slice** — §4.2 owns CLI
wiring behind the byte-identical golden gate. Wiring it here would put a
behavior change in front of that gate.

Tests: a workspace root; a repo root; a "neither" root asserting both phrases
in the message; and a short-circuit assertion that the helper stops at the
first hit (e.g. via a counting `indexable` stub).

**Done when:** `go test ./internal/engine/...` green.

---

## Task 9 — Bench manifest bootstrap

**Files:** `.gitignore` (edit),
`bench/repos/oss-ws/.codeindex/workspace.json` (new).

Per spec §4 and the reconcile-time §4a:

1. **`.gitignore` narrow negation** — immediately below the existing
   `bench/repos/` rule:

   ```
   bench/repos/
   # …except the authored workspace manifest (not a clone; cannot be re-cloned)
   !bench/repos/oss-ws/
   !bench/repos/oss-ws/.codeindex/
   !bench/repos/oss-ws/.codeindex/workspace.json
   ```

   Then **verify nothing broader leaked**: `git status --porcelain bench/repos/`
   must show no member checkout as untracked-but-unignored.
2. **Hand-author the skeleton** — `version: 1` and all 10 members with their
   `id` and `root` from spec §4a.2's table, and **`namespaces: []`**
   (deliberately empty).
3. **Run** `codeindex init-workspace bench/repos/oss-ws --scan --force` from
   the repo root and commit the filled manifest.
4. **Verify by prefix-containment**, per member, against the pinned namespace
   in §4a.2's table: each pin must be a **prefix of at least one discovered
   namespace**. Element-containment is provably wrong here — neither
   `symfony/composer.json` nor `drupal/composer.json` declares a bare
   `Symfony\` / `Drupal\` psr-4 key, so 2 of 10 would fail a correct
   implementation.

`corpus.json` is **not** on this branch (it lives in unpushed local commits on
`main`), so the pins come from the spec table and are quoted into the results
file. If any of the 10 checkouts is absent, **report which and stop** rather
than writing a partially-filled manifest.

This is a **results-recorded** step, not a `go test` case (spec Assumption 10)
— the checkouts are not in the repo and such a test would be red on a clean
clone.

**Done when:** the filled manifest is committed and all 10 members satisfy
prefix-containment.

---

## Task 10 — Openspec amendments + task check-off

**Files:** `openspec/changes/workspace-graph/design.md` (edit),
`openspec/changes/workspace-graph/tasks.md` (edit).

Append a new `## Amendments` section to `design.md` with **two dated
entries**, both 2026-08-18:

1. **D1 monorepo declaration sources** — add `lerna.json` and `package.json`
   `workspaces` to D1's `--scan` source list (design.md:65-66 currently names
   only go.work, pnpm-workspace.yaml, composer path repos). Reason: the
   corpus's only monorepo declares its members solely via `lerna.json`.
2. **D7 merge-gate interpretation** — the frozen SHALL at
   `openspec/changes/workspace-graph/specs/workspace-graph/spec.md:125`
   ("Implementation SHALL NOT merge before the pre-registered gate passes") is
   read as gating **query-behavior** slices. This slice changes no query
   answer — a manifest loader, a new verb, an uncalled detection helper — so
   it may merge ahead of the gate. **The gate still hard-blocks §3.3+ and §4
   from merging.**

Check off §3.1 in `tasks.md`.

**Done when:** both amendments are present and dated, and §3.1 is checked.

---

## Task 11 — Full regression gate

Run the **whole single-repo golden suite**:

- `go test ./...` green.
- `TestCallersTextGolden`, `TestCalleesTextGolden`, `TestNavTextGolden`
  byte-identical to pre-change output.
- `go build ./...` clean; `go mod tidy` leaves no diff.

Then walk the spec's **Acceptance checklist** top to bottom and confirm every
box, including the `.gitignore` narrowness item.

**Done when:** the suite is green and every checklist box is satisfied.
