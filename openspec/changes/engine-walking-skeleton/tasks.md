## 1. Module scaffolding

- [x] 1.1 Initialize the Go module (`codeindex`) with layout: `cmd/codeindex`, `internal/{adapter,graph,merkle,engine}`, `bench/engine`
- [x] 1.2 Add dependencies: a `go-tree-sitter` binding + tree-sitter-go grammar, and a SQLite driver; confirm the CGO build works and document the toolchain — done (smacker/go-tree-sitter + mattn/go-sqlite3; toolchain in README)
- [x] 1.3 Wire a root CLI with `build` and `bench` subcommands (stubs)

## 2. Go language adapter (slice)

- [x] 2.1 Define the adapter seam (`Parse`, `ExtractEdges`) that the full engine will reuse
- [x] 2.2 Parse Go with tree-sitter; extract function/method/type symbols (name, kind, signature, line span)
- [x] 2.3 Extract call sites as name-based `calls` edges from the enclosing symbol; mark resolution confidence; retain unresolved calls
- [x] 2.4 Test the adapter against Go fixtures: assert expected symbols and call edges

## 3. SQLite store (target schema subset)

- [x] 3.1 Create `files`, `symbols`, `edges`, `merkle` tables (skeleton denormalizes owning file as a path column rather than `file_id` — noted in FINDINGS for the full engine)
- [x] 3.2 Implement per-file upsert/replace of symbols + edges and file/merkle rows in a transaction
- [x] 3.3 Test: per-file replace removes stale rows and leaves other files untouched (covered by engine incremental==full tests)

## 4. Merkle + change detection

- [x] 4.1 Hash file contents into Merkle leaves; store per-file size, mtime, and hash
- [x] 4.2 Implement the mandatory size+mtime fast path, falling back to content hashing only on mismatch
- [x] 4.3 Compute the changed-file set (added/modified/deleted) against stored state
- [x] 4.4 Test: unchanged repo → empty change set; edit/add/delete → correct set (covered by engine add/delete/edit tests)

## 5. Build + incremental patch

- [x] 5.1 Implement `codeindex build`: walk Go files, parse concurrently, resolve name-based call edges, persist; report files and symbols
- [x] 5.2 Implement the incremental path: detect changed files, re-parse only those, replace their symbols/edges in one transaction
- [x] 5.3 Test: build → edit a file → incremental patch reflects the edit; unchanged files' rows unchanged

## 6. Benchmark (fills the engine-only gaps)

- [x] 6.1 `codeindex bench`: time cold build (files/s, LOC/s) and single-file incremental patch latency against gin, prometheus, kubernetes; emit JSON to `bench/engine/`
- [x] 6.2 Implement the incremental-equals-full proof: build, edit, patch, full-rebuild into a scratch DB, diff normalized symbol+edge sets; fail on difference
- [x] 6.3 Record the baseline machine with results; write a findings note comparing measured throughput/latency to the `core-indexing-engine` targets (`bench/engine/FINDINGS.md`)

## 7. Verification

- [x] 7.1 Run the bench on all three corpora; confirm incremental == full rebuild passes and record throughput/latency numbers — all pass, all targets met with headroom
- [x] 7.2 Fold findings into `core-indexing-engine` (targets confirmed by measurement; tasks 9.2 / 10.1 de-risked)
- [x] 7.3 Run `openspec validate engine-walking-skeleton` and confirm the change is valid and complete
- [ ] 7.4 Re-run the bench on the spec's 8-core x86 baseline machine to record official numbers (current numbers are arm64 laptop, directional)
