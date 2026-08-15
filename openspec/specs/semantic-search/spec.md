# semantic-search Specification

## Purpose
TBD - created by archiving change semantic-code-search. Update Purpose after archive.
## Requirements
### Requirement: Symbol cards with graph context
The system SHALL build, for every tier-0 symbol, an embedding card containing
its tokenized qualified name, kind, signature, doc comment, file-path
segments, and the names of up to 8 top callers and callees by usage, and
SHALL embed cards at build time.

#### Scenario: Neighborhood carries the concept
- **WHEN** a symbol's own name does not mention a concept but its callers'
  names do
- **THEN** the symbol's card contains those caller names so a query for the
  concept can retrieve it

### Requirement: Hybrid concept search
The system SHALL answer `codeindex search <repo> "<query>"` by fusing a
lexical lane (the existing find ladder over the query plus optional hint
tokens) and a vector lane (query embedding vs symbol-card vectors) with
reciprocal-rank fusion, then re-ranking by graph signals (caller count, tier,
kind, test penalty).

#### Scenario: Concept query returns entry points
- **WHEN** the user searches a concept phrase that matches no symbol name
  literally (e.g. "onboarding lifecycle")
- **THEN** the result contains symbols semantically related to the concept,
  ranked with usage-heavy entry points first, each as path:line + signature

#### Scenario: Exact name still wins
- **WHEN** the query is an exact symbol name that exists in the index
- **THEN** that symbol ranks first regardless of vector-lane ordering

#### Scenario: Client hints sharpen retrieval
- **WHEN** the caller supplies hint tokens alongside the query
- **THEN** the lexical lane also matches on the hint tokens and fusion
  reflects them

### Requirement: Feature-map output
The system SHALL cluster top fused results by call-graph connectivity and
present each cluster with its highest-caller-count member as the entry point
and remaining members as path:line + signature references; a flat ranked list
SHALL remain available via flag/JSON.

#### Scenario: Clustered default output
- **WHEN** a concept query's top results span two unconnected call-graph
  regions
- **THEN** the output shows two clusters, each led by its entry point, rather
  than one interleaved list

### Requirement: Lexical-only degradation is disclosed
The system SHALL answer from the lexical lane alone when the embedder fails
or vectors are absent for the active model, and SHALL state the degradation
in the first line of output.

#### Scenario: Missing vectors
- **WHEN** `search` runs on a repo with no embedding table for the active
  model
- **THEN** results come from the lexical lane and the output's first line
  notes that semantic matching was unavailable

### Requirement: MCP search tool with routing law
The MCP server SHALL expose a `search` tool taking `query`, optional `hints`,
and optional `limit`, whose description states the routing law: concept or
feature questions → `search`; known symbol → `impact`/`callers`; distinctive
exact name → plain text search. The `search` tool SHALL NOT be added to the
always-visible prompt note.

#### Scenario: Concept query via MCP
- **WHEN** an MCP client calls `search` with a concept phrase
- **THEN** the tool returns the feature-map text, including the cold-build
  disclosure when applicable

### Requirement: Editor-agnostic explore-feature workflow
The system SHALL ship the explore-feature flow (search → choose entry point →
impact) as an MCP prompt and as a documented drop-in rules snippet for
non-Claude clients; the Claude Code plugin SHALL wrap the same flow as a
skill.

#### Scenario: Workflow available without Claude Code
- **WHEN** an MCP client that supports prompts connects to the server
- **THEN** it lists an `explore-feature` prompt describing the
  search-then-impact flow

### Requirement: Concept-shaped find misses route to search
The `find` command's zero-result hint SHALL route multi-token queries with no
symbol-name match toward `search` instead of suggesting content grep.

#### Scenario: Concept phrase hits find
- **WHEN** `find` returns zero results for a multi-token query
- **THEN** the empty-result message recommends `search` for concept or feature
  queries

### Requirement: Pre-registered concept recall gate
The recall bench SHALL grow a concept-class query set with pre-registered
known-answer entry points across the bench repos, with bars registered before
measurement: concept hit@5 ≥ 60%, no regression of existing vague and
distinctive classes, and full-build embedding overhead within the registered
budget.

#### Scenario: Gate decides the bundled model
- **WHEN** candidate bundled models are benched on the concept class
- **THEN** the shipped default is the smallest candidate meeting all
  registered bars

