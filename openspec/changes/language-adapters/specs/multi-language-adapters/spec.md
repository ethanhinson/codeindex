## ADDED Requirements

### Requirement: TypeScript/JavaScript adapter

The system SHALL parse `.ts`, `.tsx`, `.js`, and `.jsx` files with the
appropriate tree-sitter grammar and extract named functions, classes, methods,
and named arrow-function bindings as symbols, plus call sites as name-based
call edges attributed to the innermost enclosing symbol.

#### Scenario: TS symbols and calls

- **WHEN** the adapter parses a TypeScript file containing a function, a class
  with a method, and a `const f = () => ...` binding
- **THEN** it returns symbols for the function (func), class (type), method
  (method), and arrow binding (func), each with name, signature, and line span
- **AND** a call `obj.method(x)` inside the function produces a raw call edge
  from the function to the name `method`

#### Scenario: Grammar selection by extension

- **WHEN** files with `.ts`, `.tsx`, and `.js` extensions are parsed
- **THEN** each uses the matching grammar (typescript, tsx, javascript) and
  anonymous callbacks are not emitted as symbols

### Requirement: Python adapter

The system SHALL parse `.py` files and extract module-level functions, classes,
and methods (functions lexically inside a class) as symbols, plus call sites as
name-based call edges.

#### Scenario: Python symbols and calls

- **WHEN** the adapter parses a file with a class containing a method and a
  module-level function that calls `self.save()` equivalent or `helper()`
- **THEN** the class is a type symbol, the method a method symbol, the function
  a func symbol
- **AND** calls produce edges to the final name (`save`, `helper`), attributed
  to their enclosing symbol

### Requirement: PHP adapter

The system SHALL parse `.php` files and extract functions, methods, and
class/interface/trait declarations as symbols, plus function, member, and
scoped call sites as name-based call edges.

#### Scenario: PHP symbols and calls

- **WHEN** the adapter parses a class with a method calling `$this->save()`
  and a function calling `Foo::bar()`
- **THEN** class → type, method → method, function → func symbols are emitted
- **AND** call edges target the final names (`save`, `bar`)

### Requirement: Registry-driven repository walk

The system SHALL derive the set of indexable file extensions from the adapter
registry, so the walk, line counting, and freshness detection cover exactly
the registered languages without per-language changes.

#### Scenario: Polyglot repo walk

- **WHEN** a repository contains Go, TS, Python, and PHP files plus
  unsupported files
- **THEN** the walk indexes all four languages' files, skips unsupported ones
  without error, and continues to exclude vendored trees (`vendor/`,
  `node_modules/`)

#### Scenario: Incremental freshness across languages

- **WHEN** a Python or TS file is edited after an index exists
- **THEN** the next query detects and patches exactly that file (same
  fresh-on-query behavior proven for Go)

### Requirement: Per-language correctness proof on real repositories

The system SHALL pass the incremental==full-rebuild equivalence check on at
least one pinned real repository per new language, with results recorded.

#### Scenario: Bench proof per language

- **WHEN** `codeindex bench` runs against the pinned TS (nest), Python, and
  PHP reference repositories
- **THEN** the incremental-equals-full check passes for each
- **AND** cold-build and single-file-patch numbers are recorded in
  `bench/engine/`

### Requirement: Truthful consumption-surface updates

The plugin hook, prompt note, MCP tool descriptions, and READMEs SHALL reflect
the supported language set exactly, and any cited measured savings SHALL retain
their "measured on Go repositories" qualifier.

#### Scenario: Hook fires on new languages

- **WHEN** the agent edits a `.py`, `.ts`, or `.php` function with external
  callers
- **THEN** the post-edit hook injects the blast-radius note (same noise rules
  as Go)

#### Scenario: No overclaiming

- **WHEN** any surface cites the −62–73% measurements
- **THEN** the text attributes them to Go-repo experiments
