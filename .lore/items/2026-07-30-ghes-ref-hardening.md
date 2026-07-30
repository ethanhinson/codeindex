---
id: itm-01KYTCQDP4BXQ48RBJF229GJ6Q
title: Harden gh-issue refs for GitHub Enterprise hosts
status: open
date: 2026-07-30
priority: p3
tags: [sync]
anchors:
    - path: internal/lore/ghsync/
    - path: cmd/codeindex/lore.go
---
Found by the Plan 3 final review. `lore push` against a GHES instance stores
the raw issue URL (urlToRef only parses github.com), and one unparseable ref
makes a later `lore sync github` error out entirely — a single poisoned ref
blocks the whole sync.

Fix direction: make urlToRef host-agnostic (parse any <host>/<owner>/<repo>/
issues/<n> shape, keep host in the ref when non-github.com), or refuse to
write a ref that ParseRef cannot round-trip; and make sync skip-and-report
unparseable refs instead of aborting.
