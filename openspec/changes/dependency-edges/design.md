## Context

The edges table has carried `kind` since the skeleton, and `src_file` +
`dst_name`/`dst_qualifier` give us everything dependency edges need — no new
columns. The adapters already walk every node these facts live in. What's
missing is emission (adapters), a generalized raw-edge shape (ParsedFile only
carries calls), file-level edge sources (imports belong to files, not
symbols), and the query surface.

## Goals / Non-Goals

**Goals**: import/extends/implements edges in all four languages;
dependents/deps queries (CLI + MCP); `/impact` composition + truthful coverage
line; six-repo equivalence proof; per-language spot-checks.

**Non-Goals**: Go implicit interface satisfaction (type checking), module-path
aliasing/resolution, `references` (type-usage) edges, transitive closure,
paid agent A/B (extends a validated query shape; engine proof per the
language-adapters precedent).

## Decisions

**D1 — Generalize the raw shape: `ParsedFile.Deps []RawDep`.**
`RawDep{EnclosingIdx, Kind, Target, Line}`; `EnclosingIdx = -1` means
file-level (imports). Calls stay as-is — no churn in the proven path.

**D2 — File-level edges use `src_symbol_id = 0` + `src_file`.** The schema
already supports it: per-file replacement deletes by `src_file`, and display
falls back to the file name. Snapshot's edge query switches the src join to a
LEFT JOIN with `COALESCE(sc.name,'<file>')` so equivalence still holds.

**D3 — extends/implements originate from the class symbol** (the heritage
clause sits inside the class span, so the existing innermost-enclosing
attribution lands them correctly) and resolve name-based through the existing
`resolve(name, qualifier)` — a base class is a name like any call target.

**D4 — Go import paths stay verbatim and unresolved.** `import
"codeindex/internal/graph"` → dst_name is the path (packages aren't symbols;
fabricating resolution would violate the no-fake-targets rule). The dependents
query matches exact dst_name OR last path segment (`dependents graph` and
`dependents codeindex/internal/graph` both work); documented. TS/Python/PHP
import targets are symbol names and resolve normally.

**D5 — Schema version 2 → 3 with no structural change**, purely to force
existing indexes to rebuild: incremental patching only re-parses *changed*
files, so without the bump, old indexes would silently lack dependency edges
for unchanged files. The auto-rebuild machinery from precise-resolution
handles it (measured 96 ms–31.7 s).

**D6 — `deps <anchor>` is dual-mode**: if the anchor matches an indexed file
path, it lists that file's imports; otherwise it lists the symbol's outgoing
extends/implements (+ its file's imports for context, labeled). `dependents
<anchor>` is symbol/module-mode only.

**D7 — `/impact` gains a dependents section** (counts-first, same bounds) and
its coverage line becomes "call + import/extends/implements edges; references
(type-usage) not included" — the disclosure shrinks but stays honest.

## Risks / Trade-offs

- **Import-edge volume inflates the index** (every file imports things) →
  re-measure sizes in bench; expected small relative to call edges; still
  under the ≤2× bound or recorded as a deviation.
- **PHP `use` is overloaded** (imports at top, trait-use in class body) →
  distinguish by lexical position: namespace-level `use` → imports; in-class
  `use` → extends-like trait edge (kind implements, documented).
- **TS default/namespace imports** (`import X from`, `import * as ns`) → X/ns
  emitted as import edges; `* as ns` target is the alias — skip (alias isn't
  a repo symbol); named + default only.
- **Last-segment matching for Go can over-match** (two packages ending
  `/util`) → both shown with full paths; the full-path anchor stays exact.

## Migration Plan

Version bump → automatic rebuild on first touch. Rollback: revert binary
(older binaries rebuild v2 the same way).

## Open Questions

- Whether `dependents` should also count call edges ("full blast radius") —
  `/impact` already composes both; keep `dependents` dependency-only for
  orthogonality (decided, noted).
