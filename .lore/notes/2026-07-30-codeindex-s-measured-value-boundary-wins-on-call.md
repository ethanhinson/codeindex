---
id: note-01KYTPPJQ8P164D382G97WWB99
title: 'codeindex''s measured value boundary: wins on call-graph, loses on locate-questions'
date: "2026-07-30"
related: [note-01KYTPDHJH59PG2M0FFTEYC92Z, dec-01KYTPMMBQ6V10EZF8V0BWB7WD]
tags: [engine, efficacy]
anchors:
    - path: bench/agent_ab/
    - path: plugin/
---

Durable finding from the agent A/B efficacy study (openspec change
agent-ab-efficacy, 2026-07-09), migrated on openspec/ retirement (2026-07-30).
Raw analysis stays in bench/agent_ab/FINDINGS.md and FINDINGS_v2.md.

v1 VERDICT RED: on "which FILES reference X" tasks (≈ `rg -l`, one grep call),
codeindex INCREASED cost ~17-26% at success parity — an additive round-trip on
questions grep already answers cheaply.

v2 VERDICT GREEN: redesigned around the real edge — "which FUNCTIONS call X,"
where grep gives locations but not caller names so the agent opens many files.
Median cost reduction 73% (95% CI 62-82%), 94% win rate, median turns 13 -> 2.

Defensible boundary (constrains product/plugin design):
- The query surface prioritizes callers/callees, dependents/blast-radius over
  "where is X."
- The plugin/skill must trigger codeindex on "who calls / what's affected," NOT
  on "where is X" — mis-triggering reproduces the v1 overhead. (The current
  UserPromptSubmit hook guidance reflects this: grep first for locate, codeindex
  for call-graph.)
- Precise resolution matters most for caller attribution on common/overloaded
  names — that is where the GREEN zone extends.
