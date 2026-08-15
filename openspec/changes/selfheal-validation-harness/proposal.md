# Proposal: selfheal-validation-harness

## Why

The runtime-evidence stack works in unit tests and one hand-built e2e; what
it needs before the WordPress/Drupal gate (and before field distribution) is
rigorous, repeatable validation across languages and dispatch patterns —
including PHP, which this machine can't run natively. A containerized
scenario lab plus a self-healing runner turns validation from a manual
session activity into an asset: failures auto-remediate through a recorded
ladder, and what worked is learned into configuration for the next run.
Separately, the curated question sets are author-invented; real closed
issues with fix commits are naturally-occurring concept queries with ground
truth — a query corpus nobody had to write.

## What Changes

- **Scenario lab (`bench/selfheal/`)**: generated fixture apps per runtime
  exercising the dispatch patterns static analysis misses (string-keyed
  registries, hook tables, DI wiring): Go + Node run locally with the SDKs;
  PHP runs in a Docker container with Excimer installed at image build, an
  excimer→cxprof adapter, and the spool mounted back to the host.
- **Self-healing runner (`bench/selfheal/harness.py`)**: runs the scenario
  matrix end-to-end (app → profile → spool → index → ingest → search),
  asserts (cxprof conformance, frame-resolution floor, observed-edge
  presence, cluster join, disclosure), and on failure walks a remediation
  ladder (longer sampling window → symlink-resolved repo root → container
  path re-root → quarantine). Outcomes append to `learned.json`; the
  successful remediation for a scenario is applied FIRST on later runs —
  the self-learning loop.
- **Issue corpus (`bench/selfheal/issues_corpus.py`)**: mine the pinned
  clones' git logs for fix-commits referencing issues, fetch issue titles
  (GitHub REST, unauthenticated budget), derive fixtures (query = issue
  title, accept = symbols whose spans the fix touched), and run them
  through the existing curated harness as a new `issues` class — closed
  issues are scored (ground truth known); open issues run unscored for
  miss-pattern collection only.
- Findings recorded per house rules in
  `bench/engine/FINDINGS-selfheal-validation.md`.

## Capabilities

### New Capabilities

- `validation-lab`: containerized/local scenario matrix, assertions,
  remediation ladder, learned-configuration loop.
- `issue-corpus`: fix-commit mining, issue-title fixtures, scored
  closed-issue class + unscored open-issue collection.

### Modified Capabilities

<!-- none: validation infrastructure only; no shipped-behavior requirements
     change. Anything the lab exposes lands as its own change. -->

## Impact

- New: `bench/selfheal/` (harness, generators, Dockerfiles, adapters),
  Docker images built locally (php+excimer), GitHub REST access for issue
  titles (rate-limit-aware, unauthenticated).
- No shipped-binary changes expected; discovered bugs get fixed under this
  change only when they block the lab, otherwise filed to the backlog.
- Subagent-built in parallel (user-directed): PHP lab, issue miner, harness
  core — integrated and run by the main session.
