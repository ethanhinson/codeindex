# Tasks: runtime-evidence-stack

## 1. Format + storage foundation

- [x] 1.1 Write `docs/cxprof-format.md` (one page, v1: header + stack records, frames-only rule, versioning policy) and a conformance checker (`codeindex ingest --check`)
- [x] 1.2 Schema v9 in `internal/graph`: `obs_edges` (src/dst symbol, weight, indirect flag), `obs_heat` (symbol, leaf/total samples), `obs_meta` + spool ledger; house rebuild policy notes
- [x] 1.3 `internal/runtime`: cxprof parser (JSONL, tolerant of unknown fields), frame→symbol span resolution with unresolvable-frame collapse, resolution-rate reporting
- [x] 1.4 `codeindex ingest <file|dir>` verb + spool discovery on the fresh-on-query pass (ledger-deduped, files left in place, `--prune` gc)
- [x] 1.5 Unit tests: parser conformance, span resolution incl. gap collapse, ledger idempotence, additive-evidence invariants (D7)

## 2. Ranking consumption

- [x] 2.1 Diffusion subgraph unions observed edges (weight-normalized); heat enters entry selection and boosts at the frozen compressed exponent only
- [x] 2.2 Provenance/staleness disclosure in search/impact output (`[observed <age>]`, commit-mismatch downgrade)
- [x] 2.3 Unit tests: observed-edge diffusion on synthetic graphs, disclosure rendering, no-spool no-op parity (results identical to pre-change when no runtime evidence exists)

## 3. Stage-1 SDKs (Go, Node)

- [x] 3.1 `sdk/go`: wrap `runtime/pprof` → cxprof; env-var enable, spool writer (temp+rename), bounds + kill switch per D3; README
- [x] 3.2 `sdk/node`: wrap `node:inspector` Profiler → cxprof; same contract; no runtime deps; README
- [x] 3.3 Conformance + non-disruption tests per SDK (unwritable spool, kill switch, buffer cap)
- [x] 3.4 End-to-end dev-loop proof: profile a real app run (this repo's own test binary for Go; a small express app for Node), spool → fresh-on-query ingest → observed edges visible in `search`/`impact`

## 4. Gate (pre-registered in design D8)

- [x] 4.1 Pin WordPress core + Drupal core; author + freeze curated any-of-N sets (incl. hook-dispatch subclass tagging) BEFORE any ingestion runs
- [x] 4.2 Static-only baseline runs (expected weak; the honest floor)
- [x] 4.3 PHP evidence generation: Excimer adapter (stage-3 SDK pulled forward just far enough for the gate) + scripted exercise flows; ingest
- [~] 4.4 *(gate run once, D6-completion disclosed, both iterations UNSPENT; held-out app corpus deferred — see FINDINGS)*  <!-- original: -->
<!-- - [ ] 4.4 Runtime-augmented runs vs D8 bars; ≤2 registered iterations on ingestion/ranking; held-out application corpus (pinned OSS Laravel/Django app via its test suite) once at freeze
- [x] 4.5 Record verdict in `bench/engine/FINDINGS-runtime-evidence.md`; residual analysis feeds the backlog

## 5. Stage 2+ (collector, Python) — after the gate

- [ ] 5.1 `codeindex collector` mode (bounded HTTP receiver → spool; caps, drop-and-count, no privileges)
- [ ] 5.2 `sdk/python` (stdlib sampling: 3.15 `profile.sample`, 3.12+ `sys.monitoring` fallback)
- [ ] 5.3 README + plugin docs: dev recipe per stack, prod deployment page, format link; archive ordering: `diffusion-contrast-retrieval` first
