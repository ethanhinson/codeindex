## ADDED Requirements

### Requirement: Benchmark baseline and tiers

The system SHALL define performance targets against a fixed baseline machine and
three repository tiers, and all numeric targets in this capability SHALL be
interpreted relative to that baseline.

The baseline machine is: 8 performance CPU cores, NVMe SSD, warm OS file cache.
The repository tiers are:

| Tier   | Reference size | Reference files |
| ------ | -------------- | --------------- |
| Small  | ~50k LOC       | ~500 files      |
| Medium | ~500k LOC      | ~5,000 files    |
| Large  | ~5M LOC        | ~50,000 files   |

#### Scenario: Reported numbers cite the baseline

- **WHEN** benchmark results are recorded
- **THEN** each result cites the baseline machine and the tier it was measured
  against
- **AND** results measured on other hardware are reported as relative ratios,
  not compared to absolute targets

### Requirement: Cold build throughput

The system SHALL complete a full index-from-scratch within the per-tier budget
on the baseline machine, and build time SHALL scale approximately linearly with
repository size within a tier.

| Tier   | Cold build budget |
| ------ | ----------------- |
| Small  | ≤ 3 s             |
| Medium | ≤ 30 s            |
| Large  | ≤ 5 min           |

#### Scenario: Cold build within budget

- **WHEN** `codeindex build` runs against a reference repository for a tier with
  no existing index
- **THEN** it completes within that tier's cold build budget on the baseline
  machine

#### Scenario: Build parallelism

- **WHEN** the cold build runs with the worker pool sized to the baseline's 8
  cores versus a single worker
- **THEN** the parallel build achieves at least 0.7 parallel efficiency
  (at least a 5.6× speedup at 8 cores)

### Requirement: Incremental update latency

The system SHALL bound incremental update work by the number of changed files,
not by repository size, so that a small edit is cheap regardless of tier.

| Tier   | Single-file edit re-check + patch |
| ------ | --------------------------------- |
| Small  | ≤ 150 ms                          |
| Medium | ≤ 300 ms                          |
| Large  | ≤ 750 ms                          |

#### Scenario: Single-file edit

- **WHEN** one source file is edited and an incremental update is triggered
- **THEN** the Merkle re-check, re-parse of the changed file, and graph patch
  complete within the tier's single-file budget on the baseline machine

#### Scenario: Work scales with change set, not repo size

- **WHEN** the same number of files is edited in a medium and in a large
  reference repository
- **THEN** the re-parse and patch time (excluding the change-detection walk) is
  within a small constant factor between the two tiers
- **AND** does not grow proportionally to total repository size

#### Scenario: Hot-symbol edits stay bounded

- **WHEN** an edit changes a defined symbol name that is referenced widely (the
  re-index spike measured up to ~4000 inbound references for the hottest 10–13%
  of symbols; the median symbol has only 2–7)
- **THEN** the re-parse cost is still that of the single changed file
- **AND** the additional edge re-resolution cost is proportional to the number of
  references to the changed name(s), performed via indexed name lookups
- **AND** the single-file edit budget is interpreted for the common case (median
  change-set of ~1 file editing a non-hot symbol); hot-symbol edits are permitted
  a re-resolution cost proportional to their reference count

### Requirement: Query latency including lazy re-check

The system SHALL answer queries against an unchanged repository within the
per-tier latency budget, where the budget includes the lazy Merkle re-check
overhead plus the indexed graph lookup.

| Tier   | Query p95 (unchanged repo) |
| ------ | -------------------------- |
| Small  | ≤ 75 ms                    |
| Medium | ≤ 150 ms                   |
| Large  | ≤ 400 ms                   |

Basis: the re-index spike (`bench/reindex_bench.py`, `bench/FINDINGS.md`)
measured, on kubernetes (~26k files, 231 MB), a stat-only walk of ~185 ms and a
full-content-hash walk of ~980 ms. Full hashing on every query would exceed the
large-tier budget, so the mechanisms below are required, not optional.

#### Scenario: Query on an unchanged repository

- **WHEN** a query command runs against a repository with no changes since the
  last index
- **THEN** end-to-end latency (lazy re-check plus graph lookup) is at or below
  the tier's p95 budget on the baseline machine
- **AND** the lazy re-check performs no graph writes

#### Scenario: Re-check overhead is the dominant, bounded cost

- **WHEN** a query runs on an unchanged repository
- **THEN** the graph lookup itself completes in ≤ 20 ms via indexed SQLite
  access
- **AND** the change-detection walk SHALL use the size+mtime fast path (never
  content-hashing unchanged files) and directory-level shortcutting so the walk
  does not stat every file in the repository
- **AND** vendored and generated trees are excluded from the walk

#### Scenario: Full-tree content hashing is not on the query path

- **WHEN** a query runs on an unchanged large-tier repository
- **THEN** the re-check SHALL NOT content-hash the whole tree (which measured
  ~980 ms on kubernetes and would exceed the 400 ms budget)

### Requirement: Token savings versus grep-and-read

The system SHALL answer navigation questions using substantially fewer tokens
than a naive grep-and-read approach, measured over a fixed question set. The
≥10× median target applies to the definition, callers/callees, and
dependencies/dependents query types. The outline query type has a lower target
(≥ 5× median) because a file's full symbol list is inherently larger; savings
also scale with source file size, so the target is validated on the reference
corpora, not on atypically small-file repositories.

Basis: a pre-implementation validation spike (`bench/`, `bench/FINDINGS.md`)
measured 100–500× median savings for definition and callers on large-file Go
repositories (including kubernetes at the large tier) and ~9–12× on a
small-file TypeScript repository, confirming the target and its file-size
dependence.

#### Scenario: Token reduction for core query types

- **WHEN** the benchmark runs the fixed navigation question set for the
  definition, callers/callees, and dependencies/dependents query types against a
  reference repository
- **THEN** the median tokens in the `codeindex` answers are at least 10× fewer
  than the tokens of the source files a grep-and-read strategy would load to
  answer the same questions
- **AND** a typical definition or callers answer is ≤ 500 tokens

#### Scenario: Outline savings

- **WHEN** the benchmark runs outline questions against a reference repository
- **THEN** the median outline answer uses at least 5× fewer tokens than reading
  the whole file
- **AND** outline answers for very large files are bounded by `--limit`

#### Scenario: Structured-output token premium is bounded

- **WHEN** the same answer is produced in compact text and in `--json` form
- **THEN** the JSON form uses no more than about twice the tokens of the text
  form
- **AND** compact text is the default output

### Requirement: Index size and build memory bounds

The system SHALL keep the on-disk index small relative to source and SHALL bound
peak memory during a build so it does not load the entire repository into
memory.

#### Scenario: Index size bound

- **WHEN** an index is built for a reference repository
- **THEN** the resulting `.codeindex/graph.db` is at most 25% of the total
  source byte size for that repository

#### Scenario: Build memory bound

- **WHEN** a cold build runs against the large-tier reference repository
- **THEN** peak resident memory stays at or below 1 GB
- **AND** the build streams files rather than holding all file contents in
  memory simultaneously

### Requirement: Benchmark harness and CI regression guard

The system SHALL provide a reproducible benchmark harness that measures every
targeted dimension and a CI job that records baselines and fails on regression.

#### Scenario: Reproducible benchmark run

- **WHEN** the benchmark harness runs against the reference corpora
- **THEN** it reports cold build time, build parallel efficiency, incremental
  update latency, query p50/p95, token-savings ratio, index size, and peak
  build memory per tier
- **AND** the run is repeatable from a documented command

#### Scenario: Regression guard in CI

- **WHEN** a benchmark metric regresses more than 20% versus the recorded
  baseline
- **THEN** the CI benchmark job fails
- **AND** identifies which metric and tier regressed
