# Architecture Decision Records

Immutable, numbered record of *why*. ADRs are never archived or rewritten; once `Accepted`, only the `status:` line changes (on supersession/reversal). This index is generated — do not hand-edit.

## Active

- [ADR-0001](0001-parsing-via-tree-sitter-with-edge-resolver.md) — Parsing via tree-sitter with our own edge resolver (Accepted) ← change #1
- [ADR-0002](0002-storage-sqlite-graph-db-transactional-incremental.md) — Storage is SQLite (.codeindex/graph.db), transactional incremental updates (Accepted) ← change #1
- [ADR-0003](0003-engine-language-go-single-static-binary.md) — Engine implementation language is Go (single static binary) (Accepted) ← change #1
- [ADR-0004](0004-config-driven-index-include-exclude-defaults.md) — Config-driven index include/exclude with built-in vendor defaults (Accepted) ← change #1
- [ADR-0005](0005-freshness-on-demand-build-lazy-per-query-recheck.md) — Freshness is on-demand build + lazy per-query re-check, no daemon (Accepted) ← change #1
- [ADR-0006](0006-change-detection-flat-per-file-hashes-not-merkle-tree.md) — Change detection uses flat per-file content hashes, not a Merkle tree (Accepted) ← change #1
- [ADR-0007](0007-output-contract-references-only-not-source.md) — Output contract: references only (path:line + signature), never source (Accepted) ← change #1
- [ADR-0008](0008-docket-replaces-lore.md) — Docket replaces lore (openspec → lore → docket lineage) (Accepted) ← change #1

## Superseded / Reversed

_None._

## Deprecated

_None._
