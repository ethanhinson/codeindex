## 1. Progress core

- [x] 1.1 internal/progress: Event/Reporter, TTY renderer (spinner/bar/rate/ETA/throttle), JSONL renderer (v:1), sidecar writer, multi-reporter; unit tests (monotonic, terminal event, JSONL shape)
- [x] 1.2 engine.BuildWithProgress/PatchWithProgress (+ store re-resolve callback); Build/Patch wrap with nil; parse-phase counting

## 2. Surfaces

- [x] 2.1 status.json lifecycle + `codeindex status <root> [--json]` (side-effect-free; stale-building detection)
- [x] 2.2 CLI build/export/import wiring: TTY detection, --progress JSONL, plain fallback
- [x] 2.3 query.Fresh returns {Built, FilesParsed, Duration}; CLI stderr note; MCP cold-build leading line (+ test)

## 3. Gates and close-out

- [x] 3.1 Full suite green; one-repo bench sanity (no throughput regression on nil path); sample output recorded in FINDINGS-progress-ux.md
- [x] 3.2 openspec validate; commit + push; archive
