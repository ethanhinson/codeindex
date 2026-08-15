# literal-lane Specification (delta)

## ADDED Requirements

### Requirement: Literal evidence lane in hybrid retrieval
Hybrid search SHALL run a third lane: distinctive content words of the
query (and of `error_text` when supplied) matched against file content via
grep attribution, with attributed tier-0 symbols joining rank fusion as a
ranked lane. The lane SHALL always run when distinctive words exist; its
influence SHALL be weighted deterministically from its own result
statistics (co-occurrence up, hit dispersion down, quote-shaped input up)
— never by an upfront query classifier.

#### Scenario: Symptom query lands on the erring symbol
- **WHEN** the query quotes or paraphrases an error message that exists as
  a string literal inside a symbol's span
- **THEN** that symbol enters the fused ranking via the literal lane even
  when name and card semantics carry no signal

#### Scenario: Generic words self-attenuate
- **WHEN** the query's distinctive words each hit thousands of content
  locations
- **THEN** the lane's fusion weight decays so concept-query rankings are
  not distorted (curated non-regression is the gate)

### Requirement: Verbatim-phrase precedence rung
The system SHALL pin a tier-0 symbol whose span contains a verbatim
(case-insensitive) occurrence of a quoted query phrase, the full query of
≥3 content words, or `error_text` directly below exact-name precedence and
above all fused scores, capped to the top 3 phrase-matched symbols.

#### Scenario: Pasted error message
- **WHEN** the caller supplies the program's exact warning string
- **THEN** the symbol containing that string ranks above every
  semantically-similar result, below only an exact symbol-name match

### Requirement: error_text input on the MCP search tool
The MCP `search` tool SHALL accept an optional `error_text` argument
carrying stack traces or quoted error output; supplying it SHALL grant the
literal lane maximum authority, and the tool description SHALL instruct
agents to pass symptom text through it.

#### Scenario: Agent holds a stack trace
- **WHEN** an MCP client calls search with `error_text` set
- **THEN** the literal lane runs with quote-shaped weighting and the
  verbatim rung applies to the supplied text
