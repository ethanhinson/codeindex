# codeindex

A code navigation index that lets an agent (Claude, and eventually IDEs) answer
"who calls X?", "what depends on X?", and "where is X defined?" with compact
`file:line + signature` references instead of grepping and reading whole files —
saving tokens and latency. See `docs/superpowers/specs/` for the design and
`openspec/` for the spec-driven plan.

## Status

Working tool with validated consumption surfaces:

- **Engine**: tree-sitter adapters for **Go, TypeScript/JavaScript, Python,
  PHP**; name-based call edges with `[ambiguous]` confidence flags;
  content-hash incremental updates proven equal to full rebuilds on real repos
  (kubernetes, nest, flask, laravel). Queries are always fresh (auto-build +
  patch-on-query).
- **Claude Code plugin** (`plugin/`): prompt-note + post-edit blast-radius
  hook + `/impact` — the shape that passed the pre-registered A/B gate
  (branch-out +62%, locate within tolerance, hook 100%/0 false fires).
- **MCP server**: `codeindex mcp <repo>` for Cursor/Claude Desktop/VS Code.

Evidence: `bench/engine/FINDINGS*.md` (engine), `bench/agent_ab/FINDINGS*.md` +
`results/dashboard.html` (agent A/B, v1–v4). Measured savings are from Go-repo
experiments; other languages share the mechanics (engine-validated).

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
