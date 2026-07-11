# index-progress Specification

## Purpose
TBD - created by archiving change index-progress-ux. Update Purpose after archive.
## Requirements
### Requirement: Engine progress events

Build and patch SHALL emit phase-tagged progress events (walk, parse, write,
resolve) with done/total counts through an attachable reporter, with the
reporter-less path byte-identical in behavior and measured throughput.

#### Scenario: Events are monotonic and complete

- **WHEN** a build runs with a recording reporter
- **THEN** events per phase have non-decreasing done, done==total at phase
  end, and a terminal summary is emitted

### Requirement: Status sidecar and verb

The engine SHALL maintain a best-effort status.json beside the index during
builds (state, phase, done, total, started_at; terminal: files, symbols,
indexed_at), and `codeindex status <root> --json` SHALL report
unindexed/building/fresh state plus schema version, counts, and index size
WITHOUT triggering any indexing.

#### Scenario: Detection is side-effect-free

- **WHEN** `codeindex status` runs on an unindexed repo
- **THEN** it reports unindexed and creates no .codeindex artifacts

### Requirement: Human and machine progress output

Build/export/import SHALL render live progress (spinner, bar, rate, ETA) on
TTY stderr, throttled plain lines otherwise, and with `--progress` SHALL
emit versioned JSON-lines events on stdout.

#### Scenario: JSONL feed

- **WHEN** `codeindex build <root> --progress` runs
- **THEN** stdout carries one JSON object per line with v, phase, done,
  total, and a final done-phase event

### Requirement: First-query freshness annotation

Query surfaces SHALL disclose an implicit cold build: CLI queries note it on
stderr; MCP tool results begin with one line stating files indexed and
duration when the call triggered a full build.

#### Scenario: MCP cold build is disclosed

- **WHEN** an MCP tool call triggers a first-time index build
- **THEN** the tool result's first line reports the build (files, duration)
  and subsequent calls carry no such line

