# scoped-resolution Specification

## Purpose
TBD - created by archiving change import-aware-resolution. Update Purpose after archive.
## Requirements
### Requirement: Derived project namespaces

Project-tier symbols SHALL carry a derived namespace: Go = the file's
directory, Python = the dotted module path, TS/JS = the file path, PHP = the
declared `namespace` (directory fallback), populated at index time without
adapter parsing changes (except PHP's declaration capture).

#### Scenario: Namespace derivation

- **WHEN** `pkg/scheduler/queue.go` (Go), `a/b/c.py` (Python), and a PHP file
  declaring `namespace App\Support;` are indexed
- **THEN** their symbols carry namespaces `pkg/scheduler`, `a.b.c`, and
  `App\Support` respectively

### Requirement: Scope-preference resolution ladder

Resolution SHALL prefer, in order: qualified matches, import-bound matches,
same-scope matches (the language's local scope — file/module for Python and
TS/JS, package/namespace for Go and PHP), then plain project and dep-tier
matches — each step deterministic, with single-candidate steps resolving
unambiguous and multi-candidate steps flagging ambiguous.

#### Scenario: Same-package call collapses

- **WHEN** `pkg/scheduler/a.go` calls `validate()` and `validate` is defined
  once in `pkg/scheduler` and in 40 other packages
- **THEN** the edge resolves to the same-namespace definition, unambiguous
  (previously ambiguous across 41)

#### Scenario: Scope never makes results worse

- **WHEN** no same-file or same-namespace candidate exists
- **THEN** resolution proceeds exactly as before this change

### Requirement: Import-bound resolution (Stage 2)

Import edges SHALL persist their source (specifier/path/module) and Go
alias-selector calls their namespace hint, and resolution SHALL constrain a
called name that the calling file imports to the import's mapped namespace,
with total fallback down the ladder.

#### Scenario: Named import binds

- **WHEN** a TS file imports `{helper}` from `./utils` and calls `helper()`,
  with `helper` also defined in five unrelated files
- **THEN** the edge resolves to the `utils` module's definition, unambiguous

#### Scenario: Go alias-scoped call

- **WHEN** a file imports `k8s.io/kubernetes/pkg/util` and calls `util.Clock()`
- **THEN** candidates are constrained to symbols whose namespace matches the
  import path suffix (`pkg/util`)

### Requirement: Reproducible re-resolution under scope awareness

Re-resolution SHALL reproduce insert-time results exactly — grouping updates
per (name, qualifier, source file) where scope or binding context differs —
and the incremental==full equivalence check SHALL pass on all six pinned
repositories at each stage.

#### Scenario: Equivalence at each stage

- **WHEN** `codeindex bench` runs after Stage 1 and after Stage 2
- **THEN** incremental == full rebuild holds on gin, prometheus, kubernetes,
  nest, flask, and laravel

### Requirement: Staged ambiguity measurement

The change SHALL record ambiguous-call-edge counts per repo after each stage
against the recorded baselines (kubernetes 355,442; laravel 64,593; prometheus
25,647; nest 2,455; flask 1,065; gin 1,668), and full-bench query latency
SHALL remain within budget.

#### Scenario: Metric recorded per stage

- **WHEN** each stage completes
- **THEN** `bench/engine/FINDINGS-import-resolution.md` reports per-repo
  absolute and percentage reductions, and Stage 2's scope is justified against
  Stage 1's measured residue

