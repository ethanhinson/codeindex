# codeindex

A code navigation index that lets an agent (Claude, and eventually IDEs) answer
"who calls X?", "what depends on X?", and "where is X defined?" with compact
`file:line + signature` references instead of grepping and reading whole files —
saving tokens and latency. See `docs/superpowers/specs/` for the design and
`openspec/` for the spec-driven plan.

## Status

Walking skeleton (OpenSpec change `engine-walking-skeleton`): a minimal but real
Go engine slice that validates parse/patch throughput and proves that an
incremental update equals a full rebuild. Not yet the full tool — no query
commands, one language (Go), name-based call edges only. See
`bench/engine/FINDINGS.md` for measured results.

## Build

Requires **Go 1.24+** and a **C toolchain** (CGO is used by `go-tree-sitter` and
`go-sqlite3`; macOS: Xcode command-line tools provide `clang`).

```
go build ./cmd/codeindex
go test ./...
```

## Use (skeleton)

```
codeindex build <repo-root>          # index a Go repo -> <repo>/.codeindex/graph.db
codeindex bench <repo-root> [out.json]  # measure throughput + prove incremental==full
```

## Layout

```
cmd/codeindex        CLI (build, bench)
internal/adapter     pluggable language-adapter seam
internal/adapter/golang  tree-sitter Go adapter (symbols + call sites)
internal/graph       SQLite store, data model, deterministic name resolution
internal/merkle      file walk + content hashing + fast-path change detection
internal/engine      build + incremental patch orchestration
bench/               token-savings + re-index validation spikes (Python) and
                     engine benchmark results
```

## Token-savings evidence

The `bench/` Python spikes validated the core premise before the engine existed
(100–500× fewer tokens for def/callers on large-file Go; see `bench/FINDINGS.md`).
Set `bench/.env` (see `.env.example`) to count with Claude's exact tokenizer.
