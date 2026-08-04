<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0002 — Rip out the lore product surface (engine, CLI, MCP, plugin skills)](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0002-rip-out-lore-product-surface.md)**
<!-- docket:backlink:end -->

# Rip Out the Lore Product Surface — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the lore product surface (plugin skills, MCP lore tools + enrichment, CLI `lore`/`tree`/`attach` subcommands, and lore-product-only `internal/lore` files) from codeindex, returning it toward a single blast-radius/impact tool — while keeping the build and test suite green after every task.

**Architecture:** This is a pure code-*removal* change executed as an ordered sequence of independent deletions/edits. The order is chosen so that each task ends with `go build ./...` and `go test ./...` green (green-build-per-step). Higher layers (plugin docs, MCP, CLL) come off first; the shared `internal/lore` leaf files come off last, only after their sole callers are gone.

**Tech Stack:** Go 1.26.5, SQLite-backed call-graph engine, `github.com/modelcontextprotocol/go-sdk/mcp`.

## Global Constraints

- Go version: **1.26.5**.
- **HARD CONSTRAINT — green build per step:** `go build ./...` AND `go test ./...` must pass at the end of every task. Never leave the tree red between commits.
- Working directory: `/Users/ethanhinson/codeindex/.worktrees/rip-out-lore-product-surface` (branch `feat/rip-out-lore-product-surface`, cut from `origin/main`). All paths below are relative to this worktree root.
- **OUT OF SCOPE — do not touch:** `internal/readmodel/**`, `internal/webserver/**`, `web/**`, `.lore/` data, `README.md`. These belong to changes 0003/0004.
- **DO NOT DELETE** these `internal/lore` pieces — kept packages still import them (Phase 2 / change 0003 removes them): `internal/lore/layout.go`, `internal/lore/record.go`, the whole `internal/lore/index/` package, the whole `internal/lore/gitinfo/` package. Rationale: `internal/readmodel` imports `codeindex/internal/lore` + `codeindex/internal/lore/index`; `internal/lore/index/reindex.go` imports `codeindex/internal/lore/gitinfo`; `internal/webserver/server_test.go` imports `codeindex/internal/lore`. Deleting any of them breaks `go build ./...`.
- This is a removal change: the "test" for each task is (a) the suite stays green and (b) the removed surface is provably gone (grep returns nothing). Where an existing test asserts a removed behavior, **edit that test** rather than write a new one — this is called out per task.

---

## File Structure

Files touched, by task:

- **Task 1** — delete `plugin/commands/decide.md`, `plugin/commands/lore.md`. Keep `plugin/commands/impact.md`.
- **Task 2** — edit `internal/mcpserver/mcpserver.go`; delete `internal/mcpserver/lore_tools.go`, `internal/mcpserver/lore_tools_test.go`; edit `internal/mcpserver/mcpserver_test.go`.
- **Task 3** — delete `cmd/codeindex/lore.go`, `cmd/codeindex/lore_test.go`, `cmd/codeindex/tree.go`; edit `cmd/codeindex/main.go`.
- **Task 4** — delete `internal/lore/ghsync/` (whole dir), `internal/lore/capture.go`, `internal/lore/capture_test.go`.
- **Task 5** — edit `internal/graph/store.go` (remove `ProjectSymbols`); edit `internal/graph/store_test.go` (remove `TestProjectSymbolsOrderedByFileAndLine`).

---

## Task 1: Remove the lore plugin skills

Deletes the `decide` and `lore` plugin command docs. No Go build impact — pure doc removal — so it goes first and stands alone.

**Files:**
- Delete: `plugin/commands/decide.md`
- Delete: `plugin/commands/lore.md`
- Keep (do not touch): `plugin/commands/impact.md`

**Interfaces:**
- Consumes: nothing.
- Produces: nothing (these are markdown skill docs, not imported by Go).

- [ ] **Step 1: Confirm the three files exist and note that impact.md stays**

Run:
```bash
ls plugin/commands/decide.md plugin/commands/lore.md plugin/commands/impact.md
```
Expected: all three listed.

- [ ] **Step 2: Delete the two lore plugin skills**

Run:
```bash
git rm plugin/commands/decide.md plugin/commands/lore.md
```

- [ ] **Step 3: Verify removal and that impact.md survives**

Run:
```bash
test ! -e plugin/commands/decide.md && test ! -e plugin/commands/lore.md && test -e plugin/commands/impact.md && echo OK
```
Expected: `OK`

- [ ] **Step 4: Verify build + tests stay green**

Run:
```bash
go build ./... && go test ./...
```
Expected: build succeeds; all tests PASS (unchanged — no Go touched).

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: remove decide/lore plugin skills (keep impact)"
```

---

## Task 2: De-lore the MCP server

Removes the "Related lore" enrichment from the kept MCP tools and deletes the five lore_* tools. `mcpserver.go` (kept) currently calls `relatedLoreBlock` and `addLoreTools`, both defined in `lore_tools.go`; those calls must go before/with the file deletion, and `mcpserver_test.go` asserts the lore tools exist, so it is edited in the same task.

**Files:**
- Modify: `internal/mcpserver/mcpserver.go` (lines ~62, ~77 — Related-lore description sentences and the `out += relatedLoreBlock(...)` calls in the `impact` and `callers` handlers; line ~163 — `addLoreTools(s, repo)`)
- Delete: `internal/mcpserver/lore_tools.go`
- Delete: `internal/mcpserver/lore_tools_test.go`
- Modify: `internal/mcpserver/mcpserver_test.go` (the `loreTools` map + its uses, and the lore names in the "want" list, ~lines 63-83)

**Interfaces:**
- Consumes: the kept `query.*Text` functions (`ImpactText`, `CallersText`, etc.) — unchanged.
- Produces: a `mcpserver.New(repo, version)` server exposing exactly `impact`, `callers`, `callees`, `dependents`, `find`, `grep` — no `lore_*` tools, no "Related lore" text, no dependency on `internal/lore` from this package.

- [ ] **Step 1: Edit the kept-tool call sites and descriptions in `mcpserver.go`**

In `internal/mcpserver/mcpserver.go`:

In the **`impact`** tool: remove the `relatedLoreBlock` call so the handler returns the raw impact text:
```go
	}, func(ctx context.Context, req *mcp.CallToolRequest, in symbolArgs) (*mcp.CallToolResult, any, error) {
		out, err := query.ImpactText(repo, in.Symbol, limitOr(in.Limit))
		if err != nil {
			return nil, nil, fmt.Errorf("impact %q: %w", in.Symbol, err)
		}
		return text(out), nil, nil
	})
```
And in its `Description`, delete the sentence `"Output may include a Related lore section — decisions and open work items attached to this symbol; treat active decisions as constraints. "` so the line reads:
```go
			"dead-code checks. " + trust + notFor,
```

In the **`callers`** tool: remove the `relatedLoreBlock` call the same way:
```go
	}, func(ctx context.Context, req *mcp.CallToolRequest, in symbolArgs) (*mcp.CallToolResult, any, error) {
		out, err := query.CallersText(repo, in.Symbol, limitOr(in.Limit))
		if err != nil {
			return nil, nil, fmt.Errorf("callers %q: %w", in.Symbol, err)
		}
		return text(out), nil, nil
	})
```
And delete the same "Related lore" sentence from its `Description`, so the surrounding lines read:
```go
			"Use for 'who calls X / which functions use X / is X dead code'. " +
			trust + notFor,
```

- [ ] **Step 2: Remove the lore-tool registration in `mcpserver.go`**

Delete the line:
```go
	addLoreTools(s, repo)
```
(near the end of `New`, just before `return s`). Leave the surrounding `return s` intact.

- [ ] **Step 3: Grep-confirm `mcpserver.go` no longer references lore symbols**

Run:
```bash
grep -n -iE "relatedLoreBlock|addLoreTools|Related lore" internal/mcpserver/mcpserver.go
```
Expected: no output.

- [ ] **Step 4: Delete the lore-tools implementation + its test**

Run:
```bash
git rm internal/mcpserver/lore_tools.go internal/mcpserver/lore_tools_test.go
```

- [ ] **Step 5: Update `mcpserver_test.go` to the kept tool set**

In `internal/mcpserver/mcpserver_test.go`, in `TestHandshakeListAndCall`:

Delete the `loreTools` map declaration and its explanatory comment:
```go
	// lore_* tools use a different trust model (committed files, not a derived
	// code-graph index) — they carry loreTrust, not the "COMPLETE" graph trust.
	loreTools := map[string]bool{
		"lore_search": true, "lore_for_symbol": true, "lore_backlog": true,
		"lore_show": true, "lore_add": true,
	}
```

Change the trust-language check to no longer exempt lore tools — from:
```go
		if !loreTools[tl.Name] && !strings.Contains(tl.Description, "COMPLETE") {
			t.Errorf("tool %s description missing trust language", tl.Name)
		}
```
to:
```go
		if !strings.Contains(tl.Description, "COMPLETE") {
			t.Errorf("tool %s description missing trust language", tl.Name)
		}
```

Remove the five lore names from the "want" list — from:
```go
	for _, want := range []string{"impact", "callers", "callees", "dependents", "find", "grep",
		"lore_search", "lore_for_symbol", "lore_backlog", "lore_show", "lore_add"} {
```
to:
```go
	for _, want := range []string{"impact", "callers", "callees", "dependents", "find", "grep"} {
```

(`strings` is still used by other `strings.Contains` calls in the file, so leave the import.)

- [ ] **Step 6: Grep-confirm no lore references remain in the mcpserver package**

Run:
```bash
grep -rn -iE "lore" internal/mcpserver/
```
Expected: no output.

- [ ] **Step 7: Verify build + tests green (run the package tests explicitly, then the full suite)**

Run:
```bash
go build ./... && go test ./internal/mcpserver/... && go test ./...
```
Expected: build succeeds; `mcpserver` tests PASS; full suite PASS.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "refactor: remove lore_* MCP tools and Related-lore enrichment"
```

---

## Task 3: Remove the CLI lore/tree/attach surface

Deletes the `lore`, `tree`, and `attach` CLI subcommands and strips the `--related-depth` enrichment from `impact`. `main.go` imports `codeindex/internal/lore/index` solely for `index.RelatedLoreBlock` inside `relatedLoreForImpact`; once that helper is gone the import is dropped. `loreReindex` (used by `relatedLoreForImpact`) lives in the deleted `lore.go`, so all three must be reconciled together.

**Files:**
- Delete: `cmd/codeindex/lore.go`
- Delete: `cmd/codeindex/lore_test.go`
- Delete: `cmd/codeindex/tree.go`
- Modify: `cmd/codeindex/main.go` (import block ~line 20; usage string ~line 32; `impact` case ~lines 90-108; `attach` case ~lines 151-177; `tree`/`lore` cases ~lines 238-245; `runImpact` + `relatedLoreForImpact` ~lines 597-621)

**Interfaces:**
- Consumes: `query.ImpactText`, `depmap.*` (unchanged — `depmap.Attach`/`AutoAttach` stay exported and used elsewhere by the `depmap` subcommand), `graph.*`.
- Produces: a `codeindex` CLI whose dispatch has no `lore`, `tree`, or `attach` arms; `impact` takes only `[--limit N]`; `runImpact(root, name string, limit int) error` prints only `query.ImpactText` output.

- [ ] **Step 1: Delete the lore CLI file, its test, and the tree TUI file**

Run:
```bash
git rm cmd/codeindex/lore.go cmd/codeindex/lore_test.go cmd/codeindex/tree.go
```

- [ ] **Step 2: Drop the `internal/lore/index` import from `main.go`**

In `cmd/codeindex/main.go`, delete the import line:
```go
	"codeindex/internal/lore/index"
```
from the import block. Leave every other import (`depmap`, `engine`, `graph`, `mcpserver`, `merkle`, `progress`, `query`) in place.

- [ ] **Step 3: Remove `lore`/`tree`/`attach` from the usage string**

In `main.go` near line 32, change the top-level usage string so it no longer lists `tree`, `attach`, or `lore`. From:
```go
			"usage: codeindex <build|refresh|status|callers|callees|impact|dependents|deps|find|grep|tree|depmap|attach|export|import|enclosing|lore|serve|mcp|bench> <repo-root> ...")
```
to:
```go
			"usage: codeindex <build|refresh|status|callers|callees|impact|dependents|deps|find|grep|depmap|export|import|enclosing|serve|mcp|bench> <repo-root> ...")
```

- [ ] **Step 4: Strip `--related-depth` from the `impact` dispatch case**

In `main.go`, replace the whole `case "impact":` block. From:
```go
	case "impact":
		if len(os.Args) < 4 {
			fatal(fmt.Errorf("usage: codeindex impact <repo-root> <symbol> [--limit N] [--related-depth N|all]"))
		}
		limit := 50
		relatedDepth := 2
		for i := 4; i < len(os.Args)-1; i++ {
			switch os.Args[i] {
			case "--limit":
				fmt.Sscanf(os.Args[i+1], "%d", &limit)
			case "--related-depth":
				if os.Args[i+1] == "all" {
					relatedDepth = -1
				} else {
					fmt.Sscanf(os.Args[i+1], "%d", &relatedDepth)
				}
			}
		}
		if err := runImpact(root, os.Args[3], limit, relatedDepth); err != nil {
			fatal(err)
		}
```
to:
```go
	case "impact":
		if len(os.Args) < 4 {
			fatal(fmt.Errorf("usage: codeindex impact <repo-root> <symbol> [--limit N]"))
		}
		limit := 50
		for i := 4; i < len(os.Args)-1; i++ {
			if os.Args[i] == "--limit" {
				fmt.Sscanf(os.Args[i+1], "%d", &limit)
			}
		}
		if err := runImpact(root, os.Args[3], limit); err != nil {
			fatal(err)
		}
```

- [ ] **Step 5: Delete the `attach` dispatch case**

In `main.go`, delete the entire `case "attach":` block (from `case "attach":` through the closing `}` of its `else`, immediately before `case "find":`). This is the block that calls `depmap.AutoAttach` / `depmap.Attach`. Do not touch the `depmap` package itself.

- [ ] **Step 6: Delete the `tree` and `lore` dispatch cases**

In `main.go`, delete these two arms:
```go
	case "tree":
		if err := runTree(root); err != nil {
			fatal(err)
		}
	case "lore":
		if err := runLore(root, os.Args[3:], os.Stdout); err != nil {
			fatal(err)
		}
```
Leave the neighboring `case "callees":` and `case "enclosing":` arms intact.

- [ ] **Step 7: Simplify `runImpact` and delete `relatedLoreForImpact`**

In `main.go`, replace the `runImpact` function and delete `relatedLoreForImpact` entirely. From:
```go
// runImpact prints the counts-first blast-radius summary, then any related
// lore. Lore must never break navigation: a lore failure drops the block.
func runImpact(root, name string, limit, relatedDepth int) error {
	out, err := query.ImpactText(root, name, limit)
	if err != nil {
		return err
	}
	fmt.Print(out)
	fmt.Print(relatedLoreForImpact(root, name, relatedDepth))
	return nil
}

// relatedLoreForImpact returns the related-lore block or "" on any error.
func relatedLoreForImpact(root, symbol string, depth int) string {
	_, st, _, err := loreReindex(root)
	if err != nil {
		return ""
	}
	defer st.Close()
	all, err := st.All()
	if err != nil {
		return ""
	}
	return index.RelatedLoreBlock(all, symbol, depth)
}
```
to:
```go
// runImpact prints the counts-first blast-radius summary for a symbol.
func runImpact(root, name string, limit int) error {
	out, err := query.ImpactText(root, name, limit)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}
```

- [ ] **Step 8: Grep-confirm the CLI no longer references lore/tree/attach helpers**

Run:
```bash
grep -rn -iE "runLore|runTree|relatedLoreForImpact|loreReindex|related-depth|internal/lore" cmd/codeindex/
```
Expected: no output.

- [ ] **Step 9: Verify build + full test suite green**

Run:
```bash
go build ./... && go test ./...
```
Expected: build succeeds; full suite PASS. (If the compiler flags an unused import or symbol, fix it — but per the edits above nothing should dangle: `index` was the only `internal/lore/index` use, and `depmap` is still used by the `depmap` command.)

- [ ] **Step 10: Smoke-check the CLI usage no longer advertises removed commands**

Run:
```bash
go run ./cmd/codeindex 2>&1 | grep -E "tree|attach|lore" || echo "no removed commands in usage — OK"
```
Expected: `no removed commands in usage — OK`

- [ ] **Step 11: Commit**

```bash
git add -A
git commit -m "refactor: remove lore/tree/attach CLI and impact --related-depth"
```

---

## Task 4: Delete lore-product-only `internal/lore` files

Removes the `internal/lore/ghsync` package (only the now-deleted `cmd/codeindex/lore.go` imported it) and `internal/lore/capture.go` + its test (`CaptureSession`, only the deleted `lore.go` used it). **This task must run after Task 3**, because Task 3 deletes the sole importers. Everything else under `internal/lore` (`layout.go`, `record.go`, `index/`, `gitinfo/`) stays — readmodel/webserver depend on it (Phase 2).

**Files:**
- Delete: `internal/lore/ghsync/` (entire directory)
- Delete: `internal/lore/capture.go`
- Delete: `internal/lore/capture_test.go`

**Interfaces:**
- Consumes: nothing (leaf removals).
- Produces: `internal/lore` retaining `layout.go` (`NewLayout`, `Layout`), `record.go` (`Record`, `Anchor`, `TypeDecision`, …), and the `index/` + `gitinfo/` subpackages — all still importable by `internal/readmodel` and `internal/webserver` (untouched).

- [ ] **Step 1: Verify no KEPT package imports `internal/lore/ghsync`**

Run:
```bash
grep -rln "internal/lore/ghsync" . --include="*.go"
```
Expected: no output (Task 3 deleted `cmd/codeindex/lore.go`, the only importer). If any KEPT file appears, STOP and reassess — do not delete.

- [ ] **Step 2: Verify `CaptureSession` has no remaining callers outside the capture files**

Run:
```bash
grep -rln "CaptureSession" . --include="*.go" | grep -v "internal/lore/capture"
```
Expected: no output. If a kept file appears, STOP and reassess.

- [ ] **Step 3: Delete the ghsync package and the capture files**

Run:
```bash
git rm -r internal/lore/ghsync
git rm internal/lore/capture.go internal/lore/capture_test.go
```

- [ ] **Step 4: Confirm the kept `internal/lore` pieces remain**

Run:
```bash
test -e internal/lore/layout.go && test -e internal/lore/record.go && test -d internal/lore/index && test -d internal/lore/gitinfo && echo OK
```
Expected: `OK`

- [ ] **Step 5: Verify build + full test suite green**

Run:
```bash
go build ./... && go test ./...
```
Expected: build succeeds; full suite PASS. (readmodel/webserver still compile against the retained `internal/lore` + `internal/lore/index` + `internal/lore/gitinfo`.)

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: delete lore-product-only ghsync + capture (keep index/gitinfo/layout/record)"
```

---

## Task 5: Remove the now-dead `ProjectSymbols` from the graph store

`internal/graph/store.go` has no functional lore code — only two comments. The single available cleanup is `ProjectSymbols`, whose sole caller was the deleted `cmd/codeindex/tree.go` ("the load query for the tree explorer"). Its test `TestProjectSymbolsOrderedByFileAndLine` is removed with it. **Must run after Task 3** (which deletes `tree.go`, the caller).

**Files:**
- Modify: `internal/graph/store.go` (remove the `ProjectSymbols` method + its doc comment, ~lines 579-596)
- Modify: `internal/graph/store_test.go` (remove `TestProjectSymbolsOrderedByFileAndLine`, ~lines 24-60)

**Interfaces:**
- Consumes: nothing.
- Produces: `internal/graph.Store` without a `ProjectSymbols` method. All other `Store` methods (`Open`, `Close`, `EnclosingSymbols`, `DumpNormalized`, resolver internals, etc.) are unchanged. The unrelated "attached-map" comment in `resolve()` stays.

- [ ] **Step 1: Verify `ProjectSymbols` has no remaining non-test caller**

Run:
```bash
grep -rn "ProjectSymbols" . --include="*.go"
```
Expected: only `internal/graph/store.go` (definition + doc comment) and `internal/graph/store_test.go` (the test) — no production caller (Task 3 removed `tree.go`).

- [ ] **Step 2: Remove the `ProjectSymbols` method from `store.go`**

In `internal/graph/store.go`, delete the doc comment + method:
```go
// ProjectSymbols returns every project (tier-0) symbol ordered by file then
// start line — the load query for the tree explorer.
func (s *Store) ProjectSymbols() ([]Symbol, error) {
	...
}
```
(the full function body through its closing `}`). Do not touch neighboring methods.

- [ ] **Step 3: Remove the corresponding test from `store_test.go`**

In `internal/graph/store_test.go`, delete the entire `func TestProjectSymbolsOrderedByFileAndLine(t *testing.T) { ... }` (through its closing `}`). If that deletion leaves any import used only by that test unreferenced, remove the now-unused import too (the compiler will name it).

- [ ] **Step 4: Grep-confirm `ProjectSymbols` is fully gone**

Run:
```bash
grep -rn "ProjectSymbols" . --include="*.go" || echo "gone — OK"
```
Expected: `gone — OK`

- [ ] **Step 5: Verify build + full test suite green (run the graph package explicitly first)**

Run:
```bash
go build ./... && go test ./internal/graph/... && go test ./...
```
Expected: build succeeds; graph tests PASS; full suite PASS.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: remove dead ProjectSymbols graph query (tree explorer gone)"
```

---

## Final verification (after all tasks)

- [ ] **Full green gate**

Run:
```bash
go build ./... && go test ./...
```
Expected: build succeeds; entire suite PASS.

- [ ] **Removed surface is provably gone**

Run:
```bash
echo "== plugin skills =="; ls plugin/commands/
echo "== cmd files =="; ls cmd/codeindex/
echo "== mcpserver lore =="; grep -rn -iE "lore" internal/mcpserver/ || echo "none"
echo "== cli lore/tree/attach =="; grep -rn -iE "runLore|runTree|\"attach\"|\"tree\"|\"lore\"|related-depth" cmd/codeindex/ || echo "none"
echo "== kept internal/lore =="; ls internal/lore/
```
Expected: no `decide.md`/`lore.md`; no `lore.go`/`lore_test.go`/`tree.go`; `none` for mcpserver lore and CLI lore/tree/attach; `internal/lore/` still shows `layout.go`, `record.go`, `index/`, `gitinfo/` (and NOT `ghsync/` or `capture.go`).

- [ ] **CLI smoke on this repo (optional sanity, not a gate)**

Run:
```bash
go run ./cmd/codeindex build . >/dev/null 2>&1 && go run ./cmd/codeindex impact . fatal --limit 5
```
Expected: an impact summary prints with no "Related lore" section and no error.

---

## Self-Review

**Spec coverage (Phase 1 of the spec's REMOVE list):**
- `decide.md` / `lore.md` plugin skills → Task 1. ✅
- `lore_tools.go` (`lore_search`/`lore_for_symbol`/`lore_backlog`/`lore_show`/`lore_add`) → Task 2. ✅
- `related_lore` / `--related-depth` from `impact` (MCP) → Task 2. ✅
- `related_lore` / `--related-depth` from `impact` (CLI) → Task 3. ✅
- `lore` subcommand (`cmd/codeindex/lore.go`) → Task 3. ✅
- `tree` (`cmd/codeindex/tree.go`) → Task 3. ✅
- `attach` subcommand → Task 3. ✅
- lore imports in `cmd/codeindex/main.go` → Task 3 (drops `internal/lore/index` import). ✅
- lore-product-only `internal/lore` files → Task 4 (`ghsync/`, `capture.go`). ✅
- lore code in `internal/graph/store.go` → Task 5 (dead `ProjectSymbols`; reconcile established there is no *functional* lore code). ✅
- Kept surfaces (`impact`/`callers`/`callees`/`dependents`/`find`/`grep`, `impact.md`) → untouched, asserted green. ✅
- **Deferred to change 0003 (out of scope, guarded):** the `internal/lore` bulk deletion (root + `index/` + `gitinfo/`) and the readmodel lore overlay — not deleted here because readmodel/webserver import them. ✅

**Placeholder scan:** No "TBD"/"handle edge cases"/"similar to Task N" — every edit shows the exact before/after code. ✅

**Type consistency:** `runImpact` is redefined once (Task 3) as `runImpact(root, name string, limit int) error` and its single call site in the `impact` dispatch case is updated in the same task. No other signature changes. ✅
