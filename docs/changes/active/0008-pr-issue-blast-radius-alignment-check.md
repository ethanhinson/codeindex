---
id: 8
slug: pr-issue-blast-radius-alignment-check
title: PR ↔ issue blast-radius alignment check
status: proposed
priority: medium
type: feat
created: 2026-08-04
updated: 2026-08-04
depends_on: []
related: []
discovered_from: []
adrs: []
spec:
plan:
results:
trivial: false
auto_groomable:
branch:
pr:
blocked_by:
reconciled: false
---

## Artifacts

<!-- docket:artifacts:start (generated — do not hand-edit) -->
<!-- docket:artifacts:end -->

## Why

codeindex knows the blast radius of a change. That is exactly the signal needed to answer a question CI cannot cheaply answer today: does this PR's diff actually touch the area the issue is about, or has it drifted? A PR that claims to fix issue #123 but whose changed symbols have no impact-overlap with the code the issue points at is a misalignment worth flagging — for human reviewers and for agents validating their own work.

Idea surfaced while mining the LLM-research vault: PAIChecker (uncovering PR–issue misalignment) frames this directly, and it reuses the CI export/import path codeindex already ships. This is a genuinely new product surface built entirely on the existing graph.

## What changes

A check that, given a PR's changed symbols and a target set of symbols/files (from the linked issue or supplied explicitly), computes whether the PR's blast radius intersects the target area, and reports the overlap:
- Runs against a prebuilt index (the CI export artifact), so it is cheap in a pipeline.
- Output is an alignment verdict plus the overlapping / non-overlapping symbols in the standard `file:line` contract.
- Usable both as a CI gate and as an agent self-check before opening a PR.

## Out of scope

- Natural-language parsing of the issue to *infer* the target set — the first cut takes an explicit target (files/symbols), NL inference is a later enhancement.
- Judging PR correctness — this measures topical overlap of impact, not whether the fix is right.

## Open questions

- Where does the "target area" come from — issue-linked file paths, a label convention, or a required PR field?
- Package it as a `codeindex` subcommand, a GitHub Action, or both?
- What overlap threshold constitutes "misaligned" vs. "narrow but valid"?

## Reconcile log

<!-- Appended by docket-implement-next's reconcile pass: dated entries of what changed. -->
