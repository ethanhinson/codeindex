## ADDED Requirements

### Requirement: Symbol graph data model

The system SHALL persist a symbol graph in SQLite consisting of files, symbols,
and edges, with indexes supporting fast traversal in both directions.

#### Scenario: Symbol records

- **WHEN** a source file is indexed
- **THEN** each defined function, method, class, and type is stored as a symbol
  with its name, kind, signature, start line, end line, owning file, and
  optional parent symbol

#### Scenario: Edge records and kinds

- **WHEN** a relationship is discovered between symbols
- **THEN** it is stored as an edge with a source symbol, a destination, an edge
  kind, and a resolution confidence
- **AND** the edge kind is one of `calls`, `imports`, `extends`, `implements`,
  or `references`

#### Scenario: Both-direction traversal is indexed

- **WHEN** the graph is stored
- **THEN** edges are indexed by both source and destination
- **AND** symbols are indexed by name
- **SO THAT** "callers of X" and "callees of X" are both answerable without a
  full scan

### Requirement: Name-based resolution with confidence

The system SHALL resolve edges by symbol name for this change and SHALL record a
`resolved_confidence` on each edge indicating whether the target is unambiguous
or ambiguous, without hiding ambiguous matches.

#### Scenario: Unambiguous target

- **WHEN** a referenced name matches exactly one defined symbol
- **THEN** the edge points at that symbol
- **AND** its resolution confidence is marked unambiguous

#### Scenario: Ambiguous target

- **WHEN** a referenced name matches more than one defined symbol
- **THEN** the system records the candidate relationship(s)
- **AND** marks the resolution confidence as ambiguous
- **AND** does not present the match as certain

#### Scenario: Unresolved reference

- **WHEN** a referenced name matches no defined symbol in the index
- **THEN** the reference is retained as an unresolved edge with the name
  preserved
- **AND** it does not fabricate a target symbol

### Requirement: Derived, regenerable index

The system SHALL treat `.codeindex/graph.db` as a derived artifact that can be
deleted and regenerated at any time from source, and SHALL not require it to be
committed to version control.

#### Scenario: Deleting the index

- **WHEN** `.codeindex/graph.db` is deleted
- **THEN** the next command rebuilds it from the current source with no loss of
  correctness
