# validation-lab Specification (delta)

## ADDED Requirements

### Requirement: Multi-runtime scenario matrix
The lab SHALL validate the runtime-evidence pipeline end-to-end (app →
profile → spool → index → ingest → search) per scenario, covering
string-keyed dispatch in Node and Go locally via the SDKs and in PHP via a
Docker image with Excimer installed at image build. A scenario whose
prerequisites are absent SHALL skip with an explicit "skipped" outcome,
never a silent pass.

#### Scenario: Hook dispatch validated per runtime
- **WHEN** a scenario app dispatches through a string-keyed registry and is
  profiled by its runtime's collector
- **THEN** the pipeline produces observed edges bridging dispatcher and
  handler, search disclose the evidence, and the assertion suite passes

### Requirement: Self-healing remediation ladder with memory
On assertion failure the harness SHALL apply remediations in the registered
order (extend sampling window → symlink-resolved paths → index rebuild →
quarantine), re-running only the failed step; the remediation that fixed a
scenario SHALL be recorded and applied proactively on later runs. Every
attempt SHALL be appended to a runs journal; quarantine SHALL be terminal
and visible.

#### Scenario: Learned remediation applied first
- **WHEN** a scenario previously succeeded only after a specific remediation
- **THEN** the next run applies that remediation before the first attempt
  and records that it did so

#### Scenario: Persistent failure surfaces
- **WHEN** the full ladder fails for a non-optional scenario
- **THEN** the spool is quarantined, the run exits non-zero, and the journal
  shows every attempt
