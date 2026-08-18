# Residuals buckets 3 + 1: sample-corpus penalty shipped, entry-point roles withheld

**Date:** 2026-08-15 · **Change:** residuals work from
`openspec/changes/diffusion-contrast-retrieval/residuals-backlog.md`

## Verdict

- **Bucket 3 (sample/demo corpus noise): SHIPPED** — file-class penalty
  extended to sample corpora and moved OUTSIDE the boost-compression
  envelope. Zero regression on every gate; nest mechanical strict
  **55.0 → 60.0** as a side effect. The bucket's specific hit@5 flip
  predictions did NOT come true (recorded below) — the aggregate value is
  ordering quality plus the mechanical gain, not curated points.
- **Bucket 1 (entry-point preference): WITHHELD from defaults** —
  experimental behind `CODEINDEX_ROLE_BOOST=1` (literal-lane precedent).
  nest 65.4 → **76.9** (bar ≥75 MET; every flip is the bucket's exact
  thesis) but gin 88.5 → 84.6 and flask 76.0 → 72.0 break the
  no-regression conjunction. Two-iteration budget spent; failure mode
  named and a follow-up registered in the backlog.

## Bucket 3 — what shipped

1. `filePenalty` (internal/search/find.go): test markers extended with
   `.spec.` and exact-path-segment sample dirs (sample(s), example(s),
   demo(s), benchmark(s), fixture(s), integration). Segment-equality so
   core files merely containing the words are untouched.
2. Envelope fix (the measured part): `boosts` split into `graphBoost`
   (compressed by `boostGamma` in semantic search — its raw range needs
   it) and `filePenalty` applied at FULL strength. Inside the envelope a
   0.7 penalty decays to 0.7^0.35 ≈ 0.88 — measured to move fixture noise
   by ~1 rank and flip nothing. `find`'s ladder always applied full
   strength; its behavior is unchanged by the refactor.

Measured (curated tuning sets, binary at this commit, gate off):
88.5 / 76.0 / 65.4 / 76.9 — identical to the FROZEN baselines, per-question
diffs show fixture symbols dropping below core everywhere (e.g. nest
`NotFoundException` now #1 over `ErrorsController.throwError`;
`callModuleInitHook` above the `AA`/`BB` e2e fixtures). Mechanical concept
class: gin 71.7 / flask 70.0 (unchanged), **nest 60.0 (+5.0)** — penalized
doc-clone siblings in test dirs left the strict top-5. find vague classes:
93.5% (unchanged). Registered flip predictions (nest "bootstrap", flask
"test client") did NOT flip: their blockers are retrieval (accepted answers
not in any lane's top-50), not fixture noise — that is bucket 1's problem.

## Bucket 1 — what was measured and why it's withheld

Diagnosis first (cards ruled out): the missed public APIs have GOOD cards —
`Injectable`'s doc literally reads "marks a class as a provider …
injected" — yet it sat at rank 16 with lanes=[graph]: neither lane's
top-50 retrieves it (drowned by hundreds of injector-package cards); only
diffusion pulls it in. So the fix belongs in ranking the union, exactly as
the bucket predicted.

Mechanism: structural roles from the diffusion subgraph's directed edges,
computed at query time from data already in memory.

- **Iteration 1 — foreign-caller-directory count** (`(1+dirs)^0.25` on the
  blended score): nest 65.4→73.1, gin 88.5→80.8. FAILED: in flat repos the
  dir signal is wiring, not surface — gin render helpers (`writeContentType`,
  18 same-dir callers) outranked the API. Artifacts:
  `bench/results/curated-ROLES-i1-*.json`.
- **Iteration 2 — user-side caller votes**: an in-edge votes for its callee
  iff the CALLER file is in the filePenalty class (tests, samples, e2e
  fixtures — user-side corpora exist to demonstrate the public API), and
  penalized targets get no votes (fixtures don't vote for fixtures).
  Full-circle with bucket 3: the same file class is penalized as an ANSWER
  and trusted as a VOTER. nest 65.4→**76.9** — the three flips are
  precisely the entry-point thesis (`Injectable`, `UseInterceptors`,
  `Injector.resolveSingleParam` entering top-5 over Module.* plumbing) —
  laravel unchanged, but gin −3.9 / flask −4.0. FAILED the conjunction:
  both losses are hyper-generic surface APIs with dozens of test-file
  votes (`Context.Status`, `_AppCtxGlobals.get`) edging out on-topic
  accepted answers that sat exactly at rank 5. Artifacts:
  `bench/results/curated-ROLES-i2-*.json`.

**Named failure mode:** unbounded vote counts. The signal identifies the
surface correctly; its MAGNITUDE lets the most-tested generic API beat the
right answer in near-ties. The registered follow-up (backlog bucket 1,
updated) is vote saturation — cap or squash distinct-voter counts so
"demonstrated by user code at all" dominates and "demonstrated 40 times"
adds little. That is a NEW change with a fresh 2-iteration budget and the
same bars (nest ≥75, no tuning regression), not a quiet third iteration
here.

## Discipline notes

- The curated baseline was re-reproduced (exactly 88.5/76.0/65.4/76.9) on
  today's binary BEFORE any mechanism ran.
- Bucket-3's compressed first cut measured 0.0 aggregate change everywhere —
  the envelope diagnosis came from that null result, not from theory.
- All four gates re-run on the final gated binary in one battery; the gated
  path (`CODEINDEX_ROLE_BOOST=1`, nest) reproduces 76.9.

## Appendix 2026-08-16 — vote-count diagnostic: saturation falsified pre-budget

The registered follow-up (vote saturation, fresh 2-iteration budget) was
subjected to a budget-free diagnostic BEFORE any mechanism was written:
a temporary `CODEINDEX_ROLE_DEBUG=1` dump in the role-boost block
(per-candidate query-time vote count, pre/post-boost blend score, voter
files with per-voter spread), replayed on exactly the five recorded
i2 flips (diff of `curated-FROZEN-*` vs `curated-ROLES-i2-*`; all five
reproduced the recorded top-5s exactly on today's binary).

### Measured constraints (boost factor each case needs / must stay under)

| case | symbol | votes | pre-boost | constraint |
| --- | --- | --- | --- | --- |
| gin harm | `Context.Status` | **1** | 0.613 | f < ×1.045 (NegotiateFormat at 0.641, 0 votes) |
| nest gain | `Injector.resolveSingleParam` | **1** | 0.528 | f ≥ ×1.147 (addExportedProvider at 0.605) |
| flask harm | `_AppCtxGlobals.get` | 8 | 0.292 | f < ×1.653 (do_teardown_request at 0.482) |
| nest gain | `UseInterceptors` | 12 | 0.422 | f ≥ ×1.225 |
| nest gain | `Injectable` | 15 | 0.300 | f ≥ ×1.899 (pure-diffusion floor, lanes=[graph]) |

### Verdict: infeasible, and not just for counts

1. **The 1-vote pair is contradictory.** Rows 1 and 2 have identical vote
   counts and need opposite outcomes. No cap, threshold, log, binary, or
   any other function of count satisfies both. Every hybrid also dies:
   threshold ≥2 drops resolveSingleParam (nest 19/26 = 73.1 < 75);
   cap ≤6 (needed for flask) kills Injectable (needs ≥12 effective votes).
2. **Voter identity does not rescue it.** Status's single voter is
   `benchmarks_test.go` (spread 16); resolveSingleParam's is a sample-app
   service (spread 15). One promiscuous voter each. Promiscuity weighting
   (1/spread) and directory-dedup fail quantitatively elsewhere too
   (dir-dedup leaves a γ window of ≈0.27–0.28 between keeping Injectable
   and re-breaking flask — overfit-thin — and cannot touch the 1-vote pair).
3. **Corrections to the record.** (a) The i2 account said "dozens of
   test-file votes" for the gin harm — wrong: votes are counted inside the
   query's diffusion subgraph and Status had ONE at query time; the gin
   regression was never a magnitude problem. (b) Injectable's marquee gain
   RELIED on the unbounded ×2.0 — the "membership, not magnitude" theory
   of the prior is contradicted by the mechanism's own best result.
4. **The structural insight.** The flask harm and the nest gains are the
   SAME observable event — a heavily-user-demonstrated symbol vaulting
   over unvoted plumbing from below. Count, voter spread, direction, and
   gap size are all symmetric between them (the harm's gap is SMALLER than
   the gain's). The only difference is whether the lifted symbol is topical
   for the query — which the vote channel cannot see. The prior is not too
   strong; it is query-independent.

### What survives (for the successor change, unmeasured)

- Query-conditioned evidence: weight votes by voter-to-query relevance, or
  move usage evidence into card content (definition-by-usage, = bucket 2's
  mechanism) so the query itself gates when it fires.
- Result-set diversity (MMR / cluster round-robin over the existing D4
  clusters) against the clone-wall geometry: five near-identical Module.*
  cards above Injectable at rank 16. Cannot lift off-topic symbols by
  construction; dry-runnable offline from recorded candidates.
- Corpus ground-truth for prose lanes (checked 2026-08-16): in-repo docs
  are DEAD on the weak repos (nest's real docs are out-of-repo; Readme has
  0 `Injectable` mentions; gin docs are one file) — but commit messages
  are rich in task vocabulary in ALL THREE repos ("Support negotiation
  wildcards", "Update the signature of the teardown function", "add
  Injectable decorator to..."), and nest filenames literally spell the
  queries (`injectable.decorator.ts`, `use-interceptors.decorator.ts`).

Cost of this diagnostic: zero iterations. The instrumentation was
temporary and reverted; raw dumps were session-local. To reproduce: dump
`len(userVotes[id])` and pre/post scores for `topIDs(final, 40)` plus
voter files with spread, behind an env var, and run the five flip queries
with `CODEINDEX_ROLE_BOOST=1` on the pinned bench clones.
