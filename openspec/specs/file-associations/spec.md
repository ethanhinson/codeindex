# file-associations Specification

## Purpose
TBD - created by archiving change file-type-associations. Update Purpose after archive.
## Requirements
### Requirement: Association config

The engine SHALL load `.codeindex.json` from the repo root on every build
and patch, honoring `associations` glob patterns (basename match; repo-
relative path match when the pattern contains `/`) that route files to a
named language adapter, with associations taking precedence over built-in
extensions, and SHALL fail loudly on unknown language names.

#### Scenario: Drupal module routes to PHP

- **WHEN** `.codeindex.json` maps `*.module` to `php` and `foo.module`
  contains PHP source
- **THEN** its symbols are indexed and calls resolve across `.php` and
  `.module` files identically

#### Scenario: Typo fails loudly

- **WHEN** a pattern maps to an unknown language name
- **THEN** the build fails naming the invalid entry and the valid languages

### Requirement: Config changes are incremental changes

Adding or removing associations SHALL flow through change detection —
newly covered files index as additions, uncovered files drop as deletions —
and incremental==full SHALL hold across config edits.

#### Scenario: Association added after build

- **WHEN** a repo is indexed, then `.codeindex.json` gains `*.module: php`
- **THEN** the next patch indexes the .module files and equals a full
  rebuild

### Requirement: Broadened built-in defaults

PHP SHALL cover `.php` and `.phtml`; TS/JS SHALL cover `.ts`, `.tsx`,
`.js`, `.jsx`, `.mjs`, `.cjs`, `.mts`, `.cts` with per-extension grammar
routing; Python SHALL cover `.py` and `.pyi`.

#### Scenario: Defaults parse

- **WHEN** files with each new default extension are indexed
- **THEN** each yields symbols via the correct grammar

### Requirement: Extension honors associations

The VS Code extension SHALL treat files matching the workspace's
association patterns as supported for keep-warm refreshes.

#### Scenario: Saving an associated file

- **WHEN** `.codeindex.json` maps `*.module` to `php` and a .module file is
  saved
- **THEN** a keep-warm refresh triggers

### Requirement: Content-based detection

Files not covered by extensions or associations SHALL be content-sniffed at
walk time using deterministic head evidence (PHP open tags; php/python/node
interpreter shebangs; binary files excluded), with verdicts — including
negatives — cached by size+mtime so unchanged files are never re-read, and
sniffed routes SHALL be installed per run.

#### Scenario: Zero-config Drupal repo

- **WHEN** a repo containing `*.module`, `*.inc`, and extensionless PHP/
  Python scripts is indexed with no `.codeindex.json`
- **THEN** all of them index under the right language and calls resolve
  across them, while prose and sh scripts stay unindexed

#### Scenario: Sniff transition is incremental

- **WHEN** a previously non-PHP `.inc` file gains a `<?php` head
- **THEN** the next patch indexes it identically to a full rebuild

