# Findings: robust file typing (associations + content detection)

Date: 2026-07-11 · Additive sniffcache table, no schema bump

## Why

Extension-only detection was wrong for real-world PHP (Drupal ships PHP as
.module/.inc/.install/.theme; templates as .phtml) and incomplete for TS/JS
(.mjs/.cjs/.mts/.cts) and Python (.pyi). "PHP can be anything" cannot be met
by enumeration — so detection is now three deterministic layers:

1. **Committed associations** (`.codeindex.json`, the VS Code
   files.associations model; basename or path globs; unknown language names
   fail the build loudly),
2. **built-in extensions** (broadened),
3. **content sniffing** for everything else: first 1KB — `<?php`/`<?=` head,
   php/python/node shebangs, binary guard. Zero config: a Drupal clone
   indexes .module/.inc/extensionless scripts out of the box.

Verdicts (negatives included) are cached in the index keyed by size+mtime;
unchanged files are never re-read. Sniffed routes install per run — a
cross-run global leaked routes between walk and parse and was caught by the
equivalence gate (fixed: reset at sniffer creation).

## Measured

- **kubernetes with sniffing live**: files=11005 — identical count, i.e.
  **zero false positives** across ~13k unknown-extension files; cold build
  82.6s (unchanged); query p50 123.7ms vs 110.7ms (+13ms = one stat per
  unknown file per freshness walk, cache hits, no reads) — within the 500ms
  budget; single-file patch 213ms; **inc==full true on all six repos**.
- Config changes ride change detection: association add/remove and
  a file *gaining* a `<?php` head all patch identically to full rebuilds
  (equivalence-proven).

## Tests

Sniff unit table (tags/BOM/shebangs/binary/prose/sh); zero-config
Drupal-shaped repo (module/inc/php-shebang/python-script indexed +
cross-boundary caller resolution; prose and notes NOT indexed);
association-overrides-sniff; unknown-language loud failure; broadened
defaults parse per extension; extension pure-logic suite 7/7.

## Surfaces

Engine + depmap generation (dep trees carry their own associations; overlay
re-parse sniffs odd-extension hacked dep files). VS Code extension now
triggers keep-warm on ANY save — the engine decides relevance; a no-op
refresh is milliseconds.
