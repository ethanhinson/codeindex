## ADDED Requirements

### Requirement: Interned string storage

The index SHALL store repeated strings (symbol file/name/parent/namespace/
kind; edge src_file/dst_name/dst_qualifier/dst_ns/kind/confidence) exactly
once in an interning table, with symbols/edges holding integer references,
while `symbols` and `edges` remain queryable with their original TEXT columns.

#### Scenario: Read surface unchanged

- **WHEN** any existing query (resolution ladder, callers/callees/impact,
  depmap attach, search) runs against a v7 index
- **THEN** it returns results identical to v6, in identical order

### Requirement: Size budget met

A v7 index SHALL be smaller in absolute bytes than its v6 equivalent on every
pinned repo, and laravel-framework SHALL meet the ≤2× source-size budget by
the bench tool's ratio definition.

#### Scenario: Laravel budget

- **WHEN** `codeindex bench` runs on laravel-framework at schema v7
- **THEN** the reported index ratio is ≤ 2.0× source

### Requirement: Performance and correctness preserved

Cold build and query p50/p95 SHALL stay within existing budgets on all six
pinned repos, and incremental==full SHALL pass on all six; if view flattening
degrades the resolve ladder, resolve() switches to explicit two-step id
lookups (pre-registered fallback), re-gated.

#### Scenario: Equivalence at v7

- **WHEN** `codeindex bench` runs on the six pinned repos
- **THEN** incremental == full rebuild holds and budgets are met
