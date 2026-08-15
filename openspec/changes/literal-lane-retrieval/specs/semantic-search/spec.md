# semantic-search Specification (delta)

> Applies against the `semantic-search` spec as modified by
> `runtime-evidence-stack` (which itself follows
> `diffusion-contrast-retrieval`) — archive in that order first.

## MODIFIED Requirements

### Requirement: Hybrid concept search
The system SHALL answer `codeindex search <repo> "<query>"` by fusing a
lexical lane (the existing find ladder over the query plus optional hint
tokens), a vector lane (query embedding vs symbol-card vectors), and a
literal lane (grep-attributed content evidence per the literal-lane
capability) with reciprocal-rank fusion, then diffusing seed scores over
the union of the static symbol graph and observed runtime edges, ranking
by the blended fused + diffused score. Precedence follows the exactness
ladder: exact symbol name first, verbatim-phrase content match second,
fused+diffused scores below. Results whose ranking materially depends on
observed-only evidence SHALL disclose its provenance and age.

#### Scenario: Concept query returns entry points
- **WHEN** the user searches a concept phrase that matches no symbol name
  literally (e.g. "onboarding lifecycle")
- **THEN** the result contains symbols semantically related to the concept,
  ranked with graph-central, usage-relevant symbols first, each as
  path:line + signature

#### Scenario: Symptom query wins via literal evidence
- **WHEN** the query paraphrases an error message that exists as a string
  literal inside a symbol's span
- **THEN** the literal lane carries that symbol into the top results even
  when name and card semantics carry no signal

#### Scenario: Hook-wired code becomes reachable
- **WHEN** the only connection between a concept's symbols is string-keyed
  dispatch invisible to static analysis, and ingested runtime evidence
  observed it
- **THEN** diffusion flows across the observed edge and the callback ranks
  with its feature, with the observed provenance disclosed

#### Scenario: Exact name still wins
- **WHEN** the query is an exact symbol name that exists in the index
- **THEN** that symbol ranks first regardless of vector-lane, literal-lane,
  diffusion, or observed-evidence ordering

#### Scenario: Client hints sharpen retrieval
- **WHEN** the caller supplies hint tokens alongside the query
- **THEN** the lexical lane also matches on the hint tokens and fusion
  reflects them
