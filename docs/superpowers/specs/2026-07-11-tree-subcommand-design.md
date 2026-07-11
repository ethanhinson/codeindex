# `tree` Subcommand — Design

**Date:** 2026-07-11
**Status:** Approved for planning

## Summary

Add `codeindex tree <repo>`: an interactive terminal UI for exploring the
repo structure tree held by the index — directories → files → symbols —
with a live filter and a detail side panel. Built with Bubble Tea +
Lipgloss. When stdout is not a TTY, it prints a static indented tree
instead.

## Goals

- Let a human browse everything in the index as a symbol-aware outline.
- Beautiful by default: adaptive colors, rounded borders, subtle styling.
- Instant navigation on large repos (tens of thousands of symbols).

## Non-Goals (v1)

- No call-tree or dependency-tree pivots (natural v2 from this view).
- No "open in editor" keybinding.
- No mouse support.

## Architecture

**Command dispatch:** `tree` case in `cmd/codeindex/main.go`, like other
subcommands. Runs the standard freshness check (`query.Fresh`) first, so
the tree always reflects current code; an unindexed repo builds first with
progress on stderr.

**New packages:**

- `internal/tui/tree` — the Bubble Tea app: model, update, view, styles.
  The query layer stays TUI-free.
- `internal/graph/store.go` gains one method: `ProjectSymbols()` — all
  tier-0 symbols ordered by file and start line. One indexed SQL scan;
  ~50k symbols load in well under a second and fit in memory.

**Tree construction (in-memory, at startup):** built from file paths —
directories → files → symbols — with methods nested under their parent
type via the existing `Symbol.Parent` field. Directories serve as the
"package" level; no per-language logic.

**Lazy detail data:** the detail pane's signature, kind, and `file:line`
come from the loaded symbol. Caller/callee counts are fetched on
selection via the existing `Store.Callers`/`Store.Callees` and cached per
symbol, so cursor movement is instant and counts are never precomputed
for symbols nobody views.

**Non-TTY fallback:** if stdout is not a terminal, print a static
indented tree of the same structure and exit. Consistent with the tool's
existing TTY adaptation (`progress.IsTTY`).

**Dependencies added:** `github.com/charmbracelet/bubbletea`,
`github.com/charmbracelet/lipgloss` (plus transitive deps).

## Interaction & Visual Design

```
 codeindex tree — myrepo                                    2,341 symbols
┌─────────────────────────────────────┐┌───────────────────────────────┐
│ ▾ internal/                         ││ Store.Callers                 │
│   ▾ graph/                          ││ method · store.go:629         │
│     ▸ depmaps.go                    ││                               │
│     ▾ store.go                      ││ func (s *Store) Callers(      │
│       ▸ Store            struct     ││   name, parent string,        │
│       ▾ Store (methods)             ││ ) ([]Caller, error)           │
│         Callers          method  ●  ││                               │
│         Callees          method     ││ called by  7                  │
│         Definitions      method     ││ calls      3                  │
│   ▸ query/                          ││                               │
│ ▸ cmd/                              ││                               │
└─────────────────────────────────────┘└───────────────────────────────┘
 ↑↓ move · ←→ collapse/expand · / filter · enter toggle · q quit
```

- **Layout:** two Lipgloss-bordered panes — tree ~60% left, detail right.
  Header: repo name + symbol count. Footer: key hints. Below 80 columns
  the detail pane hides and the tree goes full width.
- **Styling:** adaptive colors for light/dark terminals; kind badges
  (`struct`, `method`, `func`, …) muted; cursor line gets a subtle
  background highlight; filter matches highlighted in an accent color;
  directories bold; `▸`/`▾` affordances only on expandable nodes.
- **Keys:**
  - `↑`/`↓` or `j`/`k` — move cursor (scrolls, cursor kept in view)
  - `←`/`→` or `h`/`l` — collapse/expand; `enter` toggles dirs/files
  - `/` — live filter: tree narrows as you type, ancestor paths of
    matches auto-expand; `esc` clears
  - `q` / `ctrl+c` — quit

## Error Handling

- **Unindexed repo:** freshness check builds first (existing behavior),
  then the TUI launches.
- **Empty index (zero symbols):** print a plain message, exit 0. No TUI.
- **Terminal too small (<40 cols or <10 rows):** render a "terminal too
  small" notice; recover on resize.
- **Count lookup failure in detail pane:** show `—`; never crash.

## Testing

- Tree construction, filtering (match + ancestor auto-expand), and
  flatten-for-display (visible rows given expand state) are pure
  functions in `internal/tui/tree` — table-driven unit tests, no TTY.
- Update logic tested by feeding `tea.KeyMsg` values to the model and
  asserting on state.
- Static non-TTY output: golden-style test against a small fixture tree.
- `ProjectSymbols()`: test alongside existing store tests.
- `View()`: smoke test only (no panic, expected labels present) —
  pixel-perfect render assertions are brittle and skipped.
