<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0009 — Workspace manifest load/validate + init-workspace --scan](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0009-workspace-manifest-init-scan.md)**
<!-- docket:backlink:end -->

# Design: Workspace manifest load/validate + `init-workspace --scan`

Change: 0009 `workspace-manifest-init-scan` · openspec task §3.1 of
`openspec/changes/workspace-graph/tasks.md` · frozen design
`openspec/changes/workspace-graph/design.md` (D1 identity, D5 surfaces).

## Scope

The first engine slice of the workspace-graph campaign. Three deliverables
plus two documentation amendments:

1. **Manifest types + load/validate/save** in `internal/config` —
   `<workspace-root>/.codeindex/workspace.json` per design D1.
2. **`init-workspace --scan [--force]`** — a new CLI verb: per-language
   namespace auto-discovery and monorepo member discovery, merged into the
   manifest with authored values winning.
3. **Root-kind detection groundwork** in `internal/engine` — a
   short-circuiting helper plus `DetectRootKind`, with **no call site in
   this slice** (§4.2 owns CLI wiring behind the byte-identical golden gate).
4. **Bench manifest bootstrap (in-slice)** — a hand-authored skeleton
   manifest for `bench/repos/oss-ws` filled by `--scan --force` and verified
   against `bench/workspace/corpus.json`.
5. **Two dated amendments** to the frozen `openspec/changes/workspace-graph/design.md`.

Nothing else in the campaign moves: no overlay store (§3.2), no resolution
ladder (§3.3), no freshen/`workspace-status` (§3.4), no union-graph query
paths or MCP/CLI wiring (§4.x), no gate run (§5.x).

## Codebase facts this design is built on

Verified against the working tree at `origin/main` (2026-08-18):

- `internal/config/config.go:52` — `func Load(root string) (Config, error)`;
  missing file ⇒ zero `Config`, malformed ⇒ error. `config.FileName` is
  `.codeindex.json` (a **file** at the root); the workspace manifest is a
  different path (`.codeindex/workspace.json`, a **directory**) and does not
  collide.
- `internal/config/filter.go:62` — `func LoadFilter(root string) (*Filter, error)`.
  `config.DefaultExcludeDirs` already contains `.codeindex`, `node_modules`,
  `vendor`, `dist`, `build`, `testdata`, … so the manifest directory and
  vendored trees are pruned from any filtered walk for free.
- `internal/merkle/merkle.go:29` — `func WalkWith(root string, extra func(rel string, d fs.DirEntry) bool) ([]string, error)`.
  It calls `config.LoadFilter(root)` internally and walks the **entire** tree
  collecting every path. There is no early-exit form — which is precisely why
  root-kind detection gets its own helper (owner decision, round 1 #2).
- `internal/adapter/adapter.go:116` — `func Indexable(path string) bool` is a
  package-level function reading **process-global** state (`assocRules`,
  `exactRoutes`), installed by `SetAssociations` (`adapter.go:55`) /
  `SetExactRoutes`. Mutating that global per detection against a live MCP
  server is the hazard the owner ruled out.
- `cmd/codeindex/main.go:38-42` — arg-count guard, `os.Exit(2)`, with the
  single-line usage string listing every verb at `main.go:40`; `main.go:43` —
  `cmd, root := os.Args[1], os.Args[2]` then a `switch cmd`; `main.go:640` —
  `func fatal(err error)` → stderr + `os.Exit(1)`.
- `internal/runtime/runtime.go:70` — the version-check precedent
  (`"unsupported cxprof version %d"`). Its version-`0` case at
  `runtime.go:66-67` is fused with a JSON-unmarshal failure under one
  "not a cxprof header" message; §1 deliberately does **not** copy that
  fusion.
- `internal/depmap/depmap.go:119,142` — `DiscoverGoVendor` / `DiscoverComposer`:
  the house style for manifest-file discovery (read file, tolerate absence via
  the caller, `os.Stat` the candidate directory, return a typed slice).
- `go.mod` — module `codeindex`, Go 1.26.5. Neither `golang.org/x/mod` nor any
  YAML package is currently required (direct or indirect).
- `bench/repos/oss-ws/` exists and is **empty**.
  `bench/workspace/corpus.json` has `{workspace_root, members[]}` with 10
  members; each member carries `{id, root, lang, role, pin, namespaces}`.
  Roots are relative and all point outside the workspace root (`../symfony`,
  `../nest/packages/common`, …) — the D1 dedicated-workspace-directory shape.
- Golden tests: `internal/query/query_test.go:43,113,140` —
  `TestCallersTextGolden`, `TestCalleesTextGolden`, `TestNavTextGolden`.
  There is no unified "golden suite" make target; `internal/engine/*_test.go`
  additionally carries the incremental-equals-full proofs
  (`TestIncrementalEqualsFull_*`). The regression bar is therefore stated as
  `go test ./...` plus byte-identical output from the three named goldens.
- **`internal/graph/types.go:33` — `func DeriveNamespace(path, declared string) string`
  is a different concept and must not be reused here.** It computes a
  *per-file, per-symbol enclosing scope*: `types.go:34-36` returns `declared`
  verbatim when non-empty (for a Symfony file that is the file's own declared
  namespace, e.g. `Symfony\Component\HttpFoundation`), and otherwise falls
  back to a path-derived scope (Go/PHP → directory, Python → dotted module
  path, TS/JS → the file itself). A workspace member `namespaces` entry is a
  *whole-member published namespace prefix* read from a build manifest
  (`module` path, package `name`, psr-4 key). The granularity is wrong in
  both directions — a single file's declared namespace is neither the
  member's published set nor a member-identifying prefix — so
  `DeriveNamespace` is not a substitute for §2a and is not called by it.
- `internal/engine/engine.go:162` — `loadAssociations(root)` calls
  `adapter.SetAssociations` before every walk; that existing call site is the
  reason a *detection* helper must not add another one (see §5).

## Design

### 1. Manifest types and I/O (`internal/config`)

New file `internal/config/workspace.go`.

```go
// WorkspaceFile is the manifest path relative to the workspace root.
const WorkspaceFile = ".codeindex/workspace.json"

type Workspace struct {
    Version int      `json:"version"`
    Members []Member `json:"members"`
}

type Member struct {
    ID         string   `json:"id"`
    Root       string   `json:"root"`
    Namespaces []string `json:"namespaces"`
    Deps       []string `json:"deps,omitempty"`
}

func LoadWorkspace(wsRoot string) (*Workspace, error)
func SaveWorkspace(wsRoot string, ws *Workspace) error
func (w *Workspace) Resolve(wsRoot string) (present []ResolvedMember, missing []string, err error)
```

**`LoadWorkspace` validates shape only and never stats a member root.** This
is load-bearing, not a stylistic preference: the frozen scenario at
`openspec/changes/workspace-graph/specs/workspace-graph/spec.md:117-122`
requires a workspace `callers` answer to be *produced* while one member is
unavailable, with the coverage clause naming the missing member. If load
failed on a missing root, that scenario would be unreachable. `Resolve` is
the separate, later-callable pass that stats each `root` and reports which
members are absent; **it has no call site in this slice** (§3.2/§3.4 own the
consumers) and exists so the split is fixed before the overlay store is
written against it.

Validation rules, all errors:

| Rule | Rationale |
|---|---|
| `version` absent (`0`) or `!= 1` | `internal/runtime/runtime.go:70` is the precedent for the `!= 1` case (`"unsupported cxprof version %d"`); mirror it as `"unsupported workspace version %d"`. Note the version-`0` case there (`runtime.go:66-67`) is *fused* with a JSON-unmarshal failure under one "not a cxprof header" message — that fusion is **not** copied: a manifest that parses but omits `version` gets its own message, since unlike a JSONL header the field's absence is diagnosable on its own |
| `members` empty | a manifest that declares nothing is an authoring mistake, not a valid degenerate workspace |
| `id` empty, or containing a character outside `[A-Za-z0-9._-]` | D4 gives ids an anchor-prefix role (`api:HandleLogin`); a `:` or `/` in an id makes that grammar ambiguous |
| duplicate `id` | ids are the cross-edge key (D2) |
| `root` empty, or `filepath.IsAbs(root)` | roots are relative to the workspace root per D1; an absolute root makes the manifest non-portable and non-committable |
| duplicate `root` (after `filepath.Clean`) | two ids for one tree would double-count in the union graph |

`../` in `root` is **accepted** — the D1 sanctioned dedicated-workspace-directory
shape depends on it, and every member in the frozen bench corpus uses it.

Unknown JSON fields are **ignored** (plain `encoding/json`, no
`DisallowUnknownFields`) — forward compatibility with §3.2+ manifest growth,
matching `Config`'s existing posture.

Absence is distinguishable: a missing file returns an error wrapping the
underlying `os` error so `errors.Is(err, fs.ErrNotExist)` holds. Callers that
want "no manifest ⇒ single-repo mode" test that, never a string match.

`SaveWorkspace` writes the slice it is given, in the order it is given, with
`json.MarshalIndent` two-space indent and a trailing newline; it creates
`.codeindex/` if absent. **It contains no ordering policy** — see §3b.

### 2. Discovery (`internal/workspace`, new package)

New package `internal/workspace` holding the scan. It is a new package rather
than more surface on `internal/config` because it reads foreign build files
and pulls in two new module dependencies; `internal/config` stays a thin,
dependency-light loader.

#### 2a. Namespace discovery per language

`func Namespaces(memberRoot string) ([]string, error)` — runs every probe and
concatenates, deduplicated, sorted:

- **Go** — `go.mod` at the member root; the `module` path via
  `golang.org/x/mod/modfile.Parse`. (Owner decision round 2 #3: use the
  library, not a hand-rolled line scan — `module` may be quoted or sit in a
  block.)
- **TS/JS** — `package.json` `name`.
- **PHP** — composer `name` **plus** every `autoload.psr-4` key. The psr-4
  value must decode as **string-or-array**: laravel-framework maps
  `Illuminate\Support\` to three paths, so a `map[string]string` decode fails
  outright. Decode into `map[string]json.RawMessage` and branch on the first
  byte, or into `map[string]any` and type-switch.
- **Python** — top-level importable modules, probing `src/` **first** and
  falling back to the member root. Both Python members in the frozen corpus
  (werkzeug, flask) are src-layout; a root-only rule silently zeroes the 12
  Python tasks of the 65. A candidate is a directory containing
  `__init__.py`, or a top-level `*.py` file (module name = basename without
  extension); `test`/`tests`/`docs`/`examples` and dot-prefixed entries are
  skipped.

A language whose marker file is absent contributes nothing and is not an
error — members are single-language in practice but the probes are
independent.

#### 2b. Monorepo member discovery

`func Members(root string) ([]config.Member, error)` reads member
declarations from **five** sources:

| Source | Extraction |
|---|---|
| `go.work` | `use` directives, via `golang.org/x/mod/modfile.ParseWork` |
| `pnpm-workspace.yaml` | `packages:` list, via `gopkg.in/yaml.v3` |
| `composer.json` | `repositories[]` entries with `"type": "path"` → their `url` |
| `lerna.json` | `packages:` list |
| `package.json` | `workspaces` — array form, **and** the object form `{"packages": [...]}` (yarn) |

`lerna.json` and `package.json` `workspaces` are the **round-2 owner amendment**
to design D1, which originally listed only the first three. The motivating
observation: the corpus's only monorepo, `bench/repos/nest`, declares its
members **solely** via `lerna.json` (`packages: ["packages/*"]`); its root
`package.json` has `name: "@nestjs/core"` and **no `workspaces` key**, so
under the unamended three-source list `Members()` on a bare nest root finds
zero.

**Scope note — this path gets no bench coverage in this slice, and the spec
does not claim otherwise.** The bench bootstrap (§4) scans
`bench/repos/oss-ws`, whose three nest members are hand-authored with
`../nest/packages/*` roots; nothing in the specified flow ever runs
`Members()` against `bench/repos/nest`. The five-source discovery path and
all three over-collection guards are therefore covered **only** by the
fixture unit tests on the acceptance checklist. Organic coverage for the
other four declaration formats is change 0010's subject, deliberately not
this slice's.

Each source yields glob-ish patterns (`packages/*`). Expansion via
`filepath.Glob` relative to `root`, then the **three over-collection guards**
(owner decision round 1 #1), applied in order:

1. **Directories only** — a glob hit that is not a directory is dropped. Also
   dropped: any candidate whose path traverses a `config.DefaultExcludeDirs`
   basename (`node_modules`, `vendor`, `dist`, …). This is the same intent as
   guard 1 — a glob is not the repo filter, so the exclusion must be applied
   explicitly — and it is why `packages/*` under a populated `node_modules`
   does not explode.
2. **Marker at the candidate's own top level** — the candidate must itself
   contain `go.mod` / `package.json` / `composer.json` / a Python marker. A
   marker one level down does not qualify it.
3. **Subset suppression** — a candidate whose discovered namespace set is a
   subset of an already-declared member's namespace set is suppressed. This
   is what drops a monorepo's own root package when it re-exports its
   children.

Guards 1–3 together were verified by the owner to fix all three spurious
members observed on the frozen bench corpus.

Discovered member **id** is the candidate directory's basename, sanitized to
`[A-Za-z0-9._-]` (every other rune → `-`) and lowercased; on collision, a
`-2`, `-3`, … suffix in expansion order. Ids are derived only for members the
scan *introduces*; an authored member keeps its authored id untouched, so the
bench corpus's hand-authored `nest-common` / `nest-core` /
`nest-microservices` ids are never regenerated from basenames.

#### 2c. New module dependencies

`golang.org/x/mod` (go.mod / go.work parsing) and `gopkg.in/yaml.v3`
(pnpm-workspace.yaml) — owner decision round 2 #3. Both are pure Go with no
cgo and no runtime assets, so **ADR-0003 (single static binary) is
unaffected** and no new ADR is warranted. Added as direct requires; `go mod
tidy` run and `go.sum` committed.

### 3. `init-workspace --scan [--force]` (CLI)

```
codeindex init-workspace <workspace-root> --scan [--force]
```

Wired as a new `case "init-workspace"` in `cmd/codeindex/main.go`'s switch,
and added to the usage verb list on the single-line usage string at
`main.go:40` (inside the `len(os.Args) < 3` guard at `main.go:38-42`). The
verb reaches `cmd, root := os.Args[1], os.Args[2]` at `main.go:43` unchanged
(`<verb> <root>` satisfies the ≥3 argv guard).

Behavior:

- `--scan` is **required**. Without it: usage error via `fatal()` → exit 1.
  (Exit 2 belongs to `main.go:38-42`'s arg-count guard alone and stays there.)
- Existing manifest without `--force` → refuse, exit 1, message pointing at
  `--force`.
- Otherwise run **the scan** (§3a), then `Merge` (§3b), then `SaveWorkspace`.

#### 3a. What the scan actually does — two independent passes

This is the load-bearing detail, and it is why `--scan` on the D1
dedicated-workspace-directory shape does useful work at all. The scan is
**two passes, not one**:

```go
func Scan(wsRoot string, authored []config.Member) ([]config.Member, error)
```

1. **Member-discovery pass** — `Members(wsRoot)` (§2b) reads the five
   monorepo declaration sources **at the workspace root** and returns any
   members declared there. On a dedicated workspace directory (an empty dir
   whose members all live at `../`) this legitimately returns **zero**.
2. **Namespace pass — runs over every authored member root, and over every
   member the first pass discovered.** For each, `Namespaces(filepath.Join(wsRoot, m.Root))`
   (§2a) is called and the result attached to that member.

Pass 2 is what fills the bench skeleton: the 10 members are authored with
`namespaces: []` and roots at `../symfony`, `../nest/packages/common`, …, and
each of those roots is where `go.mod` / `package.json` / `composer.json` /
the Python layout actually lives. Without pass 2, `Namespaces` would have no
caller anywhere in the slice and `--force` would return the skeleton
unchanged.

**`Scan`'s return contract.** `authored` is passed in solely to enumerate the
roots pass 2 must probe. `Scan` returns **scan results only** — for each
member it touched, the `id`/`root` identifying it plus the `namespaces` it
discovered, and nothing else. It never echoes authored `deps`, and never
invents fields. `Merge` is the single place authored values and scanned
values meet, and its authored-wins rule is what resolves them.

A member root that does not exist on disk is **not** an error in pass 2 — it
contributes no namespaces and the member keeps whatever it was authored with.
Erroring there would re-introduce the member-root stat that §1 deliberately
keeps out of the load path.

**Empty-result rule:** the "found nothing" error fires only when the run
would write a manifest with **no members at all** — i.e. no authored members
*and* zero discovered. It names the five declaration sources it looked for.
It must **not** fire merely because pass 1 discovered zero members, because
that is the normal, D1-sanctioned outcome for a dedicated workspace directory
and is exactly the bench bootstrap's own case.

#### 3b. The `--force` merge — and where the ordering rule lives

The merge is a function in `internal/workspace`:

```go
func Merge(existing []config.Member, scanned []config.Member) []config.Member
```

Rules:

- Match by `id` first, then by cleaned `root`.
- For a matched member: a **non-empty authored field is an override** and
  wins. An **empty `namespaces` is a gap the scan fills.** This asymmetry is
  what makes the D1 dedicated-workspace-directory bootstrap work at all —
  it is the mechanism the bench bootstrap (§4) rides on, not an edge case.
- `root` and `deps` are never overwritten by the scan.
- **Order preservation lives here, in `Merge` — not in `SaveWorkspace`.**
  `Merge` returns the existing members in their manifest order, followed by
  newly discovered members sorted by id. `SaveWorkspace` receives only the
  final slice and has no way to distinguish authored from appended, so the
  rule is unimplementable there. D4 answers "members ordered by manifest," so
  a scan that reshuffles a reviewed manifest would silently change future
  answer ordering.

### 4. Bench manifest bootstrap (in-slice)

Owner decision round 2 #2. Two steps, both landing in this change:

1. **Hand-author** `bench/repos/oss-ws/.codeindex/workspace.json`: `version: 1`
   and all 10 members from `bench/workspace/corpus.json` with their `id` and
   `root` **and `namespaces: []`** (empty, deliberately).
2. **Run** `codeindex init-workspace bench/repos/oss-ws --scan --force` and
   commit the filled manifest.

Then **verify**, per member, against the member's **`namespaces` field in
`corpus.json`** — *not* its `pin` field, which is a separate literal in the
same object holding the checkout's git tag (`"pin": "v7.2.2"`) and is
irrelevant here. Each entry of that `namespaces` field must be a **prefix of
at least one discovered namespace** (prefix-containment), not an element of
the discovered set. This is not a softening — element-containment is provably
wrong on the actual corpus data:

| member | corpus pin | what §2a discovers | element? | prefix? |
|---|---|---|---|---|
| symfony | `Symfony\` | name `symfony/symfony`; psr-4 keys `Symfony\Bridge\Doctrine\`, `Symfony\Bridge\Monolog\`, `Symfony\Bridge\PsrHttpMessage\`, `Symfony\Bridge\Twig\`, `Symfony\Bundle\`, `Symfony\Component\`, `Symfony\Runtime\Symfony\Component\` | **no** | yes |
| drupal | `Drupal\` | name `drupal/core`; psr-4 keys `Drupal\Core\`, `Drupal\Component\` | **no** | yes |
| laravel | `Illuminate\` | psr-4 keys incl. `Illuminate\` | yes | yes |
| nest-common / -core / -microservices | `@nestjs/common` etc. | package.json `name` | yes | yes |
| werkzeug / flask | `werkzeug`, `flask` | `src/`-layout top-level module | yes | yes |
| client_golang / prometheus | `github.com/prometheus/…` | go.mod `module` | yes | yes |

Neither `symfony/composer.json` nor `drupal/composer.json` declares a bare
`Symfony\` / `Drupal\` psr-4 key, so an element-containment check fails on 2
of 10 members even when the implementation is completely correct.
Prefix-containment is the honest predicate: the pin names the namespace
*family* the corpus's rung-1 tasks traverse, and D3 rung 1 matches "on
namespace boundaries" (prefix), not by set membership.

This is not decoration: it is the only end-to-end exercise of the
empty-`namespaces`-is-a-gap merge path, and it hands bench arm B a real
manifest to point at when §3.3+ lands.

**Corpus checkouts are a precondition, not a deliverable.** The 10 member
trees under `bench/repos/` are the existing pinned OSS checkouts; if any is
absent at build time, the bootstrap step reports which and stops rather than
writing a partially-filled manifest. The verification is recorded in the
change's results, not as a `go test` case — the checkouts are not part of the
repo and a test depending on them would be red on a clean clone.

**Growing the corpus is explicitly out of scope** — that is change 0010.

### 5. Root-kind detection groundwork (`internal/engine`)

New file `internal/engine/rootkind.go`.

```go
type RootKind int
const (
    RootRepo RootKind = iota
    RootWorkspace
)

// DetectRootKind classifies root. A root containing the workspace manifest is
// a workspace; a root with neither a manifest nor any indexable source is an
// error naming both possibilities.
func DetectRootKind(root string) (RootKind, error)

// hasIndexableSource reports whether root contains at least one file the
// given filter admits and indexable claims, aborting the walk on the first
// hit.
func hasIndexableSource(root string, filter *config.Filter, indexable func(rel string) bool) (bool, error)
```

- `DetectRootKind` stats `<root>/.codeindex/workspace.json`; present ⇒
  `RootWorkspace` (no parse — a malformed manifest is `LoadWorkspace`'s error
  to report, and misclassifying it as a repo would be worse).
- Otherwise it loads the real filter and passes it, with the real predicate,
  explicitly. `config.LoadFilter` returns `(*Filter, error)`, so the two-value
  call cannot be inlined as an argument:

  ```go
  filter, err := config.LoadFilter(root)
  if err != nil {
      return RootRepo, err
  }
  ok, err := hasIndexableSource(root, filter, adapter.Indexable)
  ```

  **The filter and the predicate are parameters**, passed explicitly at the
  one call site — never a hard-coded extension list, which would diverge from
  the registry the moment a language or an `associations` rule is added.
  `adapter.Indexable` is only ever *read*; `adapter.SetAssociations` is
  **not** called, because it replaces process-global routes and a detection
  running inside a live MCP server would corrupt the serving repo's routing.
  (The consequence — a repo whose only source is routed by an
  `associations`-only pattern with no registered extension may classify as
  "neither" — is accepted here: §4.2 owns the CLI wiring and can install
  associations once at startup before detecting.)
- `hasIndexableSource` uses `filepath.WalkDir` with `filter.SkipDir` /
  `filter.SkipFile` and returns a sentinel error on the first admitted file,
  so it is O(files-until-first-hit), not a full-tree walk. `merkle.WalkWith`
  is deliberately **not** reused: its signature returns the complete path
  slice with no early-exit hook, so reusing it would mean walking the whole
  tree on every detection.
- The "neither" error names both possibilities, e.g.
  `%s: not a code repository (no indexable source) and not a workspace (no .codeindex/workspace.json)`.

**`DetectRootKind` has no call site in this slice.** Task §4.2 owns CLI
root-kind branching behind the byte-identical single-repo golden gate; wiring
it here would put a behavior change in front of that gate.

### 6. Amendments to the frozen design

Both land in this change's PR, as dated entries in a new `## Amendments`
section appended to `openspec/changes/workspace-graph/design.md` (the same
mechanism for both, per the round-2 owner decision):

1. **2026-08-18 — D1 monorepo declaration sources.** Add `lerna.json` and
   `package.json` `workspaces` to D1's `--scan` source list (design.md:65-66
   currently names only go.work, pnpm-workspace.yaml, composer path repos).
   Reason: the corpus's only monorepo declares members solely via
   `lerna.json`.
2. **2026-08-18 — D7 merge-gate interpretation.** The frozen SHALL at
   `openspec/changes/workspace-graph/specs/workspace-graph/spec.md:125`
   ("Implementation SHALL NOT merge before the pre-registered gate passes")
   is read as gating **query-behavior** slices. This slice changes no query
   answer — it adds a manifest loader, a new verb, and an uncalled detection
   helper — so it may merge ahead of the gate. The gate still hard-blocks
   §3.3+ and §4 from merging.

`openspec/changes/workspace-graph/tasks.md` §3.1 is checked off in the same
PR.

## Acceptance checklist

- [ ] `internal/config/workspace.go`: `Workspace`/`Member`, `LoadWorkspace`,
      `SaveWorkspace`, `Resolve`; every validation rule in §1's table has a
      unit test, including `version: 0`, `version: 2`, a bad id rune, an
      absolute root, a duplicate id, a duplicate root, and an accepted `../` root.
- [ ] `LoadWorkspace` never stats a member root — a test loads a manifest
      whose members all point at nonexistent paths and asserts success;
      `Resolve` on the same manifest reports them missing.
- [ ] Missing manifest satisfies `errors.Is(err, fs.ErrNotExist)`.
- [ ] `internal/workspace`: `Namespaces` tested per language against fixture
      trees — go.mod module path; package.json name; composer name **plus**
      psr-4 keys with a **multi-path array** value; Python **src-layout**
      resolved before root layout.
- [ ] `internal/workspace`: `Members` tested against all five declaration
      sources, plus one fixture per over-collection guard (a file matching the
      glob; a `node_modules` hit; a marker one level below the candidate; a
      subset-namespace root package).
- [ ] **`Scan` runs its namespace pass over authored member roots**, not only
      over members discovered at the workspace root — asserted by a test whose
      workspace root is an empty directory with authored members at `../`
      roots, mirroring the bench shape. A nonexistent member root contributes
      no namespaces and is not an error.
- [ ] The "found nothing" error fires **only** when there are no authored
      members *and* zero discovered — a test asserts `--scan --force` on a
      dedicated workspace directory with authored members succeeds.
- [ ] `Merge`: authored non-empty field overrides the scan; **empty
      `namespaces` is filled**; existing manifest order preserved with new
      members appended sorted by id. The order rule is asserted against
      `Merge`, and `SaveWorkspace` carries no ordering policy.
- [ ] `init-workspace` wired into `main.go`'s switch and into the usage verb
      list at `main.go:40`; `--scan` required; existing manifest refused
      without `--force`; every usage error exits **1** via `fatal()`; exit 2
      remains exclusive to the `main.go:38-42` arg-count guard.
- [ ] `internal/engine/rootkind.go`: `DetectRootKind` + `hasIndexableSource`;
      the helper takes `*config.Filter` and the indexable predicate as
      **explicit parameters** and short-circuits on the first hit; no call to
      `adapter.SetAssociations`; the "neither" error names both possibilities.
      `DetectRootKind` has no non-test call site.
- [ ] `go.mod` requires `golang.org/x/mod` and `gopkg.in/yaml.v3`; `go mod
      tidy` clean; `go.sum` committed; the binary still builds with `CGO`
      unchanged.
- [ ] `bench/repos/oss-ws/.codeindex/workspace.json` committed, filled by
      `init-workspace --scan --force`, and verified for all 10 members by
      **prefix-containment** — each entry of that member's **`namespaces`
      field** in `corpus.json` (not its `pin` field, which holds a git tag)
      is a prefix of at least one discovered namespace. Element-containment
      is explicitly wrong here
      (`Symfony\` and `Drupal\` are not psr-4 keys in their composer files);
      see §4's table.
- [ ] **Dated amendment section added to
      `openspec/changes/workspace-graph/design.md`** carrying both the D1
      source-list amendment and the D7 merge-gate interpretation; `tasks.md`
      §3.1 checked off.
- [ ] **Regression bar: the whole single-repo golden suite.** `go test ./...`
      green, and `TestCallersTextGolden`, `TestCalleesTextGolden`,
      `TestNavTextGolden` (`internal/query/query_test.go:43,113,140`)
      byte-identical to pre-change output. No existing file's behavior
      changes: every deliverable is additive except the `main.go` switch case
      and usage string.

## Out of scope

Overlay store / `workspace.db` (§3.2) · cross-repo resolution ladder (§3.3) ·
workspace freshen + `workspace-status` (§3.4) · union-graph query paths, fan-out,
`repo` field, `<member-id>:` anchor prefix, MCP surface (§4.x) · the evidence
gate run (§5.x) · **growing the bench corpus with more monorepo declaration
examples — that is change 0010** · any new ADR.

## Assumptions

Every decision below was defaulted by the autonomous groomer, with the owner
decisions from the change file's `## Groom context` treated as binding input
and never re-litigated.

| # | Decision | Chosen | Rejected | Why |
|---|---|---|---|---|
| 1 | Where the scan lives | new package `internal/workspace` | more surface on `internal/config`; inside `internal/engine` | the scan pulls in two new module deps and parses foreign build files; `internal/config` is a thin dependency-light loader consumed by nearly everything, and `internal/engine` is the build pipeline |
| 2 | `hasIndexableSource` signature | takes `*config.Filter` **and** an `indexable func(string) bool` as parameters | taking only the root and calling `config.LoadFilter`/`adapter.Indexable` internally | the owner's binding fix requires the real filter passed **explicitly**; `adapter.Indexable` is a package-level function over process globals, so "explicit" can only mean a function-valued parameter. Also makes the helper unit-testable without touching adapter globals |
| 3 | Manifest-present ⇒ workspace without parsing | stat only | parse and fall back to `RootRepo` on a malformed manifest | a malformed manifest silently downgrading to single-repo mode is exactly the silent-wrong-answer failure the project's trust story rejects; the parse error belongs to `LoadWorkspace` |
| 4 | Scanned member id derivation | sanitized, lowercased directory basename; `-2`/`-3` on collision | derive from the discovered namespace; require the human to name every member | basenames are stable and reviewable. The bench corpus's ids (`nest-common`) are hand-authored and `Merge` never regenerates an authored id, so this rule is not on the bench path at all |
| 5 | When the "found nothing" error fires | only when there are **no authored members and zero discovered** | fire whenever member-*discovery* returns zero | firing on zero discovery would abort `--scan` on the D1-sanctioned dedicated-workspace-directory shape — where discovery legitimately finds nothing because every member lives at `../` — and would abort §4's own bench bootstrap. The condition that actually matters is "would this write a manifest `LoadWorkspace` then rejects" |
| 6 | Unknown JSON fields | ignored | `DisallowUnknownFields` | matches `Config`'s existing posture and keeps §3.2+ manifest growth backward-compatible |
| 7 | Duplicate `root` rejected (owner rules covered duplicate `id`, not `root`) | reject | allow | two ids over one tree double-count every symbol in the union graph and would make D4's member ordering nondeterministic in effect |
| 8 | Guard 1 extended to `DefaultExcludeDirs` traversal | drop candidates under an excluded basename | rely on guard 1's directory-only test alone | `filepath.Glob` is not the repo filter; `packages/*` inside a populated `node_modules` is exactly the over-collection the guards exist to stop. Same intent as the owner's guard 1, made explicit |
| 9 | Bench verification predicate | **prefix-containment** — each pin is a prefix of some discovered namespace | exact set equality; element-containment (pin ∈ discovered set) | equality fails because a `psr-4` block declares several namespaces. Element-containment also fails, on real data: `symfony/composer.json` declares `Symfony\Bridge\…`/`Symfony\Component\…` but never bare `Symfony\`, and `drupal/composer.json` declares `Drupal\Core\`/`Drupal\Component\` but never bare `Drupal\` — so 2 of 10 members would fail a correct implementation. Prefix also matches D3 rung 1's own "prefix match on namespace boundaries" |
| 10 | Bench bootstrap is a results-recorded step, not a `go test` case | results | a test | the 10 corpus checkouts are not in the repo; a test depending on them is red on a clean clone and would poison the "whole golden suite green" regression bar |
| 11 | PHP psr-4 decode | string-or-array, via `RawMessage`/`any` branch | `map[string]string` | laravel-framework's `Illuminate\Support\` maps to three paths; a `map[string]string` decode fails outright — settled in the prior groom rounds, restated because it is a silent-corpus-loss trap |
| 12 | Python probes `src/` before root | src-first with root fallback | root only | both corpus Python members are src-layout; root-only silently zeroes 12 of the 65 tasks — settled in the prior groom rounds |
| 13 | `Resolve` ships with no call site | ship it | defer to §3.2 | it is the half of the load/resolve split that keeps the frozen coverage-clause scenario (spec.md:117-122) reachable; shipping it with `LoadWorkspace` is what fixes the split before consumers are written against a merged version |
| 14 | No new ADR | none | an ADR for the two new deps | both are pure-Go libraries that leave ADR-0003 (single static binary) intact; settled in the prior groom rounds |
| 15 | Shape of the scan (§3a) | **two independent passes** — member discovery at the workspace root, then a namespace pass over *every* member, authored ones included | a single pass that only derives namespaces for members it discovered itself | a discovery-only scan leaves `Namespaces` with no caller in the slice and returns the bench skeleton unchanged, so the empty-`namespaces` merge path — the thing owner decision round 2 #2 exists to exercise — never executes. The authored members are precisely the ones whose roots hold the build manifests |
| 16 | A nonexistent member root during the namespace pass | contributes nothing; not an error | error out | erroring re-introduces the member-root stat that §1 keeps out of the load path to preserve the frozen coverage-clause scenario (spec.md:117-122). `Resolve` is where absence is reported |

**Dependency state:** `depends_on: []` — nothing gates this slice. `related: [10]`
(bench corpus growth) is informational and deliberately not folded in.
