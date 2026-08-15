# Proposal: runtime-evidence-stack

## Why

Static analysis — ours or any compiler's — cannot see string-keyed hooks,
DI containers, config-wired dispatch, or reflection, and it cannot tell a
real entry point from dead code when frameworks are the caller. These are
the measured blind spots behind the residual buckets (entry-point
preference: 11 of 24 tuning misses) and the expected failure mode on
hook-based corpora (WordPress/Drupal). Runtime evidence sees all of it:
the interpreter executes hook → callback, and a sampled stack says so.

Strategy decision (deliberate reversal, recorded): we OWN the whole
reporting stack — collection SDKs, wire format, collector, ingestion,
reporting — rather than riding third-party profiler agents. Rationale:
onboarding must be free and frictionless for downstream providers (no
accounts, no licenses, no someone-else's-agent as a prerequisite), and the
evidence pipeline is too load-bearing to depend on ecosystems we don't
control. The compensating discipline: the wire format is open, versioned,
and trivially emittable by anyone — we own the reference implementation,
never the wire.

## What Changes

- **cxprof format (v1)**: an open, one-page, versioned profile format —
  JSONL; header record (version, lang, pid, sample unit, time span,
  optional commit) + one record per unique stack: frames as
  `[file, line]` innermost-last, with a sample count. Documented in
  `docs/cxprof-format.md`. Anything that can write JSON lines can onboard.
- **Agent SDKs** (thin, in-process, fire-and-forget — no root, no kernel,
  no daemons in dev):
  - Go: wraps stdlib `runtime/pprof` → cxprof (in-repo module `sdk/go`).
  - Node: wraps built-in `node:inspector` V8 sampler → cxprof (`sdk/node`).
  - Python (stage 2): wraps stdlib sampling (`sys.monitoring` /
    `profile.sample` on 3.15+) → cxprof (`sdk/python`).
  - PHP (stage 3): adapter over the Excimer extension (Wikimedia,
    production-proven) → cxprof; an owned extension only if adoption
    demands it.
  Non-disruption is spec-level: sampling defaults, hard-bounded buffers,
  drop-never-block, frames-only payloads (no arguments/values), kill-switch
  env var.
- **Dev deployment = file drop, zero processes**: SDKs spool
  `*.cxprof.jsonl` into `.codeindex/runtime/`; the existing fresh-on-query
  pass ingests new spools exactly like changed files patch the index.
- **`codeindex ingest <file|dir>`**: explicit ingestion verb — frames
  resolve to symbols by span lookup (the grep-attribution machinery),
  producing OBSERVED call edges (with sample counts) and per-symbol heat
  in `graph.db`, provenance-stamped (profile time, source, commit).
- **Ranking consumes runtime evidence**: diffusion runs over static ∪
  observed edges; entry-point selection and boosts gain observed-heat and
  externally-invoked signals. Results citing runtime-only edges disclose
  provenance (`[observed <age>]`), house-style.
- **`codeindex collector` (stage 2)**: same binary, one flag — a bounded
  localhost/network HTTP receiver that spools cxprof for prod sampling.
- **Gate (pre-registered before measurement)**: WordPress and/or Drupal —
  the corpora where static analysis is expected to fail — curated concept
  sets; bar: runtime-augmented search materially beats static-only on
  hook-dispatch questions (exact bars registered in design D8 before any
  run).

## Capabilities

### New Capabilities

- `cxprof-format`: the open wire format — record shapes, versioning rules,
  frame addressing, non-goals (no PII, frames-only).
- `runtime-ingestion`: spool discovery + `ingest` verb, frame→symbol
  resolution, observed edges + heat storage, provenance stamping, staleness
  disclosure, model of sampled truth (absence proves nothing).
- `agent-sdks`: per-language in-process samplers emitting cxprof with the
  non-disruption requirements (bounds, drop-never-block, kill switch).
- `runtime-collector` (stage 2): the prod receiver mode.

### Modified Capabilities

- `semantic-search`: ranking requirements gain runtime evidence — diffusion
  over static ∪ observed edges, heat-aware entry selection, observed
  provenance disclosure in output.

## Impact

- **Code**: `internal/graph` (observed edges + heat tables, schema bump),
  `internal/runtime` (cxprof parse, spool discovery, frame→symbol
  resolution), `internal/engine` or query layer (fresh-on-query spool
  pickup), `internal/search` (diffusion edge union, heat in boosts/entry),
  `cmd/codeindex` (`ingest`, later `collector`), `sdk/go`, `sdk/node`
  (new modules), docs.
- **Schema**: v8 → v9 (observed edges/heat + runtime meta); house
  delete-and-rebuild policy applies (graph rebuilds; spools re-ingest).
- **New surface**: two SDK packages now, two more later — versioned against
  the format, not against codeindex internals.
- **Bench**: WordPress/Drupal probe repos + curated sets; doubles as the
  corpus-coverage probe from the field-testing discussion.
- **Out of scope here**: the field-measurement loop (search journal,
  mark-miss, report) — separate change; eBPF/OS-level collection —
  rejected for v1 (symbolization cost, privileged deploys vs the
  simplicity requirement).
