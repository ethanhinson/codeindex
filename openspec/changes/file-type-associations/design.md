## Context

adapter registry maps extension → Adapter; merkle.Walk filters by
adapter.Extensions(); engine's parseAll calls adapter.For(relPath). One
registry, one walk filter, one lookup — three touch points total.

## Goals / Non-Goals

**Goals**: robust real-world PHP/TS/Python coverage; explicit, committed,
deterministic association config; config changes flow through the existing
incremental gate.
**Non-Goals**: content sniffing; IDE-side language overrides; ignore rules.

## Decisions

**D1 — Committed `.codeindex.json` at repo root.** Associations are a
property of the repo (Drupal repos always need them), so the file is
committed and every developer + CI inherits it. Loaded fresh on every
Build/Patch — a long-lived MCP server picks up edits on the next query.

**D2 — Associations resolve at the adapter seam, checked before
extensions.** adapter.SetAssociations(pattern→language) is applied by the
engine at build entry; For(relPath) tries association rules (sorted
patterns, path.Match on basename or on the full relative path when the
pattern contains '/'), then the extension registry. Walk defers to
adapter.Indexable — the registry stays the single source of truth for what
gets indexed. Global set-at-entry state is acceptable: query.mu already
serializes index mutation; bench runs repos sequentially. Recorded.

**D3 — Unknown language names are build errors.** `{"*.inc": "hpp"}` fails
with the valid-name list. Silent skipping would recreate the exact hole
this change exists to close.

**D4 — Config changes ride the existing change detection.** Walk output is
a function of (tree, config); Detect diffs walk against stored state, so an
association add surfaces newly covered files as new, and a removal
surfaces them as deletions — no special-casing, and incremental==full
proves it.

**D5 — tsjs grammar routing for new/unknown extensions.** .mts/.cts →
typescript; .mjs/.cjs → javascript; association-routed unknown extensions →
typescript (parses the widest slice safely). PHP/Python/Go adapters are
extension-agnostic already.

**D6 — depmap generation loads the dep root's config** — a vendored Drupal
module tree needs its associations when mapped, same mechanism.

**D7 — Content sniffing for everything else.** "PHP can be .inc, .module,
anything" cannot be met by enumeration. Files the registry doesn't cover get
their first 1KB read at walk time: NUL byte → binary, skip; `<?php`/`<?=`
head (BOM/whitespace tolerated) → php; shebang containing php/python/
node|deno|bun → that language. Deterministic evidence only — no statistical
guessing; an HTML file or prose mentioning php stays unindexed.

**D8 — Persistent sniff cache, negatives included.** Verdicts live in a
sniffcache table keyed by size+mtime (additive table, no schema bump —
CREATE IF NOT EXISTS on open). Without cached negatives every freshness walk
would re-read every unknown file; with them the steady-state cost is one
stat per unknown file (kubernetes: +13ms on query p50, measured). Sniffed
routes are installed per run (SetExactRoutes replaces; stale cross-run
globals proved to break walk/parse agreement — caught by the equivalence
gate and reset at sniffer creation).

**D9 — Extension stops filtering saves.** The engine decides relevance now;
the extension triggers its debounced refresh on any file save. A refresh
that finds nothing is milliseconds.

## Risks / Trade-offs

- **Pattern cost per file** → few rules, basename path.Match is cheap;
  measured by the six-repo gate (no configs → zero rules → unchanged).
- **Global association state vs concurrent roots** → serialized today
  (query.mu); revisit if the MCP server ever goes multi-root parallel.
- **`.inc` in non-Drupal repos is C/asm sometimes** → not a built-in
  default; content evidence (or explicit config) decides.
- **Sniff false positives** → head evidence is strict (tag/shebang at start);
  kubernetes gate showed zero new files claimed across ~13k unknowns.
- **Walk overhead** → one stat per unknown file per walk (cache hits);
  measured +13ms on kubernetes query p50, within budget.

## Migration Plan

Additive; no schema bump. Broadened defaults mean previously ignored
.phtml/.mjs/.pyi files index on the next patch (appearing as new files —
exactly D4's path).

## Open Questions

None.
