# Design: runtime-evidence-stack

## Context

The diffusion-contrast gate passed everywhere static evidence exists, and
its residual analysis plus the customer-framework exploration identified
what static evidence cannot reach: string-keyed dispatch (WordPress
`add_action`, Drupal hooks), config/DI wiring, reflection, and
entry-point identification when frameworks are the caller. Runtime stacks
see all of these trivially. The product decision (user-directed, recorded
in the proposal): own the entire reporting stack, with deployment
simplicity and free downstream onboarding as hard constraints.

Research verified per-runtime in-process collection (no OS-level agent
needed): Node's built-in `node:inspector` V8 sampler; Python 3.12+
`sys.monitoring` (PEP 669) and the 3.15 stdlib statistical sampler; PHP's
Excimer (Wikimedia, Wikipedia-production-proven, ~1ms sampling); Go's
stdlib `runtime/pprof`. All emit or can be adapted to file:line frames —
the address our span-attribution machinery already resolves.

## Goals / Non-Goals

**Goals:**
- One-page open wire format anyone can emit; our SDKs are convenience,
  never lock-in.
- Dev mode with zero new processes: file-drop spool + fresh-on-query
  ingestion.
- Observed edges + heat improving search/impact where static is blind,
  with provenance disclosure.
- Non-disruption as spec: sampled, bounded, drop-never-block, frames-only.
- Pre-registered gate on hook-based corpora before promotion.

**Non-Goals:**
- eBPF/OS-level collection (rejected v1: per-runtime symbolization war,
  privileged deploys contradict the simplicity constraint).
- Tracing/spans/APM semantics — this is sampled stacks only.
- Any network egress by default; any account/licensing gate.
- The field-measurement loop (separate change).

## Decisions

### D1: cxprof v1 — JSONL, frames as [file, line], counts per unique stack
Header record: `{"cxprof":1,"lang":"php","unit":"samples","hz":100,
"start":...,"end":...,"commit":"<optional>","tag":"dev|prod"}`. Stack
record: `{"st":[["src/a.php",12],["src/b.php",40]],"n":17}` — innermost
LAST, paths repo-relative when the emitter knows the root, else absolute
(ingest re-roots). Rationale: JSONL is emittable from any language in an
afternoon (the onboarding constraint); file:line is the only frame address
that is runtime-agnostic AND resolvable by our existing span lookup.
Alternatives considered: pprof proto (powerful, but protobuf dependency
contradicts trivial emission), collapsed stacks (no file:line), speedscope
(viewer-oriented, heavier). Versioning: additive fields only within v1;
breaking = v2 header.

### D2: SDKs wrap each runtime's native sampler; never instrument calls
Go: `pprof.StartCPUProfile` into a buffer, convert. Node:
`inspector.Session` Profiler domain (built-in). Python: stdlib sampling
(3.15 `profile.sample`; 3.12–3.14 `sys.monitoring`-assisted sampler);
PHP: Excimer adapter (owned extension only if adoption demands).
Instrumenting every call (Xdebug-style) is banned even in dev mode —
one mechanism across environments, only the sampling rate differs.

### D3: Non-disruption requirements (spec-level, all SDKs)
Defaults: dev 99Hz, prod 19Hz or 1%-of-requests; buffers hard-capped
(default 8MB) with whole-profile drop on overflow; spool writes and
collector sends are async and fire-and-forget; failure is silent-with-
counter, never thrown into the host app; `CODEINDEX_PROFILING=off` kills
everything; payloads are frames-only (no argument values, no env, no PII
shapes).

### D4: Dev transport = file drop into `.codeindex/runtime/`
SDKs write `<unixts>-<pid>.cxprof.jsonl` (write-temp, rename). The
fresh-on-query pass treats unseen spool files exactly like changed source
files: ingest, record in a ledger table, leave files in place (operator
deletes; a `--prune` flag can gc ingested spools). No daemon, no socket,
no config. Prod transport (stage 2) = `codeindex collector` — same
binary, bounded HTTP receiver writing the same spool dir.

### D5: Ingestion — frames resolve by span; edges are adjacent-frame pairs
For each stack, resolve each frame (file,line) to the enclosing tier-0
symbol via span lookup (grep-attribution machinery); unresolvable frames
(vendor, runtime internals) collapse — an edge spans the gap between the
nearest resolvable frames, flagged `via-external`. Adjacent resolved
frames yield OBSERVED edges (caller→callee) weighted by sample count;
per-symbol heat = total samples where the symbol is on-stack (leaf +
non-leaf tracked separately). Stored in new tables (`obs_edges`,
`obs_heat`, `obs_meta` with source/time/commit); schema v9, house
rebuild policy (spools re-ingest after rebuild via the ledger).

### D6: Ranking consumption — union edges for diffusion, heat as tiebreak
Diffusion's subgraph query unions static edges with observed edges
(observed carry weight from sample counts, row-normalized as usual).
Entry selection prefers externally-invoked observed symbols (first
resolvable frame of stacks = something the framework/runtime called).
Heat enters boosts only at the compressed exponent the contrast gate
froze (no new popularity contest — the boost-domination lesson stands).
Any result whose ranking materially depended on observed-only evidence
carries `[observed <age>]`; stale evidence (older than a configurable
horizon or a different commit) is disclosed, not silently trusted —
sampled truth: presence is evidence, absence proves nothing.

### D7: Sampled-truth invariants
Observed edges never REMOVE static conclusions (no dead-code verdicts
from absence); they only add edges/weight. Dead-code candidacy = static
zero-callers AND zero heat AND no registration — and even then reported
as candidacy.

### D8: Gate (pre-registered here, before any measurement)
Corpora: WordPress core and Drupal core (the hook-dispatch corpora static
analysis fails on), pinned; curated any-of-N sets authored under
concept-eval rules, frozen before ingestion runs. Evidence generation:
exercise each app's request paths locally (installer/front page/admin,
scripted) under the PHP SDK. Bars:
1. Static-only curated hit@5 measured first (expected weak — the honest
   baseline).
2. Runtime-augmented ≥ +15 points over static-only on each corpus, and
   ≥ 55% absolute.
3. Hook-dispatch question subclass (questions whose answers are hook
   callbacks): runtime-augmented ≥ 2× static-only hit rate.
4. Tuning/held-out curated sets from the diffusion gate: non-regression
   (observed evidence must not distort corpora that don't need it —
   trivially true where no spools exist, verified on one repo WITH a dev
   profile ingested).
5. SDK overhead: measured wall-clock delta under 99Hz dev sampling ≤ 3%
   on the exercised flows.
Iteration budget: two registered iterations on ingestion/ranking; the
gate corpora function as tuning for runtime mechanisms; a held-out
runtime corpus (a pinned OSS Laravel or Django APPLICATION exercised via
its test suite) runs once at freeze — this also finally measures the
application-code axis.

## Risks / Trade-offs

- [Sampling misses rare paths] → D7 invariants; docs state absence ≠ dead.
- [Frame→symbol misses on inlining (Go) / generators (Py) / eval frames] →
  unresolvable-frame collapse (D5) keeps edges usable; resolution rate
  reported per ingest so degradation is visible.
- [Two evidence lifecycles (code vs profiles) desync] → provenance stamps
  + staleness disclosure (D6); commit mismatch downgrades weight.
- [SDK maintenance across 4 runtimes] → SDKs version against cxprof v1
  (stable by construction), not against codeindex internals; format
  conformance suite in-repo.
- [PHP dependency on Excimer] → explicit stage-3 note; owned extension is
  a future change gated on adoption; WordPress gate uses Excimer and that
  dependency is disclosed in findings.
- [Heat re-introduces popularity bias] → compressed exponent + tiebreak-
  only role (D6); the boost-domination measurement from the contrast gate
  is the guard rail.
- [Prod collector = a daemon (house principle bends)] → optional, stage 2,
  never on the dev path; recorded as a deliberate bend.

## Migration Plan

Schema v8→v9 (additive tables); house policy rebuilds the index, spools
re-ingest from the ledger. Rollback = prior binary (rebuild without obs
tables). SDKs are forward-compatible: cxprof v1 emitters keep working.

## Open Questions

- Exercise scripts for WordPress/Drupal gate flows: hand-rolled curl
  scripts vs their own test suites — decide when authoring the gate.
- Heat decay: should old samples age out at ingest time or only be
  disclosed? v1 = disclose only; decay revisited with field data.
- Node ESM loader hook vs explicit `require` for SDK init — decide in
  implementation with a bias to explicit.
