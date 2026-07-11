# shared-index Specification

## Purpose
TBD - created by archiving change shared-index-artifacts. Update Purpose after archive.
## Requirements
### Requirement: Index export

`codeindex export <out>` SHALL freshen the index for the current tree and
write a compact, consistent snapshot of it to `<out>`, self-describing via
its embedded schema version.

#### Scenario: Export freshens first

- **WHEN** files changed since the last index update and `codeindex export`
  runs
- **THEN** the artifact reflects the current tree, not the stale index

### Requirement: Index import with drift patching

`codeindex import <artifact>` SHALL reject an artifact whose schema version
differs from the binary's (naming both versions), otherwise install it and
patch it to the current tree, reporting files re-parsed and deleted.

#### Scenario: Schema mismatch is loud

- **WHEN** an artifact with a different user_version is imported
- **THEN** the command fails with both versions named and no index is
  installed

#### Scenario: mtime-only drift is free

- **WHEN** an artifact is imported into a fresh checkout of the same tree
  (all mtimes differ, content identical)
- **THEN** zero files are re-parsed

### Requirement: Cross-state equivalence

An artifact exported at tree state A and imported at state B SHALL, after
patching, be snapshot-identical to a from-scratch build at state B.

#### Scenario: Export/mutate/import equals rebuild

- **WHEN** the tree is mutated (file edited, added, and deleted) between
  export and import
- **THEN** the imported-and-patched index snapshot equals a full rebuild

### Requirement: CI recipe

The repository SHALL document a working CI workflow that builds and exports
the index once and lets developers import it.

#### Scenario: Recipe present

- **WHEN** a user reads docs/ci.md
- **THEN** it contains a complete GitHub Actions example and the import
  command developers run

