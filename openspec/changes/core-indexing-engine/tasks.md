## 1. Project scaffolding

- [ ] 1.1 Initialize Go module (`codeindex`) with directory layout: `cmd/codeindex`, `internal/{adapter,resolver,merkle,graph,query,cli}`
- [ ] 1.2 Add dependencies: `go-tree-sitter` (+ TypeScript/TSX/JavaScript/Go grammars), SQLite driver (`mattn/go-sqlite3`), a CLI framework (e.g. `cobra`)
- [ ] 1.3 Add `.gitignore` entry for `.codeindex/` and set up a basic `make build`/`go build` target that produces the binary
- [ ] 1.4 Wire a root CLI command with `build`, `callers`, `callees`, `def`, `deps`, `dependents`, `search`, `outline` subcommand stubs and global `--json` / `--context N` flags

## 2. Storage layer (symbol-graph)

- [ ] 2.1 Define the SQLite schema: `files`, `symbols`, `edges`, `merkle` tables with indexes on `symbols.name`, `edges.src_symbol_id`, `edges.dst_symbol_id`
- [ ] 2.2 Implement schema creation/migration on first open of `.codeindex/graph.db`
- [ ] 2.3 Implement graph store CRUD: upsert file, insert/replace symbols and edges for a file, delete a file's symbols/edges, all within a transaction
- [ ] 2.4 Test: round-trip a file's symbols and edges; verify per-file replace removes stale rows and leaves other files untouched

## 3. Merkle content tree (code-indexing)

- [ ] 3.1 Implement the repo walk honoring `.gitignore` + built-in ignore defaults
- [ ] 3.2 Implement per-file content hashing with a size+mtime fast-path, and directory-node hashing up to a root
- [ ] 3.3 Persist Merkle nodes and compute a diff (added/modified/deleted files) between the current tree and the stored tree
- [ ] 3.4 Test: no changes → empty diff; edit/add/delete each produce the correct changed-file set; fast-path skips unchanged files

## 4. Language adapters (code-indexing)

- [ ] 4.1 Define the adapter interface (`Parse`, `ExtractEdges`, `ResolveImports`) and an extension→adapter registry
- [ ] 4.2 Implement the Go adapter: extract funcs/methods/types/structs as symbols; extract call, import, and interface `implements`/embed edges
- [ ] 4.3 Implement the TypeScript/JavaScript adapter (`.ts/.tsx/.js/.jsx`): extract functions/classes/methods/types; extract call, import, `extends`, `implements` edges
- [ ] 4.4 Test each adapter against fixture files: assert the expected symbols (name/kind/signature/line span) and raw edges are produced

## 5. Name-based resolver (symbol-graph)

- [ ] 5.1 Resolve raw edges by symbol name across the index; set `resolved_confidence` = unambiguous / ambiguous
- [ ] 5.2 Retain unresolved references as edges with the name preserved (no fabricated target)
- [ ] 5.3 Test: single definition → unambiguous; multiple definitions → ambiguous with candidates; no definition → unresolved

## 6. Build orchestration (code-indexing)

- [ ] 6.1 Implement `codeindex build`: walk → hash → parse (concurrent worker pool) → resolve → persist, reporting files indexed and symbols found
- [ ] 6.2 Skip files with no registered adapter without error
- [ ] 6.3 Test: full build of a mixed Go+TS fixture repo produces the expected symbol/edge counts

## 7. Lazy re-check integration (code-indexing)

- [ ] 7.1 Implement a pre-query hook: recompute Merkle root, diff, re-parse+patch only changed files in one transaction, before answering
- [ ] 7.2 Build the index automatically if `.codeindex/graph.db` is missing when a query runs
- [ ] 7.3 Test: query after an edit reflects new content; query with no changes makes zero graph writes; query before any build triggers a build

## 8. Query engine + output (graph-queries)

- [ ] 8.1 Implement callers/callees traversals over `calls` edges; flag ambiguous results; disambiguate when the requested name matches multiple definitions
- [ ] 8.2 Implement definition/signature lookup (with `--context N` bounded lines) and "not found" handling
- [ ] 8.3 Implement dependencies/dependents over `imports`/`extends`/`implements`/`references` edges (blast radius)
- [ ] 8.4 Implement fuzzy symbol search (ranked) and file/module outline
- [ ] 8.5 Implement the output layer: default `path:line  signature` text and `--json` structured records (path, line, signature, edge kind, confidence)
- [ ] 8.6 Test each query command end-to-end against the fixture repo in both text and JSON modes

## 9. Verification

- [ ] 9.1 Integration test: build → query → edit a file → re-query, asserting incremental correctness end-to-end
- [ ] 9.2 Add a README documenting the CLI commands, the output contract, and the build toolchain (CGO note)
- [ ] 9.3 Run `openspec validate core-indexing-engine` and confirm the change is valid and complete
