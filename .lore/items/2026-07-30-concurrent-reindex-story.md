---
id: itm-01KYTCQDP48M7SX769JYP9GNF3
title: Define the concurrent-reindex story (races across CLI/MCP/hook writers)
status: open
date: 2026-07-30
priority: p2
tags: [concurrency]
anchors:
    - path: internal/lore/index/
---
Found by Plan 3 reviews (AddSignals two-query TOCTOU; last_scanned_commit
double-scan window). Today reindex runs in one goroutine per process, but
three processes can race: a long-lived MCP server, CLI commands, and
Stop-hook captures. WAL + busy_timeout serialize writes, yet cross-process
interleavings can double-count signals or double-apply scans (closes is
idempotent; AddSignals is not).

Fix direction: single-SQL-statement increments for AddSignals; a
transactional claim on last_scanned_commit advancement (compare-and-swap on
the meta row); or an advisory lock file for the signals pass. Decide when
parallel-agent draining (claim/lease item) lands — same milestone.
