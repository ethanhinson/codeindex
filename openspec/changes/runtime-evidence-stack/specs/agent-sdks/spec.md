# agent-sdks Specification (delta)

## ADDED Requirements

### Requirement: In-process sampling SDKs emitting cxprof
The project SHALL provide thin in-process sampling SDKs — Go and Node in
stage 1, Python in stage 2, PHP (Excimer adapter) in stage 3 — that wrap
each runtime's native sampler and emit cxprof v1, requiring no root, no
kernel facilities, and no daemon in dev mode.

#### Scenario: One-step dev onboarding
- **WHEN** a developer adds the SDK package and sets its enable env var
- **THEN** running their app produces spool files under
  `.codeindex/runtime/` with no other configuration

### Requirement: Non-disruption guarantees
Every SDK SHALL sample (never instrument calls), hard-cap its buffers
(dropping whole profiles on overflow), write/send asynchronously and
fire-and-forget, swallow its own failures (counter, not exception), honor
`CODEINDEX_PROFILING=off`, and emit frames-only payloads.

#### Scenario: Spool directory unavailable
- **WHEN** the spool target is unwritable or the collector is down
- **THEN** the host application observes no error and no blocking; the SDK
  drops data and counts the drop

#### Scenario: Kill switch
- **WHEN** `CODEINDEX_PROFILING=off` is set
- **THEN** the SDK performs no sampling and allocates no buffers
