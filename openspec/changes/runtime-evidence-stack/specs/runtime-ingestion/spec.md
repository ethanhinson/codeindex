# runtime-ingestion Specification (delta)

## ADDED Requirements

### Requirement: Spool discovery on the freshness pass
The system SHALL treat unseen `*.cxprof.jsonl` files in
`.codeindex/runtime/` like changed source files: the fresh-on-query pass
ingests them, records them in a ledger, and leaves the files in place.

#### Scenario: Dev loop with zero processes
- **WHEN** an SDK drops a spool file and any query runs
- **THEN** the query answers with the new runtime evidence already
  ingested, with no daemon or manual step involved

### Requirement: Frame resolution by symbol span
Ingestion SHALL resolve each frame (file, line) to the enclosing tier-0
symbol by span lookup; unresolvable frames SHALL collapse so an observed
edge spans the nearest resolvable frames, flagged as indirect. Ingestion
SHALL report its frame-resolution rate.

#### Scenario: Hook dispatch produces an edge
- **WHEN** a sampled stack runs framework code between two project symbols
  (e.g. WordPress `do_action` between caller and hook callback)
- **THEN** an observed edge links the two project symbols, flagged
  indirect, weighted by sample count

### Requirement: Observed evidence is additive and provenance-stamped
Observed edges and heat SHALL be stored separately from static edges,
stamped with source file, time span, and commit when present. Observed
evidence SHALL never remove or override static conclusions; absence of
samples SHALL NOT be treated as evidence of death.

#### Scenario: Stale profile disclosed
- **WHEN** ranking uses observed evidence older than the staleness horizon
  or from a different commit
- **THEN** the output marks it (e.g. `[observed 12d ago]`) instead of
  silently trusting it
