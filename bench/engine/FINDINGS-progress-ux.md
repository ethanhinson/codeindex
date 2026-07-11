# Findings: index progress UX (engine events + surfaces)

Date: 2026-07-11 · No schema change

## What shipped

One progress-truth source (internal/progress): the engine emits phase events
(walk → parse → write → resolve, per-name resolve granularity since
resolution dominates big builds); every surface renders the same stream.

- **TTY** (stderr interactive): braille spinner, block bar, live rate + ETA,
  final ✓ summary —
  `⠙ index prometheus resolving ██████████████████████ 5482/5482 (100%) 700/s`
  `✓ indexed 792 files (8991 symbols) in 7.8s`
- **Plain** (non-TTY): throttled log lines, no ANSI.
- **JSONL** (`--progress`, stdout): versioned events
  (`{"v":1,"phase":"parse","done":1,"total":91}` … terminal
  `{"phase":"done","summary":"indexed 91 files (1179 symbols)","v":1}`) —
  the IDE extension's feed.
- **Sidecar**: `.codeindex/status.json` maintained through every
  build/patch (building/patching + phase + counts; terminal fresh state with
  files/symbols/indexed_at/duration); best-effort, atomic rename, never
  fails a build.
- **`codeindex status <root> [--json]`**: unindexed / building (with
  stale-builder detection >10min) / stale-schema / indexed with counts and
  size. Side-effect-free — verified it creates no .codeindex on an
  unindexed repo. The extension's detection primitive.
- **Cold-build disclosure**: first query on an unindexed repo reports
  `[codeindex: indexed 91 files (1179 symbols) in 698ms — first query on
  this repo; subsequent queries are fast]` (CLI: stderr; MCP: first line of
  the tool result via the shared text() helper). Second query: silent.
  One disclosure per build, by construction (ConsumeColdBuild).

## Gates

- Full suite green, including new tests: JSONL well-formed/versioned/
  terminal-event, sidecar lifecycle (building → progress → fresh), TTY
  bar+summary rendering, build events monotonic per phase with
  done==total at completion, Multi tolerates nil reporters.
- Bench sanity (nil-reporter path + always-on sidecar): prometheus cold
  build 7.78s vs 7.78s pre-change — no measurable regression; inc==full
  still true.

## Notes

- Export/Import now accept reporters; CLI wires them (same three-way
  renderer policy). Machine feed owns stdout; human summary stays on stderr.
- ANSI on legacy Windows terminals falls back via the non-TTY path
  (recorded design risk, not blocking).
