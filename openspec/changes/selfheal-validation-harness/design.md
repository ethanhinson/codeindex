# Design: selfheal-validation-harness

## Context

Runtime-evidence stages 1–3 shipped with unit tests and one hand-built e2e.
Before the WordPress/Drupal gate and field distribution, validation needs to
be repeatable, multi-runtime (including PHP, which only exists here in
containers), and cheap to re-run after every pipeline change. The session
that built the stack also demonstrated the failure modes worth automating
around: too-short sampling windows, path aliasing (macOS /tmp symlink),
stale ledgers from operator sequencing errors.

## Goals / Non-Goals

**Goals:**
- Scenario matrix covering the dispatch patterns static analysis misses,
  per runtime (Go/Node local via SDKs; PHP via Docker + Excimer).
- Self-healing: failures walk a remediation ladder; what worked is recorded
  and applied first next time (learned configuration, not guesswork).
- A naturally-occurring query corpus: closed issues + fix commits as
  concept questions with ground truth; open issues collected unscored for
  miss patterns.

**Non-Goals:**
- Changing shipped ranking/ingestion behavior (bugs found get filed or
  fixed under their own justification).
- CI integration (local runner first; CI is a follow-up once green).
- Authenticated GitHub scraping (unauthenticated budget, cached).

## Decisions

- **D1: Container paths solved at the emitter.** The PHP adapter emits
  repo-relative frames (stripping the /app mount prefix) — cxprof already
  allows relative paths, so container↔host path translation never reaches
  the ingester. Host-side symlink resolution (already shipped) covers the
  rest.
- **D2: Remediation ladder is ordered by diagnosis cost**, not likelihood:
  longer window → resolved paths → index rebuild → quarantine. Quarantine
  is terminal and visible; silent retry-forever is banned.
- **D3: Learning = replay, not inference.** learned.json maps scenario →
  remediations that historically fixed it, applied proactively next run.
  No statistics, no thresholds tuning themselves — the "algorithm" is a
  recorded ladder with memory, deterministic and auditable.
- **D4: Issue mining is local-first.** Fix commits and diffs come from
  deepened local clones (git log/show); the network is only for issue
  titles, budgeted and cached. Titles containing a mapped symbol name are
  excluded (locate-class, not concept-class).
- **D5: Subagent construction.** Three parallel builders (PHP lab, issue
  miner, harness core) with the harness treating the PHP lab as an
  optional, skip-clean scenario — no cross-agent file contention.

## Risks / Trade-offs

- [Excimer trace API details unknown until tried] → agent experiments
  in-container; adapter owns the quirks; README records them.
- [Issue-title quality varies] → concept-class filter (no symbol-name
  titles) + honest funnel reporting; small n is acceptable — this corpus
  supplements curated sets, it doesn't replace them.
- [Unauthenticated rate limit] → hard request budget + cache + early stop.
- [Learned remediations could mask real regressions] → runs.jsonl keeps
  every attempt; a scenario that ALWAYS needs remediation is a finding,
  and the findings doc must call those out.

## Migration Plan

Additive tooling only; nothing ships in the binary. Removal = delete
bench/selfheal/.

## Open Questions

- Whether the open-issue (unscored) collection produces useful miss
  patterns or just noise — decided by the first findings pass.
- CI containerization of the PHP lab — after local green.
