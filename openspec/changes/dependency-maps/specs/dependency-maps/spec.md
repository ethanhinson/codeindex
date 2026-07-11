## ADDED Requirements

### Requirement: Versioned symbols-only depmap artifacts

The system SHALL generate a dependency map from a source directory:
definitions only (name, parent, signature, spans) with per-file content hashes
and namespace + version metadata, storing no call or dependency edges.

#### Scenario: Generating a map

- **WHEN** `codeindex depmap <dir> --namespace github.com/sirupsen/logrus
  --version v1.9.3 -o logrus.db` runs
- **THEN** the artifact contains the dep's symbols with parents and
  signatures, per-file content hashes, and the namespace/version metadata
- **AND** contains no edges

#### Scenario: Auto-generation from vendor metadata

- **WHEN** `codeindex attach <repo> --auto` runs on a repo with
  `vendor/modules.txt` (Go) or `composer.lock` + `vendor/` (PHP)
- **THEN** one map per top-level module is generated (or reused from the
  per-`namespace@version` cache) and attached

### Requirement: Tiered resolution with project priority

Attached dep symbols SHALL act as resolution targets only, with deterministic
priority: qualified project match > qualified dep match > plain project match
> plain dep match. Dep symbols SHALL never source edges.

#### Scenario: Call into a dep resolves

- **WHEN** project code calls `WithFields` defined only in an attached logrus
  map
- **THEN** the edge resolves to the dep symbol with its signature and location
  available to queries

#### Scenario: Project always beats dep on collision

- **WHEN** a name is defined both in project code and in an attached map
- **THEN** resolution selects the project symbol, and the dep candidate does
  not degrade the confidence of the project match

#### Scenario: Dep symbols cannot be callers

- **WHEN** caller-attribution queries run on any symbol
- **THEN** no attached-map symbol ever appears as a caller (dep rows have no
  outgoing edges by construction)

### Requirement: Hash-verified overlay for locally modified deps

Attached maps SHALL record per-file hashes; covered in-tree files SHALL join
change detection (under a documented file-count threshold, else verified at
attach/build), and a modified file SHALL be re-parsed locally to shadow the
map's rows for that file until its content is restored.

#### Scenario: Hacking a vendored dep

- **WHEN** a developer edits `vendor/github.com/x/y/file.go` (adds a function
  or changes a signature) and runs any query
- **THEN** the changed file's symbols reflect the edit (marked modified) and
  `impact`/`callers`/`callees` answers include the local reality

#### Scenario: Restoring the file

- **WHEN** the file's content returns to the mapped hash (e.g. `git checkout`)
- **THEN** the file's symbols return to map-equivalent content

#### Scenario: Above-threshold trees are verified on demand

- **WHEN** the covered-file count exceeds the threshold (node_modules scale)
- **THEN** per-query re-check skips those trees and verification happens at
  attach/build time, with the behavior documented in output/README

### Requirement: Provenance and measured improvement

Dep-resolved targets SHALL display `[dep <namespace>@<version>]` (plus
`modified` when overlaid), and the change SHALL record the unresolved-call
share before/after on kubernetes (Go vendor) and laravel (composer), plus a
scripted hacked-dep round-trip test, with all existing engine gates re-run.

#### Scenario: Provenance in impact

- **WHEN** `impact` lists a callee resolved into an attached map
- **THEN** the line carries the `[dep ns@ver]` marker

#### Scenario: Recorded metric

- **WHEN** maps are attached to kubernetes and laravel
- **THEN** the findings note reports unresolved-call share before/after
  against the recorded baseline (19.6% and 27.5% respectively)
