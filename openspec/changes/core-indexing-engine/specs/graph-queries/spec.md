## ADDED Requirements

### Requirement: Callers and callees queries

The system SHALL answer, for a given symbol, both which symbols call it
(callers) and which symbols it calls (callees), using the `calls` edges in the
graph.

#### Scenario: Listing callers

- **WHEN** the user requests callers of a symbol
- **THEN** the system returns each calling symbol as a reference
  (`path:line  signature`)
- **AND** flags any result whose edge resolution confidence is ambiguous

#### Scenario: Listing callees

- **WHEN** the user requests callees of a symbol
- **THEN** the system returns each called symbol as a reference

#### Scenario: Ambiguous symbol name in the request

- **WHEN** the requested symbol name matches multiple definitions
- **THEN** the system lists the matching definitions so the user can
  disambiguate

### Requirement: Definition and signature lookup

The system SHALL answer where a symbol is defined and return its signature
without emitting the full source file.

#### Scenario: Definition found

- **WHEN** the user looks up a symbol definition
- **THEN** the system returns the `path:line` and the symbol's signature
- **AND** does not print the whole file

#### Scenario: Optional bounded context

- **WHEN** the user passes `--context N`
- **THEN** the system includes at most N source lines around the definition

#### Scenario: Definition not found

- **WHEN** the requested symbol is not defined in the index
- **THEN** the system reports that no definition was found

### Requirement: Dependencies and dependents queries

The system SHALL answer, for a given symbol or module, what it depends on
(dependencies) and what depends on it (dependents), using `imports`, `extends`,
`implements`, and `references` edges.

#### Scenario: Listing dependencies

- **WHEN** the user requests dependencies of a class or module
- **THEN** the system returns the symbols/modules it imports, extends,
  implements, or references, as references

#### Scenario: Listing dependents (blast radius)

- **WHEN** the user requests dependents of a symbol
- **THEN** the system returns the symbols that depend on it, as references
- **SO THAT** the user can assess the impact of changing it

### Requirement: Symbol search and outline

The system SHALL support fuzzy searching for symbols by name and producing a
compact outline of a file or module.

#### Scenario: Fuzzy symbol search

- **WHEN** the user searches for a partial or fuzzy name
- **THEN** the system returns matching symbols as references, ranked by match
  quality

#### Scenario: File outline

- **WHEN** the user requests the outline of a file
- **THEN** the system returns all symbols in that file with their kinds and
  signatures, as a compact map
- **AND** does not emit the full source

### Requirement: Reference-based output contract

The system SHALL default to compact reference output and SHALL provide a
structured JSON output mode, so that consumers receive `path:line + signature`
references rather than full-file dumps.

#### Scenario: Default compact output

- **WHEN** any query command runs without `--json`
- **THEN** results are printed as `path:line  signature` lines

#### Scenario: JSON output

- **WHEN** any query command runs with `--json`
- **THEN** results are emitted as structured records including path, line,
  signature, edge kind, and resolution confidence
