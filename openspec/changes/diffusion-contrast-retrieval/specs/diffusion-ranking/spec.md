# diffusion-ranking Specification (delta)

## ADDED Requirements

### Requirement: Seed-conditioned score diffusion
The system SHALL, after hybrid lane fusion, propagate seed scores over the
symbol graph via bounded personalized PageRank: the subgraph SHALL be
induced from the top fused seeds plus their 2-hop neighborhood over call
edges (both directions) and extends/implements edges, with caps on total
nodes and per-node degree; restart mass SHALL be proportional to fused seed
scores; iteration count, damping, and blend weight SHALL be fixed constants,
making the computation deterministic for a given index state and query.

#### Scenario: Neighborhood carries relevance at query time
- **WHEN** a concept query's vector seeds land on symbols whose neighbors
  (callers/callees) are semantically central to the concept
- **THEN** those neighbors receive diffused mass and can enter the final
  top results even when their own card similarity is weak

#### Scenario: Determinism
- **WHEN** the same query runs twice against an unchanged index
- **THEN** ranking is identical

### Requirement: Diffusion-blended final ranking
The final score SHALL blend fused lane score and diffused mass with a fixed
blend constant frozen before held-out measurement; exact-name precedence
SHALL continue to outrank both.

#### Scenario: Exact name still wins
- **WHEN** the query is an exact symbol name present in the index
- **THEN** that symbol ranks first regardless of diffusion

#### Scenario: Hub containment
- **WHEN** a high-fan-in utility symbol (e.g. a logger) neighbors many seeds
  but shares no query semantics
- **THEN** degree caps and normalization prevent it from dominating the
  final top-5

### Requirement: Feature map consumes the diffusion subgraph
Feature-map clustering SHALL group over the diffusion subgraph's edges (not
only edges among returned results), and cluster entry selection SHALL
prefer diffused mass over raw caller count.

#### Scenario: Connective tissue joins a cluster
- **WHEN** two strong hits are connected only through an intermediate symbol
  that was not itself a seed
- **THEN** they cluster together, with the intermediate eligible as a member

### Requirement: Diffusion latency budget
Diffusion SHALL stay within the registered per-query latency budget on a
~30k-symbol repo; when the subgraph construction exceeds its caps, the
system SHALL degrade to fused-only ranking for that query rather than
exceed budget, without erroring.

#### Scenario: Oversized neighborhood degrades gracefully
- **WHEN** seed neighborhoods exceed the subgraph node cap
- **THEN** the query answers within budget using capped-subgraph or
  fused-only ranking, and results remain deterministic
