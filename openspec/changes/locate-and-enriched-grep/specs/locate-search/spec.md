## ADDED Requirements

### Requirement: Ranked symbol search under partial knowledge

The system SHALL provide `codeindex find <repo> <query>` matching symbols via
convention-aware tokenization (camelCase, snake_case, acronym runs — one
shared splitter), deterministic synonym/stem expansion, and subsequence-fuzzy
matching, ranked by match quality combined with graph signals (caller count,
tier, kind, prod-over-test), with deterministic ties and bounded reference
output.

#### Scenario: Convention-independent token match

- **WHEN** the user runs `find "config load"`
- **THEN** `LoadConfig`, `load_config`, and `ConfigLoader` all match,
  regardless of token order or naming convention

#### Scenario: Synonym expansion

- **WHEN** the query says `fetch user` and the code defines `getUser`
- **THEN** the symbol matches via the static synonym table at reduced weight,
  and the ranking remains deterministic

#### Scenario: Graph-signal ranking

- **WHEN** multiple symbols match a query
- **THEN** heavily-called project symbols rank above rarely-called, dep-tier,
  and test-file matches, and exact/prefix matches rank above fuzzy ones

### Requirement: Enriched grep with symbol attribution

The system SHALL provide `codeindex grep <repo> <pattern>` that searches file
contents (ripgrep when available, internal scan otherwise), attributes every
hit to its enclosing symbol via the index, marks definition-line hits, dedups
hits by symbol with counts, ranks definitions first and production files
before tests, and reports the raw-hits→symbols compression.

#### Scenario: Attributed, deduped output

- **WHEN** a pattern occurs 20 times inside one function and once at a
  definition site
- **THEN** the output shows the definition entry first and one entry for the
  function with its hit count — not 21 raw lines

#### Scenario: Fallback without ripgrep

- **WHEN** `rg` is not on PATH
- **THEN** the internal scanner produces the same attributed output (slower,
  noted in output)

### Requirement: Locate routing across surfaces

The plugin note and MCP tool descriptions SHALL route locate work: distinctive
full names to plain grep, partial/vague names to `find`, and
occurrence-understanding to `codeindex grep`, keeping the trust instruction
unchanged.

#### Scenario: Routing text

- **WHEN** the prompt note is injected or MCP tools are listed
- **THEN** the three-way routing is stated, and plain grep remains the stated
  choice for distinctive full names

### Requirement: Pre-registered two-level validation

The change SHALL be validated by (1) a deterministic offline recall benchmark
— generated vague queries (token drop, reorder, synonym swap, case fold) over
sampled kubernetes and laravel symbols with a pre-registered bar of hit@5 ≥
70% on vague classes, failure of which is the sole trigger for exploring an
optional embeddings tier — and (2) an agent A/B (v6) whose classes and
thresholds are registered before running: distinctive-name regression ≤ 10%,
vague-partial savings ≥ 30%, attribute-occurrences savings ≥ 30%, success
delta ≥ −5pp.

#### Scenario: Recall benchmark gates the matcher

- **WHEN** the recall benchmark runs with a fixed seed
- **THEN** it reports hit@1/hit@5 per query class per repo, and the vague-class
  hit@5 result is compared against the 70% bar

#### Scenario: v6 gate protects grep's turf

- **WHEN** the v6 report is generated
- **THEN** the distinctive-name class shows ≤ 10% cost regression versus the
  control arm, or the change iterates its routing wording once (registered)
  and re-runs
