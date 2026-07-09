## ADDED Requirements

### Requirement: Initial index build

The system SHALL provide a `codeindex build` command that walks the repository,
parses every supported source file, resolves relationships, and persists the
resulting symbol graph and Merkle content tree to `.codeindex/graph.db`.

#### Scenario: Building an index from scratch

- **WHEN** `codeindex build` runs in a repository with no existing index
- **THEN** the system walks the repository honoring `.gitignore` and built-in
  ignore defaults
- **AND** parses each TypeScript/JavaScript and Go file via its language adapter
- **AND** writes symbols, edges, and per-file Merkle hashes to
  `.codeindex/graph.db`
- **AND** reports the number of files indexed and symbols discovered

#### Scenario: Unsupported files are skipped

- **WHEN** the repository contains files whose extension has no registered
  language adapter
- **THEN** the system skips those files without error
- **AND** does not create symbols or edges for them

#### Scenario: Rebuilding over an existing index

- **WHEN** `codeindex build` runs and `.codeindex/graph.db` already exists
- **THEN** the system produces an index equivalent to a fresh build for the
  current working-tree state

### Requirement: Language adapter interface

The system SHALL define a pluggable language-adapter interface, registered by
file extension, that isolates per-language parsing and edge extraction so that
adding a language does not modify other adapters.

#### Scenario: Adapter registration by extension

- **WHEN** the indexer encounters a file during the walk
- **THEN** it selects the adapter registered for that file's extension
- **AND** uses `.ts`, `.tsx`, `.js`, `.jsx` for the TypeScript/JavaScript
  adapter and `.go` for the Go adapter

#### Scenario: Adapter parses symbols and raw edges

- **WHEN** an adapter parses a source file
- **THEN** it returns the symbols defined in that file (functions, methods,
  classes, types) with their kind, signature, and line span
- **AND** returns the raw references (calls, imports, extends, implements) found
  in that file for the resolver to link

### Requirement: Merkle-based incremental update

The system SHALL maintain a Merkle content tree whose leaves hash file contents
and whose interior nodes hash their children, and SHALL use it to detect exactly
which files changed since the last index so that only those files are re-parsed.

#### Scenario: Detecting changed files

- **WHEN** the current Merkle root differs from the stored root
- **THEN** the system walks only the subtrees whose hashes changed
- **AND** identifies the exact set of added, modified, and deleted files

#### Scenario: Patching only affected graph data

- **WHEN** a set of changed files is identified
- **THEN** the system re-parses only those files
- **AND** replaces their symbols and edges within a single transaction
- **AND** removes symbols and edges belonging to deleted files
- **AND** leaves unchanged files' graph data untouched

#### Scenario: Re-resolving inbound edges only when defined names change

- **WHEN** a changed file's set of defined symbol names is unchanged (e.g. a
  function body was edited without adding, removing, or renaming a symbol)
- **THEN** the system does not re-resolve edges that resolve into that file
- **WHEN** a changed file adds, removes, or renames a defined symbol name
- **THEN** the system re-resolves edges that reference the affected name(s)
- **AND** that re-resolution uses indexed name lookups whose cost is proportional
  to the number of references to the affected names, not to repository size

#### Scenario: Unchanged-file fast path (required)

- **WHEN** a file's size and mtime match the stored values
- **THEN** the system SHALL treat it as unchanged without content hashing
- **AND** SHALL fall back to content hashing only when size or mtime differs

#### Scenario: Directory-level change-detection shortcutting

- **WHEN** the change-detection walk runs on a large repository
- **THEN** the system SHALL descend only into directories whose recorded state
  (e.g. directory mtime or stored subtree hash) indicates a possible change
- **AND** SHALL skip directories excluded by ignore rules, including vendored and
  generated trees (e.g. `vendor/`, `node_modules/`, build output), so the walk
  does not stat every file on every query
- **AND** the excluded/ignored trees are reported so coverage is not silently
  reduced

### Requirement: Lazy freshness on query

The system SHALL, before answering any query, perform a Merkle re-check and
apply incremental updates so that answers always reflect the current working
tree, including uncommitted edits, without a background process.

#### Scenario: Query after an edit

- **WHEN** a source file is edited and then a query command is run
- **THEN** the system detects the changed file via the Merkle re-check
- **AND** re-parses and patches it before producing the answer
- **AND** the answer reflects the edited content

#### Scenario: Query with no changes

- **WHEN** a query command is run and no files have changed since the last index
- **THEN** the Merkle re-check makes no graph modifications
- **AND** the query proceeds with minimal overhead

#### Scenario: Query before any build

- **WHEN** a query command is run and `.codeindex/graph.db` does not exist
- **THEN** the system builds the index first, then answers the query
