# concept-eval Specification (delta)

## ADDED Requirements

### Requirement: Curated any-of-N concept question sets
The bench SHALL support curated concept question sets as frozen fixtures:
per repo, a list of natural-language concept questions each with a set of
acceptable answer symbols (any-of-N scoring at hit@5). Fixtures SHALL be
authored from repo documentation (never from search output), SHALL record
their authorship provenance, and SHALL be frozen before the mechanisms they
gate are measured.

#### Scenario: Any-of-N scoring
- **WHEN** a curated question's top-5 contains any symbol from its
  acceptable-answer set
- **THEN** the question scores as a hit

#### Scenario: Fixture provenance
- **WHEN** a curated set is loaded
- **THEN** it carries repo identity (name, pinned commit) and authorship
  provenance, and the harness refuses sets whose repo pin does not match
  the indexed clone

### Requirement: Tuning/held-out split with one-shot held-out protocol
The harness SHALL distinguish tuning repos from held-out repos. Held-out
repos SHALL be evaluated exactly once per registered gate, after parameters
freeze; the harness SHALL record the run (parameters, binary identity,
results) so a repeated held-out run is detectable in findings.

#### Scenario: Held-out run is recorded
- **WHEN** a held-out evaluation runs
- **THEN** the output artifact records frozen parameters, binary/model
  identity, and per-question results

### Requirement: Measurability guard on the mechanical concept class
The mechanical doc-phrase class SHALL emit a case only when the
name-stripped residual query retains at least 2 informative content words
(non-stopword, length ≥ 3), SHALL report the discard rate per repo, and
SHALL be reported as a valid measurement only where the guard admits at
least half of the candidate cases.

#### Scenario: Terse-doc corpus is flagged unmeasurable
- **WHEN** the guard discards more than half of a repo's candidate cases
- **THEN** the harness reports the mechanical class as not valid for that
  repo instead of reporting a misleading percentage
