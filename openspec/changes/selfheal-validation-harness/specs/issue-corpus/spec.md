# issue-corpus Specification (delta)

## ADDED Requirements

### Requirement: Fix-commit mining produces ground-truth fixtures
The miner SHALL derive question fixtures from pinned clones: fix commits
referencing issues are mined from local git history, changed line ranges
map to symbols via index span overlap, and the issue title becomes the
query with the mapped symbols as the any-of-N accept set. Titles containing
a mapped symbol's name SHALL be excluded (locate-class). Fixtures SHALL use
the curated format (repo pin, provenance, split "issues-closed").

#### Scenario: Closed issue becomes a scored question
- **WHEN** a mined fix commit maps to at least one live symbol and its
  issue title is concept-shaped
- **THEN** the fixture contains that question and the existing curated
  harness scores it against the pinned index

### Requirement: Bounded, cached network use
Issue-title fetching SHALL respect a hard request budget, cache responses
on disk, back off on rate-limit responses, and degrade to a smaller corpus
rather than failing the run.

#### Scenario: Rate limit hit mid-mining
- **WHEN** the API rate-limits before the budget is spent
- **THEN** mining stops fetching, emits the fixture from what it has, and
  reports the funnel counts honestly
