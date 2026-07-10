## ADDED Requirements

### Requirement: v3 pre-registered gate on the packaged plugin

The change SHALL be validated by an A/B run (harness `--tag v3`) whose arm B
uses the real plugin via `--plugin-dir`, against a mixed task set, with
thresholds recorded in the task-file header before the run. The change is not
complete until the gate passes.

#### Scenario: Mixed task set

- **WHEN** the v3 task set is generated
- **THEN** it contains locate-style tasks (v1-shaped, where codeindex SHOULD
  NOT fire), branch-out tasks (v2-shaped caller attribution, where it SHOULD),
  and edit-flavored tasks (modify a symbol, then report what is affected),
  with ground truth arm-neutral as in v1/v2

#### Scenario: Pre-registered thresholds

- **WHEN** the gate is evaluated
- **THEN** it passes only if: locate tasks regress ≤10% in median paired cost
  versus arm A, branch-out tasks retain ≥50% median paired savings, and the
  hook fired on ≥80% of symbol edits and on 0 non-symbol edits in the
  edit-flavored tasks

#### Scenario: Trigger precision and recall reported

- **WHEN** the v3 report is generated
- **THEN** it reports, from transcripts: codeindex usage rate on locate tasks
  (mis-trigger rate) and on branch-out tasks (adoption), alongside the standard
  paired cost/success/turns metrics

#### Scenario: One registered iteration on YELLOW

- **WHEN** the gate lands between pass and fail (e.g. locate regression 10–20%
  or branch-out savings 30–50%)
- **THEN** one revision of skill/hook wording is permitted and the run repeated
  once, with both results reported
