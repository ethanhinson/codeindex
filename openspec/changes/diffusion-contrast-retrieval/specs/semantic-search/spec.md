# semantic-search Specification (delta)

> Applies against the `semantic-search` spec introduced by the
> `semantic-code-search` change — archive that change first.

## MODIFIED Requirements

### Requirement: Symbol cards with graph context
The system SHALL build, for every tier-0 symbol, an embedding card
containing its tokenized qualified name, kind, signature, doc context, and
file-path segments, contrast-weighted against the symbol's structural
family (symbols sharing a parent type, and symbols in the same directory
module): doc phrases whose family document-frequency is ≥ 0.8 in families
of ≥ 5 members SHALL be suppressed, and tokens unique to the symbol within
its family SHALL be emphasized in a distinct card field. Neighborhood
semantics (caller/callee context) SHALL be carried by query-time diffusion
rather than by embedding neighbor names into cards, subject to the
registered paired measurement; cards SHALL be embedded at build time.

#### Scenario: Boilerplate siblings are distinguishable
- **WHEN** many sibling symbols share near-identical doc text differing only
  in their distinctive tokens (e.g. a family of exception classes)
- **THEN** each sibling's card suppresses the shared boilerplate and carries
  its distinctive tokens, so a query naming the distinctive concept ranks
  the right sibling above its clones

#### Scenario: Contrast is deterministic and content-addressed
- **WHEN** neither a symbol nor its family changes between builds
- **THEN** its card text (and therefore its content hash and stored vector)
  is unchanged

### Requirement: Hybrid concept search
The system SHALL answer `codeindex search <repo> "<query>"` by fusing a
lexical lane (the existing find ladder over the query plus optional hint
tokens) and a vector lane (query embedding vs symbol-card vectors) with
reciprocal-rank fusion, then diffusing seed scores over the symbol graph
(per the diffusion-ranking capability) and ranking by the blended fused +
diffused score.

#### Scenario: Concept query returns entry points
- **WHEN** the user searches a concept phrase that matches no symbol name
  literally (e.g. "onboarding lifecycle")
- **THEN** the result contains symbols semantically related to the concept,
  ranked with graph-central, usage-relevant symbols first, each as
  path:line + signature

#### Scenario: Exact name still wins
- **WHEN** the query is an exact symbol name that exists in the index
- **THEN** that symbol ranks first regardless of vector-lane or diffusion
  ordering

#### Scenario: Client hints sharpen retrieval
- **WHEN** the caller supplies hint tokens alongside the query
- **THEN** the lexical lane also matches on the hint tokens and fusion
  reflects them

### Requirement: Feature-map output
The system SHALL cluster top results by call-graph connectivity over the
diffusion subgraph (results plus their connective neighborhood) and present
each cluster with an entry point selected by diffused mass, members as
path:line + signature references; a flat ranked list SHALL remain available
via flag/JSON.

#### Scenario: Clustered default output
- **WHEN** a concept query's top results span two unconnected call-graph
  regions
- **THEN** the output shows two clusters, each led by its entry point,
  rather than one interleaved list

#### Scenario: Connected-through-intermediate results share a cluster
- **WHEN** two results connect only via an intermediate symbol inside the
  diffusion subgraph
- **THEN** they appear in one cluster rather than two
