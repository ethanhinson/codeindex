## ADDED Requirements

### Requirement: Dependency-edge extraction per language

Every adapter SHALL emit import, extends, and implements edges where the
language expresses them lexically: Go import specs (verbatim paths) and struct
embedding; TS/JS named+default imports, class/interface extends, implements;
Python imports, from-imports, and class bases; PHP namespace `use` imports,
extends, implements, and in-class trait use.

#### Scenario: TS class heritage

- **WHEN** the adapter parses `class A extends B implements C {}`
- **THEN** it emits an extends edge A→B and an implements edge A→C, both
  originating from symbol A

#### Scenario: Imports are file-level edges

- **WHEN** a file contains `import {helper} from './utils'` (TS),
  `from utils import helper` (Python), `use App\Support\Helper;` (PHP), or
  `import "codeindex/internal/graph"` (Go)
- **THEN** an imports edge is recorded with no owning symbol (file-level
  source) and the target name (`helper`, `Helper`) or the verbatim Go path

#### Scenario: Go embedding

- **WHEN** a struct embeds another type (`type A struct { B }`)
- **THEN** an extends edge A→B is emitted

### Requirement: Typed-edge storage with file-level sources

The store SHALL persist dependency edges in the existing edges table using
kinds `imports`/`extends`/`implements`, with `src_symbol_id = 0` and `src_file`
set for file-level edges, resolving symbol-name targets through the existing
qualifier-aware resolver and leaving Go import paths unresolved verbatim.

#### Scenario: Resolvable extends target

- **WHEN** `class A extends B` and `B` is defined once in the repo
- **THEN** the extends edge resolves to B unambiguously

#### Scenario: Incremental equivalence with dependency edges

- **WHEN** files are patched incrementally versus fully rebuilt
- **THEN** normalized snapshots (including file-level-source edges) are equal
  on all six pinned repositories

#### Scenario: Version bump forces repopulation

- **WHEN** a binary with dependency edges opens an older index
- **THEN** the index rebuilds automatically so unchanged files also gain
  dependency edges

### Requirement: Dependents and deps queries

The system SHALL answer `dependents <anchor>` (who imports/extends/implements
the anchor) and `deps <anchor>` (what the anchor imports/extends/implements)
on the CLI and as MCP tools, with Go import paths matched exact or by last
path segment, and file-level sources displayed as their file path.

#### Scenario: Who extends a class

- **WHEN** the user runs `dependents <Class>` where subclasses exist
- **THEN** each subclass is returned as a reference with kind `extends`

#### Scenario: Who imports a Go package

- **WHEN** the user runs `dependents graph` or `dependents codeindex/internal/graph`
- **THEN** the files importing that package are returned (last-segment and
  exact matching respectively)

#### Scenario: Deps of a file and of a symbol

- **WHEN** `deps <file>` is run with an indexed file path
- **THEN** the file's imports are listed
- **WHEN** `deps <Class>` is run with a symbol anchor
- **THEN** its extends/implements targets are listed (plus its file's imports,
  labeled)

### Requirement: Impact composition updated truthfully

`/impact` SHALL include a dependents section (bounded, counts-first) and its
coverage statement SHALL name what is covered (call + import/extends/implements
edges) and what is not (type-usage references).

#### Scenario: Impact on a subclassed class

- **WHEN** `impact <Class>` runs where the class has callers and subclasses
- **THEN** counts lead with definitions, callers, callees, and dependents, and
  the dependents section lists the subclasses/importers
