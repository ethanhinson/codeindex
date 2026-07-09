## ADDED Requirements

### Requirement: Go parsing to symbols and call edges

The system SHALL parse Go source with tree-sitter and extract function, method,
and type definitions as symbols and function/method call sites as name-based call
edges.

#### Scenario: Extracting symbols from a Go file

- **WHEN** the skeleton indexes a Go source file
- **THEN** each function, method, and type definition is recorded as a symbol
  with its name, kind, signature, and line span

#### Scenario: Extracting call edges

- **WHEN** a function or method call appears in a Go file
- **THEN** a `calls` edge is recorded from the enclosing symbol to the called
  name, resolved name-based with a resolution confidence
- **AND** a call whose name matches no known definition is retained as an
  unresolved edge

### Requirement: SQLite persistence on the target schema

The system SHALL persist symbols, call edges, files, and Merkle state to SQLite
using the target schema subset (`files`, `symbols`, `edges`, `merkle`) so the
full engine extends rather than migrates it.

#### Scenario: Persisting a build

- **WHEN** `codeindex build` completes on a Go repository
- **THEN** the `files`, `symbols`, `edges`, and `merkle` tables are populated in
  `.codeindex/graph.db`
- **AND** the schema matches the columns the full engine expects (unused columns
  may be null but present)

### Requirement: File-level Merkle with mandatory fast path

The system SHALL hash file contents into Merkle leaves and SHALL treat a file as
unchanged when its size and mtime match the stored values, without content
hashing.

#### Scenario: Detecting a changed file

- **WHEN** a Go file is modified and the change detector runs
- **THEN** the file is identified as changed via a size/mtime mismatch followed by
  a content-hash comparison
- **AND** files whose size and mtime are unchanged are skipped without hashing

### Requirement: Incremental patch of only changed files

The system SHALL, on an incremental update, re-parse only the changed files and
replace their symbols and call edges within a single transaction, leaving
unchanged files' graph data intact.

#### Scenario: Incremental update after a single-file edit

- **WHEN** one Go file is edited and the incremental path runs
- **THEN** only that file is re-parsed
- **AND** its symbols and edges are replaced in one transaction
- **AND** other files' rows are untouched

### Requirement: Benchmark of throughput and incremental correctness

The system SHALL provide a benchmark that measures cold build throughput and
single-file incremental patch latency, and that proves an incremental update
produces a graph equal to a full rebuild.

#### Scenario: Throughput measurement

- **WHEN** the benchmark runs `build` against a Go reference repository
- **THEN** it reports files-per-second and lines-per-second for the cold build
- **AND** reports the latency of a single-file incremental patch
- **AND** records the baseline machine with the results

#### Scenario: Incremental equals full rebuild

- **WHEN** the benchmark builds an index, edits a file, applies the incremental
  patch, and separately performs a full rebuild of the edited tree into a scratch
  database
- **THEN** the normalized symbol and edge sets of the incremental graph and the
  full-rebuild graph are equal
- **AND** the benchmark fails if they differ, reporting the difference
