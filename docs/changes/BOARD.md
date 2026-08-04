# Backlog

**4 changes** — 🟡 2 proposed · 🔵 1 implemented · ✅ 1 done

## 🟡 Proposed (2)

| # | Title | Priority | Type | Readiness |
|---|-------|----------|------|-----------|
| [0003](active/0003-decouple-graph-query-layer.md) | Decouple the symbol-graph query layer (headless JSON API + CLI) | `high` | `refactor` | ⏳ waiting on #2 — needs your merge |
| [0004](active/0004-cleanup-delete-lore-rewrite-readme.md) | Cleanup — delete .lore/, drop lore config, rewrite README | `medium` | `chore` | ⏳ waiting on #3 — not yet built |

## 🔵 Implemented — awaiting merge (1)

| # | Title | Priority | Type | PR | Readiness |
|---|-------|----------|------|----|-----------|
| [0002](active/0002-rip-out-lore-product-surface.md) | Rip out the lore product surface (engine, CLI, MCP, plugin skills) | `high` | `refactor` | [#4](https://github.com/ethanhinson/codeindex/pull/4) |  |

```mermaid
graph TD
  0002
  0002 --> 0003
  0001 --> 0004
  0003 --> 0004
  0001:::done
  classDef done fill:#d3f9d8;
```

<details><summary>✅ Archive — done (1)</summary>

| # | Title | Merged |
|---|-------|--------|
| [0001](archive/2026-08-04-0001-migrate-keeper-lore-decisions-to-adrs.md) | Migrate keeper lore decisions to docket ADRs | 2026-08-04 |

</details>
