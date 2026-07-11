# Dependency maps — findings

**Date:** 2026-07-10 · **Change:** `dependency-maps`

## What shipped

Versioned, symbols-only depmaps (`codeindex depmap` / `attach` / `attach
--auto`): per-`namespace@version` artifacts (definitions + per-file hashes, no
edges), cached globally (`~/.codeindex/depmaps/`), attached into repo indexes
with language-native namespacing. Tiered resolution ladder — qualified project
> qualified dep > plain project > plain dep — so maps can never degrade
project resolution and dep symbols can never appear as callers. Hash-verified
per-file overlay handles locally modified ("hacked") deps via the existing
size+mtime fast path on every query (≤25k covered files; above that,
attach/build-time verification, documented). Provenance markers:
`[dep ns@ver]` / `[dep ns@ver modified]`. Auto-discovery: Go
`vendor/modules.txt`, PHP `composer.lock`.

## Headline metric — kubernetes (Go vendor, 175 modules, 66,190 dep symbols)

| | Before | After |
| --- | --- | --- |
| Unresolved calls | **19.6%** (122,103) | **5.5%** (34,319) |
| Calls resolved into dep symbols | 0 | **87,784** |

Previously-unresolved names now split into dep-resolved (with signatures,
locations, provenance) and honestly-flagged dep-ambiguous.

## The hacked-dep round-trip (live on kubernetes klog)

1. Appended a function to `vendor/k8s.io/klog/v2/exit.go` → next query indexed
   it (`def HackedBugProbe vendor/.../exit.go:72`) and flipped provenance to
   `[dep k8s.io/klog/v2@v2.130.1 modified]`.
2. `git checkout -- <file>` → symbol gone, provenance clean. No hidden state.

Also covered by an end-to-end unit test (generate → attach → tier priority →
provenance → hack → restore).

## Performance

- Attach (cold cache, 175 maps): ~91 s first-ever; **cached re-attach 9.2 s**
  after batching re-resolution (one pass, not per-module — 10× fix).
- Query latency on kubernetes WITH maps + per-query overlay verification:
  ~120–170 ms wall including process start — within the 400 ms budget.
- Index size: 210 MB → 229 MB (+19 MB for 66k dep symbols). Global map cache:
  31 MB for all 175 modules (shared across every repo using them).
- Six-repo incremental==full: all pass (bench builds are map-free by design;
  dep-tier consistency has its own round-trip test).

## Honest limits / notes

- PHP metric not measurable on our laravel clone (framework SOURCE repo — no
  vendored deps/composer.lock committed). Composer discovery is implemented
  and the attach/overlay path is language-independent (unit-covered); a
  composer-installed application repo would exercise it end-to-end.
- Node/Python: manual `depmap` command only in v1 (lockfile auto-gen is
  mechanical follow-up).
- Stdlib calls remain unresolved (stdlib maps = future; it's "just another
  pinned dep").
- Nested Go modules overlap vendor trees — attach dedups per file,
  first-attached wins (fixed after genproto collision).
- Import-aware PROJECT resolution (per-file import sets) deliberately deferred
  to its own change.
