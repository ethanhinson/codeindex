<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0004 — Cleanup — delete .lore/, drop lore config, rewrite README](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0004-cleanup-delete-lore-rewrite-readme.md)**
<!-- docket:backlink:end -->

# Cleanup — delete .lore/, drop lore config, rewrite README — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the last lore product remnants from the codeindex repo — delete `.lore/`, retarget stale deleted-tree example strings the earlier phases left in `internal/config`/`internal/merkle`, purge the plugin's dead lore hooks and copy, fix a stale `serve` doc-comment, and rewrite the README to pure blast-radius positioning with the decoupled versioned symbol-graph API.

**Architecture:** Pure subtraction plus documentation. No behavioral code changes: every Go edit is to a doc-comment or a test fixture string that names a directory (`web/`, `internal/webserver/dist/`) that changes 0002/0003 already deleted. The fixtures exercise generic exclude/glob logic — retarget them to live-shaped paths (`vendor/…`, `pkg/dist`) so the same code paths are covered without naming a dead tree. The build/test suite stays green throughout.

**Tech Stack:** Go 1.24 (CGO — tree-sitter + SQLite), Python (plugin hooks), Markdown/JSON docs.

## Global Constraints

- `go build ./...` and `go test ./...` MUST stay green after every task.
- All work happens in the feature worktree `/Users/ethanhinson/codeindex/.worktrees/cleanup-delete-lore-rewrite-readme` on branch `feat/cleanup-delete-lore-rewrite-readme` (cut from `origin/main`).
- The feature branch carries only code + this plan; it never touches docket metadata (`.docket/`, change files, BOARD.md, ADRs).
- No behavioral change to the impact engine or the graph API (owned by changes 0002/0003).
- Do NOT touch the historical `docs/superpowers/{plans,specs}/*lore*` design docs — they are the archived record of a shipped-then-removed product.
- Reference for API copy: `docs/graph-api.md` (already on the branch) — `schemaVersion` is `"1"`; endpoints `/api/health`, `/api/graph?symbol=<name>&parent=<optional>`, `/api/graph/full`; default serve addr `127.0.0.1:7676`, `--addr` flag.

---

### Task 1: Delete the `.lore/` directory

The keeper decisions were migrated to ADRs 0001–0008 by change 0001 (merged), so deletion is now safe. `.lore/` holds `decisions/`, `items/`, `notes/` (42 tracked files).

**Files:**
- Delete: `.lore/` (entire tree)

- [ ] **Step 1: Confirm `.lore/` is tracked and has no other references in code**

Run:
```bash
cd /Users/ethanhinson/codeindex/.worktrees/cleanup-delete-lore-rewrite-readme
git ls-files .lore/ | wc -l          # expect ~42
git grep -n '"\.lore"' -- 'internal/**/*.go' 'cmd/**/*.go' || echo "no Go code opens .lore literally"
```
Expected: a nonzero file count; no Go source constructs a `.lore` path (the lore engine that did was deleted in Phase 1). If the grep finds a live Go reference, STOP and report — scope changed.

- [ ] **Step 2: Delete the directory**

Run:
```bash
git rm -r .lore/
```

- [ ] **Step 3: Verify build + tests still green**

Run:
```bash
go build ./... && go test ./...
```
Expected: PASS (nothing in the kept code depends on `.lore/`).

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "chore: delete .lore/ (decisions migrated to ADRs 0001-0008)"
```

---

### Task 2: Retarget stale deleted-tree example strings in config + merkle

Changes 0002/0003 deleted `web/` and `internal/webserver/dist/`. Three files still name those dead trees purely as illustrative exclude/glob examples: a doc-comment in `internal/config/config.go`, and test fixtures in `internal/config/filter_test.go` and `internal/merkle/walk_test.go`. Retarget each to a live-shaped path that exercises the identical prefix/glob/default-basename logic. `dist` is a default-excluded basename (`internal/config/filter.go` `DefaultExcludeDirs`), so keep a `dist` case but under a non-deleted parent (`pkg/dist`). `node_modules` is likewise a default basename — keep it under `vendor/react` shape (`external/node_modules`).

**Files:**
- Modify: `internal/config/config.go` (Exclude doc-comment, ~line 26)
- Modify: `internal/config/filter_test.go`
- Modify: `internal/merkle/walk_test.go`
- Test: `internal/config/filter_test.go`, `internal/merkle/walk_test.go` (these ARE the tests)

**Interfaces:**
- Consumes: nothing new. `config.NewFilter`, `Filter.SkipDir`, `Filter.SkipFile`, `merkle.Walk` are unchanged.
- Produces: nothing new — behavior-preserving edits only.

- [ ] **Step 1: Fix the `config.go` Exclude doc-comment**

In `internal/config/config.go`, the `Exclude` field comment currently reads:
```go
	// Exclude lists repo-relative paths/globs to skip when indexing, in
	// addition to the built-in defaults (vendored/compiled dirs). A pattern
	// with no wildcard is a path prefix ("internal/webserver/dist" skips that
	// whole subtree); a pattern with '*'/'**'/'?' is a glob matched against the
	// repo-relative path ('**' spans directory separators).
```
Replace the example `"internal/webserver/dist"` with a live path — use `"docs/generated"`:
```go
	// Exclude lists repo-relative paths/globs to skip when indexing, in
	// addition to the built-in defaults (vendored/compiled dirs). A pattern
	// with no wildcard is a path prefix ("docs/generated" skips that
	// whole subtree); a pattern with '*'/'**'/'?' is a glob matched against the
	// repo-relative path ('**' spans directory separators).
```

- [ ] **Step 2: Fix `internal/config/filter_test.go` fixtures**

Retarget every `internal/webserver/dist` and `web/node_modules` occurrence to live-shaped paths, preserving which default basename each case exercises (`dist`, `node_modules`).

In `TestFilterDefaults`, change the `skipDirs` slice:
```go
	skipDirs := [][2]string{
		{"pkg/dist", "dist"},
		{"external/node_modules", "node_modules"},
		{"pkg/vendor", "vendor"},
		{".git", ".git"},
	}
```

In `TestFilterIncludeOverrides`, retarget the `dist` include-override case and the unrelated-prune case:
```go
func TestFilterIncludeOverrides(t *testing.T) {
	f := NewFilter(Config{Include: []string{"pkg/dist/keep.js"}})

	// The included file survives the default dist exclusion...
	if f.SkipFile("pkg/dist/keep.js") {
		t.Error("included keep.js should not be skipped")
	}
	// ...its directory is descended (not pruned) so the walk can reach it...
	if f.SkipDir("pkg/dist", "dist") {
		t.Error("dist must be descended when an Include is under it")
	}
	// ...but its non-included siblings are still skipped.
	if !f.SkipFile("pkg/dist/bundle.js") {
		t.Error("non-included dist sibling should still be skipped")
	}
	// An unrelated default dir stays pruned.
	if !f.SkipDir("external/node_modules", "node_modules") {
		t.Error("unrelated node_modules should still be pruned")
	}
}
```

- [ ] **Step 3: Run the config tests to verify they pass**

Run:
```bash
go test ./internal/config/ -run TestFilter -v
```
Expected: PASS (`TestFilterDefaults`, `TestFilterUserExclude`, `TestFilterIncludeOverrides`).

- [ ] **Step 4: Fix `internal/merkle/walk_test.go` fixtures**

Retarget both fixture maps and the assertion lists.

In `TestWalkExcludesCompiledAndVendored`:
```go
	writeFiles(t, root, map[string]string{
		"internal/graph/store.go":     "package graph\n",
		"pkg/dist/assets/app.js":      "var x=1\n",
		"external/node_modules/react/index.js": "module.exports={}\n",
		"static/vendor.min.js":        "var y=2\n",
	})
```
and its excluded-assertion list:
```go
	for _, bad := range []string{
		"pkg/dist/assets/app.js",
		"external/node_modules/react/index.js",
		"static/vendor.min.js",
	} {
```

In `TestWalkIncludeReadmitsExcludedFile`:
```go
	writeFiles(t, root, map[string]string{
		"pkg/dist/assets/app.js": "var x=1\n",
		"pkg/dist/keep.js":       "export const keep=1\n",
		".codeindex.json":        `{"include":["pkg/dist/keep.js"]}`,
	})
```
and both assertions:
```go
	if !slices.Contains(got, "pkg/dist/keep.js") {
		t.Errorf("expected included keep.js indexed; got %v", got)
	}
	if slices.Contains(got, "pkg/dist/assets/app.js") {
		t.Errorf("expected non-included dist sibling excluded; got %v", got)
	}
```

- [ ] **Step 5: Run the merkle tests to verify they pass**

Run:
```bash
go test ./internal/merkle/ -run TestWalk -v
```
Expected: PASS (`TestWalkExcludesCompiledAndVendored`, `TestWalkIncludeReadmitsExcludedFile`).

- [ ] **Step 6: Confirm no `web/` or `internal/webserver/dist` example strings remain in these files**

Run:
```bash
git grep -n 'web/node_modules\|internal/webserver/dist' -- internal/config/ internal/merkle/ || echo "clean"
```
Expected: `clean` (no matches).

- [ ] **Step 7: Full build + test**

Run:
```bash
go build ./... && go test ./...
```
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/config/config.go internal/config/filter_test.go internal/merkle/walk_test.go
git commit -m "refactor: retarget stale web/dist example strings to live paths in config+merkle"
```

---

### Task 3: Delete the plugin's dead lore hooks

The lore CLI/engine those hooks called was removed in Phase 1, so `plugin/hooks/lore_capture.py` and `plugin/hooks/lore_context.py` are dead. Remove both scripts and their two wirings in `plugin/hooks/hooks.json` (the `UserPromptSubmit` block that runs `lore_context.py`, and the `Stop` block that runs `lore_capture.py`). Keep `prompt_context.py` (UserPromptSubmit) and `post_edit.py` (PostToolUse).

**Files:**
- Delete: `plugin/hooks/lore_capture.py`
- Delete: `plugin/hooks/lore_context.py`
- Modify: `plugin/hooks/hooks.json`

- [ ] **Step 1: Delete the two lore hook scripts**

Run:
```bash
git rm plugin/hooks/lore_capture.py plugin/hooks/lore_context.py
```

- [ ] **Step 2: Rewrite `plugin/hooks/hooks.json` without the lore wirings**

Replace the file contents with (drops the second `UserPromptSubmit` entry that ran `lore_context.py` and the entire `Stop` block that ran `lore_capture.py`):
```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "python3 \"${CLAUDE_PLUGIN_ROOT}/hooks/prompt_context.py\"",
            "timeout": 5
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Edit|Write|MultiEdit",
        "hooks": [
          {
            "type": "command",
            "command": "python3 \"${CLAUDE_PLUGIN_ROOT}/hooks/post_edit.py\"",
            "timeout": 15
          }
        ]
      }
    ]
  }
}
```

- [ ] **Step 3: Validate JSON + confirm no lore hook references remain**

Run:
```bash
python3 -c "import json; json.load(open('plugin/hooks/hooks.json')); print('valid json')"
git grep -n 'lore_context\|lore_capture' -- plugin/ || echo "no lore hook refs"
```
Expected: `valid json` then `no lore hook refs`.

- [ ] **Step 4: Commit**

```bash
git add plugin/hooks/hooks.json
git add -A plugin/hooks/
git commit -m "chore(plugin): remove dead lore hooks (lore_context, lore_capture)"
```

---

### Task 4: Purge lore copy from plugin docs + marketplace

`.claude-plugin/marketplace.json` and `plugin/README.md` still advertise lore. Remove the lore sentences from the marketplace descriptions, delete the `## Lore` section and `### Kill switches (lore hooks)` subsection from the plugin README, and fix the plugin README's stale `codeindex attach` mention (the `attach` CLI was removed in Phase 1).

**Files:**
- Modify: `.claude-plugin/marketplace.json`
- Modify: `plugin/README.md`

- [ ] **Step 1: De-lore `.claude-plugin/marketplace.json`**

Top-level `description`, replace:
```json
  "description": "codeindex: blast-radius code navigation and lore decision history for AI agents.",
```
with:
```json
  "description": "codeindex: blast-radius code navigation for AI agents.",
```
Plugin `description`, replace:
```json
      "description": "Blast-radius navigation for refactoring: who calls a symbol, what it calls, and what breaks if it changes — plus lore decision history and work-item tracking. Not a search tool; use grep to find things.",
```
with:
```json
      "description": "Blast-radius navigation for refactoring: who calls a symbol, what it calls, and what breaks if it changes. Not a search tool; use grep to find things.",
```

- [ ] **Step 2: Validate the JSON**

Run:
```bash
python3 -c "import json; json.load(open('.claude-plugin/marketplace.json')); print('valid json')"
```
Expected: `valid json`.

- [ ] **Step 3: Delete the `## Lore` section and its `### Kill switches (lore hooks)` subsection from `plugin/README.md`**

Remove everything from the line `## Lore` through the end of the `### Kill switches (lore hooks)` block (up to but NOT including `## Hook controls`). After the edit the document flows `## What you get` → `## Hook controls`.

- [ ] **Step 4: Fix the stale `attach` mention in `plugin/README.md` "Honest limits"**

The "Honest limits" list has a bullet:
```
- Vendored dependencies: run `codeindex attach <repo> --auto` (Go vendor /
  composer) to resolve calls into deps with `[dep ns@ver]` provenance; locally
  modified dep files overlay automatically and show `modified`.
```
`attach` was removed in Phase 1; dependency maps are now produced by `codeindex depmap`. Replace that bullet with:
```
- Vendored dependencies: `codeindex depmap <dir> --namespace <ns> --version <v>
  -o <out.db>` builds a dependency map so calls into deps resolve with
  `[dep ns@ver]` provenance.
```

- [ ] **Step 5: Confirm no lore copy remains in plugin docs**

Run:
```bash
git grep -in 'lore' -- plugin/README.md .claude-plugin/marketplace.json || echo "no lore copy"
git grep -n 'codeindex attach' -- plugin/README.md || echo "no attach refs"
```
Expected: `no lore copy` then `no attach refs`.

- [ ] **Step 6: Commit**

```bash
git add .claude-plugin/marketplace.json plugin/README.md
git commit -m "docs(plugin): purge lore copy + fix stale attach reference"
```

---

### Task 5: Fix the stale `serve` doc-comment

`cmd/codeindex/serve.go`'s `runServe` comment still says it serves "the read-only graph API and SPA" — the SPA/static hosting was removed in change 0003 (`serve` is now a headless JSON API).

**Files:**
- Modify: `cmd/codeindex/serve.go`

- [ ] **Step 1: Fix the comment**

Replace:
```go
// runServe freshens the index, then serves the read-only graph API and SPA.
```
with:
```go
// runServe freshens the index, then serves the read-only headless JSON graph API.
```

- [ ] **Step 2: Build + confirm no other stale SPA/static copy in serve path**

Run:
```bash
go build ./...
git grep -in 'SPA\|static' -- cmd/codeindex/serve.go || echo "no SPA/static copy in serve.go"
```
Expected: build PASS; `no SPA/static copy in serve.go`.

- [ ] **Step 3: Commit**

```bash
git add cmd/codeindex/serve.go
git commit -m "docs: serve is a headless JSON API, not an SPA host"
```

---

### Task 6: Rewrite `README.md` to pure blast-radius positioning + graph API

Remove the lore engine sections (`## Lore` and its `### Host setup`, `### Third-party sync` subsections), drop the deleted `tree`/`attach`/`lore` CLI lines, replace the stale "Dependencies" `attach` copy, add a graph-API section for the decoupled `serve` endpoint, and refresh the repository-layout block. Keep the accurate existing positioning: blast-radius framing, token-savings evidence, supported languages, install/usage, MCP/plugin/CI sections.

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Remove the `codeindex tree` line from the `## Commands` block**

In the fenced command list, delete the line:
```
codeindex enclosing <repo> <file> <a>:<b> which symbol encloses these lines
codeindex tree <repo>                     interactive tree explorer (static print when piped)
```
→ keep `enclosing`, delete the `tree` line. Then add a `serve` line after the `mcp` line in the same block:
```
codeindex mcp <repo>                      serve the index over MCP (stdio)
codeindex serve <repo> [--addr host:port] headless JSON graph API (default 127.0.0.1:7676)
codeindex bench <repo> [out.json]         throughput benchmark and incremental-vs-full check
```

- [ ] **Step 2: Delete the entire `## Lore: decisions, work items, and notes` section**

Remove everything from the header `## Lore: decisions, work items, and notes` through the end of its `### Third-party sync` subsection — i.e. up to but NOT including `## Using it with Claude Code`. That deletes the lore CLI block, `### Host setup`, and `### Third-party sync` in one cut.

- [ ] **Step 3: Insert a graph-API section before `## Using it with Claude Code`**

Add this new section where the deleted `## Lore` section was (immediately before `## Using it with Claude Code`):
```markdown
## Graph API: query the symbol graph over HTTP

`codeindex serve <repo>` exposes the project's symbol call graph as a headless,
read-only JSON API over loopback HTTP — no static hosting, just data other tools
can consume. It freshens the index on start and binds to `127.0.0.1:7676` by
default (`--addr host:port` to change it). Every graph response carries a
top-level `schemaVersion` so external consumers are insulated from internal
shape changes.

```sh
codeindex serve /path/to/your/repo
# GET /api/health            -> { "status": "ok", "version": "<build>", "root": "<repo>" }
# GET /api/graph?symbol=Foo  -> Foo's neighborhood: focus + direct callers + callees
# GET /api/graph/full        -> the whole symbol graph (nodes grouped by package dir)
```

The full request/response contract — node and edge shapes, the `parent`
disambiguator, and the versioning policy — is documented in
[docs/graph-api.md](docs/graph-api.md).
```

- [ ] **Step 4: Rewrite the `## Using it with Claude Code` section to drop the lore pointer**

The section's last line points at `plugin/README.md` "for details and the measurement history behind the design" — keep that (it is still accurate). Verify no lore wording is inside this section after Step 2's cut; the three bullet points (prompt note, post-edit hook, `/codeindex:impact`) and the install block stay as-is. No edit needed here unless a lore sentence remains — if so, remove it.

- [ ] **Step 5: Fix the `## Dependencies` section (stale `attach`)**

The `## Dependencies` section uses the removed `codeindex attach` command. Replace the whole section body with a `depmap`-based description:
```markdown
## Dependencies

Calls into vendored or installed dependencies can be resolved by building a
dependency map and indexing it:

```sh
codeindex depmap /path/to/dep --namespace <ns> --version <v> -o dep-map.db
```

Resolved dependency symbols show `[dep namespace@version]` provenance, and
locally modified dependency files overlay the attached map and are marked
`modified`.
```

- [ ] **Step 6: Refresh the `## Repository layout` block**

Replace the layout block so it drops `.lore/` and reflects the current tree (add `internal/readmodel` and `internal/webserver`, keep the rest):
```
cmd/codeindex        the CLI
internal/adapter     language adapters (tree-sitter based)
internal/graph       SQLite store, data model, name resolution
internal/merkle      file walking, content hashing, change detection
internal/engine      build and incremental patch orchestration
internal/query       query layer (auto-build and patch-on-query)
internal/readmodel   symbol read model behind the graph API
internal/webserver   headless JSON graph API (codeindex serve)
internal/mcpserver   MCP server
internal/depmap      dependency map generation
plugin/              Claude Code plugin (hooks and /impact command)
editors/vscode       VS Code integration
bench/               benchmarks and A/B experiment findings
docs/                CI setup, the graph API contract, and design docs
```

- [ ] **Step 7: Confirm the README no longer references removed surface**

Run:
```bash
git grep -in 'lore\|codeindex tree\|codeindex attach\|graph-ui\|galaxy\|starfield\|react' -- README.md || echo "README clean"
```
Expected: `README clean` (no matches).

- [ ] **Step 8: Commit**

```bash
git add README.md
git commit -m "docs: rewrite README to blast-radius + graph-API positioning; drop lore"
```

---

### Task 7: Final gate

**Files:** none (verification only).

- [ ] **Step 1: Full build + test**

Run:
```bash
cd /Users/ethanhinson/codeindex/.worktrees/cleanup-delete-lore-rewrite-readme
go build ./... && go test ./...
```
Expected: PASS, no failures.

- [ ] **Step 2: Repo-wide lore-surface sweep (excluding historical design docs)**

Run:
```bash
git grep -In '\.lore\b\|codeindex lore\|lore_\|lore init\|lore_tools' -- . ':(exclude)docs/superpowers/*' ':(exclude)bench/*' || echo "no live lore surface remains"
```
Expected: `no live lore surface remains`. (Matches inside `docs/superpowers/*` historical plans/specs and the `bench/` session-log fixture are expected and intentionally out of scope.)

- [ ] **Step 3: Smoke the serve API on this repo (optional but recommended)**

Run:
```bash
go run ./cmd/codeindex serve . --addr 127.0.0.1:7699 &
SRV=$!; sleep 3
curl -s http://127.0.0.1:7699/api/health
kill $SRV 2>/dev/null
```
Expected: a JSON health object with `"status":"ok"` and a `schemaVersion`-free health payload (health is `{status,version,root}`); the graph endpoints carry `schemaVersion`.

---

## Self-Review

**Spec coverage (Phase 3 of the design doc):**
- "Delete `.lore/`" → Task 1. ✓
- "drop lore/web indexing excludes and `lore.db` handling from config" → Task 2 (reconcile established there are no such excludes/handling in config code; the real deferred item is the stale example strings, which Task 2 fixes). ✓
- "Rewrite `README.md` to the pure blast-radius positioning (drop lore/graph-UI sections)" → Task 6. ✓
- Reconciled additions (de-advertise the removed product): plugin lore hooks → Task 3; plugin/marketplace lore copy → Task 4; stale serve doc-comment → Task 5. ✓
- `bench/` audit → resolved in reconcile (no real reference); Task 7 Step 2 guards it. ✓

**Placeholder scan:** every code/doc step carries the actual replacement content or an exact deletion boundary. No TODO/TBD. ✓

**Type consistency:** no new types or signatures introduced; all Go edits are comment/fixture-string retargets that preserve `config.NewFilter`/`Filter.SkipDir`/`Filter.SkipFile`/`merkle.Walk` behavior. Fixture path renames are self-consistent within each test (`pkg/dist`, `external/node_modules`). ✓
