# Backlog

**4 changes** — 🟡 1 proposed · 🔵 1 implemented · ✅ 2 done

## 🟡 Proposed (1)

| # | Title | Priority | Type | Readiness |
|---|-------|----------|------|-----------|
| [0004](active/0004-cleanup-delete-lore-rewrite-readme.md) | Cleanup — delete .lore/, drop lore config, rewrite README | `medium` | `chore` | ⏳ waiting on #3 — needs your merge |

## 🔵 Implemented — awaiting merge (1)

| # | Title | Priority | Type | PR | Readiness |
|---|-------|----------|------|----|-----------|
| [0003](active/0003-decouple-graph-query-layer.md) | Decouple the symbol-graph query layer (headless JSON API + CLI) | `high` | `refactor` | [#5](https://github.com/ethanhinson/codeindex/pull/5) |  |

```mermaid
graph TD
  0002 --> 0003
  0001 --> 0004
  0003 --> 0004
  0001:::done
  0002:::done
  classDef done fill:#d3f9d8;
```

<details><summary>✅ Archive — done (2)</summary>

| # | Title | Merged |
|---|-------|--------|
| [0002](archive/2026-08-04-0002-rip-out-lore-product-surface.md) | Rip out the lore product surface (engine, CLI, MCP, plugin skills) | 2026-08-04 |
| [0001](archive/2026-08-04-0001-migrate-keeper-lore-decisions-to-adrs.md) | Migrate keeper lore decisions to docket ADRs | 2026-08-04 |

</details>
