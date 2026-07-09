## Context

The overall design is set (`docs/superpowers/specs/2026-07-08-codeindex-design.md`)
and the token + re-index assumptions are validated by proxy spikes (`bench/`,
`bench/FINDINGS.md`). What proxies cannot measure is real engine behavior:
tree-sitter parse speed, SQLite write cost, and whether incremental patching is
correct. This walking skeleton builds the smallest real slice that produces those
numbers and that correctness proof, so the full `core-indexing-engine` build
starts from evidence and a proven foundation.

## Goals / Non-Goals

**Goals**

- Prove the pipeline end to end for one language: parse → symbols + call edges →
  SQLite → Merkle → incremental patch.
- Measure cold build throughput (files/s, LOC/s) and single-file incremental
  patch latency on the real reference corpora (gin, prometheus, kubernetes).
- Prove an incremental update yields a graph byte-identical (by content) to a
  full rebuild.
- Lock the package layout, adapter seam, and SQLite schema for the full build.

**Non-Goals**

- Query commands, additional languages, precise resolution, dependency/outline
  edges, directory-mtime shortcutting, MCP, plugin. This is not a usable tool
  yet — it is a measurement and de-risking instrument.

## Decisions

**D1 — Go as the single slice language.** Matches the implementation language,
has clean static structure, and we already have Go reference corpora. tree-sitter
supports it well. *Alternative:* TS/JS first — more resolution ambiguity, no
benefit for a throughput/correctness probe.

**D2 — Call edges only, name-based.** Calls are the highest-value edge (validated
as the headline query) and name-based resolution matches the MVP. Adding imports/
extends here buys nothing for the two questions this change answers. *Alternative:*
full edge set — scope creep with no de-risking value.

**D3 — Reuse the target SQLite schema subset, not a throwaway.** Use the real
`files`/`symbols`/`edges`/`merkle` tables (minus columns the slice does not fill)
so `core-indexing-engine` extends rather than migrates. *Alternative:* ad-hoc
tables — would need rework and would not validate the real write cost.

**D4 — Prove incremental == full by graph diff.** After `build`, mutate a file,
run the incremental path, then run a full rebuild into a scratch DB and diff the
normalized graphs (symbols + edges, order-independent). Equality is the pass
condition. This is the correctness proof proxies could not give. *Alternative:*
trust-by-construction — exactly the assumption we are trying to retire.

**D5 — Bench harness in Go, alongside the engine.** `codeindex bench` times cold
build and incremental patch and runs the D4 diff, emitting JSON. This seeds the
real benchmark harness (`core-indexing-engine` task 9.2). *Alternative:* an
external script — cannot measure in-process parse/patch cost precisely.

## Risks / Trade-offs

- **CGO grammar build friction** (`go-tree-sitter` + tree-sitter-go) → document
  the toolchain in the module README; if static-build friction is severe, note it
  as a finding feeding the full build's distribution decision.
- **Name-based call extraction over/under-counts edges** → acceptable; this
  change measures throughput and incremental *correctness relative to a full
  rebuild of the same extractor*, not resolution accuracy (that is change 2).
- **Throughput numbers are machine-specific** → report on the same baseline as
  the performance spec (8 cores, NVMe, warm cache) and include the machine in the
  results, consistent with `core-indexing-engine`'s benchmark methodology.

## Migration Plan

Greenfield. The slice's module, schema, and package layout are intended to be the
starting commit of `core-indexing-engine`; nothing to migrate. If findings demand
a schema or layout change, fold it into `core-indexing-engine` before its build
begins.

## Open Questions

- tree-sitter-go binding choice (`smacker/go-tree-sitter` vs official
  `tree-sitter/go-tree-sitter`) — decide at implementation based on grammar
  currency and build simplicity.
- Whether to record LOC via a cheap line count or from tree-sitter node spans —
  default to a cheap line count for the throughput metric.
