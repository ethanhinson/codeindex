## Why

Language detection is extension-only, and for PHP that's simply wrong in the
wild: Drupal ships PHP as `.module`/`.inc`/`.install`/`.theme`/`.profile`,
templates live in `.phtml`, CakePHP used `.ctp`. TS/JS misses `.mjs`/`.cjs`/
`.mts`/`.cts` and Python misses `.pyi` stubs. A repo full of un-indexed PHP
silently produces a graph with holes — the worst failure mode for a tool
whose pitch is trustworthy coverage.

## What Changes

- **Broadened built-in defaults**: PHP += `.phtml`; TS/JS += `.mjs`, `.cjs`,
  `.mts`, `.cts` (grammar routed per extension); Python += `.pyi`.
- **File type associations** (the VS Code `files.associations` model): a
  committed `.codeindex.json` at repo root maps glob patterns to languages —
  `{"associations": {"*.module": "php", "*.inc": "php"}}`. Patterns match
  basenames; patterns containing `/` match repo-relative paths. Associations
  take precedence over built-in extensions (explicit intent wins). Valid
  language names: `go`, `tsjs`, `python`, `php`; unknown names fail the
  build loudly (a typo must not silently skip files).
- Adapters gain a `Name()`; the walk defers to a single
  `adapter.Indexable(rel)` predicate; engine loads the config on every
  build/patch, so association changes are picked up like any other change —
  newly covered files appear as additions, uncovered ones as deletions,
  through the existing incremental machinery.
- **Content detection (zero-config)**: files no extension or association
  covers are sniffed by head — PHP open tags (`<?php`, `<?=`) and
  interpreter shebangs (php/python/node) — so a Drupal clone indexes
  `.module`/`.inc`/extensionless scripts out of the box. Verdicts (including
  negatives) are cached in the index keyed by size+mtime; unchanged files
  are never re-read. Precedence: associations > extensions > sniffed routes.
- **VS Code extension**: keep-warm triggers on ANY file save (the engine
  decides relevance — the extension cannot predict sniffed files); a no-op
  refresh is milliseconds.

**Validation (pre-registered)**: Drupal-shaped fixture (`*.module` PHP
routed, symbols + cross-file resolution); incremental==full under
association add/remove (config edits patch identically to rebuilds);
unknown-language config fails loudly; broadened-default parse tests per
extension; zero-config Drupal-shaped sniffing (module/inc/shebang scripts
indexed, prose and sh scripts not); sniff-transition equivalence (a file
GAINING a php head patches identically to a rebuild); association-overrides-
sniff; six-repo gate re-run with sniffing live (kubernetes: identical file
count — no false positives — and walk overhead measured and recorded);
extension pure-logic tests.

Non-goals: per-file language overrides in the IDE; remapping files *away*
from built-ins to "ignore" (use walk excludes later if needed); sniffing
beyond deterministic head evidence (no statistical classification).

## Capabilities

### New Capabilities
- `file-associations`: association config, precedence, loud validation,
  incremental behavior under config change, broadened defaults.

### Modified Capabilities
None at requirement level (language-adapters coverage widens; resolution
semantics untouched).

## Impact

- internal/adapter (Name, associations, Indexable), internal/config (new),
  internal/merkle (walk predicate), internal/engine (config load), adapters
  (Name + default extensions + tsjs grammar routing), editors/vscode
  (association-aware save filter).
- No schema change. Existing indexes unaffected until a config appears.
