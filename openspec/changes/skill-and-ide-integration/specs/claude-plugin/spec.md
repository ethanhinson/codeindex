## ADDED Requirements

### Requirement: Fresh-on-query engine behavior

Every query command SHALL apply the incremental update before answering and
SHALL build the index automatically when it does not exist, so answers always
reflect the current working tree. This covers `query`/`callers`, `callees`,
`enclosing`, and all future query commands.

#### Scenario: Query after an edit reflects the edit

- **WHEN** a source file is edited and a query command runs
- **THEN** the changed file is re-parsed and patched before the answer is
  produced
- **AND** the answer reflects the edited content

#### Scenario: Query on a repo with no index

- **WHEN** a query command runs and `.codeindex/graph.db` is missing
- **THEN** the index is built first, then the query is answered

### Requirement: Enclosing-symbol lookup

The engine SHALL provide `codeindex enclosing <repo> <file> <start>:<end>`
returning the symbols whose spans overlap the given line range, each with its
caller count and the count of callers outside the given file.

#### Scenario: Edited range maps to its enclosing function

- **WHEN** `enclosing` is called with a file and a line range inside a function
  body
- **THEN** it returns that function's name, kind, span, total caller count, and
  external-caller count (callers defined in other files)

#### Scenario: Range outside any symbol

- **WHEN** the range overlaps no symbol span
- **THEN** the command returns an empty result and exit code 0 (the hook treats
  this as "inject nothing")

### Requirement: Trigger-disciplined skill

The plugin SHALL ship a skill that instructs the agent to use codeindex only
when branching out from a known anchor, with explicit negative triggers for
locate-questions.

#### Scenario: Positive triggers

- **WHEN** the agent is about to modify, rename, or delete a named
  function/method/type, assess a diff's impact, trace callers while debugging,
  or check whether a symbol is dead code
- **THEN** the skill directs it to run `/impact <symbol>` (or the primitives)
  before editing

#### Scenario: Negative triggers are explicit

- **WHEN** the agent needs to locate a definition, find files mentioning a
  term, or explore unfamiliar code without a named anchor
- **THEN** the skill explicitly directs it to use Grep/Glob instead of
  codeindex, citing that this is measured to be cheaper

### Requirement: Impact workflow and primitive commands

The plugin SHALL provide `/impact <symbol>` composing callers + callees (and
dependents when the engine provides them) into a bounded, counts-first summary,
plus `/callers <symbol>` and `/callees <symbol>` primitives.

#### Scenario: Impact summary

- **WHEN** `/impact Foo` runs
- **THEN** the output leads with counts (definitions, callers, callees), then
  bounded reference lists (`--limit` honored), preserving `[ambiguous]` flags
- **AND** the summary states what edge kinds it covers so missing dependents
  are not implied to be absent

#### Scenario: Primitives pass through

- **WHEN** `/callers Foo` or `/callees Foo` runs
- **THEN** the corresponding CLI query output is returned without modification
  beyond the command's `--limit`

### Requirement: Post-edit blast-radius hook

The plugin SHALL ship a PostToolUse hook on Edit/Write of `.go` files that maps
the changed hunks to enclosing symbols and injects a compact blast-radius note,
subject to strict noise controls.

#### Scenario: Editing a symbol with external callers

- **WHEN** the agent edits a function that has at least one caller outside the
  edited file
- **THEN** the hook injects a note (≤150 tokens) naming the symbol, its caller
  count, top caller files, and suggesting `/impact <symbol>` for detail

#### Scenario: Noise controls

- **WHEN** the edited symbol has no external callers, or the same symbol was
  already reported this session, or the file is untracked (no git diff), or the
  edit is outside any symbol
- **THEN** the hook injects nothing
- **AND** a plugin setting can disable the hook entirely

#### Scenario: Hook failures are silent

- **WHEN** the hook script errors for any reason (missing binary, index build
  failure, timeout)
- **THEN** it exits without injecting and without blocking the agent's edit
