## 1. Graph model + store

- [x] 1.1 `graph.RawDep{EnclosingIdx, Kind, Target, Line}` + `ParsedFile.Deps`; edge kinds `imports`/`extends`/`implements` constants
- [x] 1.2 PutFile inserts deps (src_symbol_id=0 for EnclosingIdx=-1; Go paths [contain '/'] stay unresolved verbatim; other targets resolve via existing resolver); affected-names bookkeeping includes dep targets
- [x] 1.3 Snapshot: src side becomes LEFT JOIN with `<file>` fallback; schemaVersion 2→3
- [x] 1.4 Store queries: `Dependents(name)` (kinds imports/extends/implements; exact dst_name OR last-path-segment for '/'-paths; src = symbol qname or file) and `DepsOf(anchor)` (file-mode: file's imports; symbol-mode: extends/implements + file imports labeled)
- [x] 1.5 Store tests: extends resolution, file-level edge lifecycle through per-file replace, dependents matching modes

## 2. Adapters

- [x] 2.1 Go: import_spec → file-level imports (verbatim path); struct embedded fields → extends; tests
- [x] 2.2 TS/JS: import_statement named+default specifiers → file-level imports; class_heritage extends/implements (class + interface); tests
- [x] 2.3 Python: import_statement / import_from_statement → file-level imports; class bases → extends; tests
- [x] 2.4 PHP: namespace `use` → imports; class extends/implements; in-class trait `use` → implements; tests

## 3. Query surface

- [x] 3.1 `internal/query`: DependentsText + DepsText (bounded, kinds labeled); ImpactText gains dependents section + updated coverage line
- [x] 3.2 CLI subcommands `dependents` and `deps`; MCP tools `dependents`/`deps` with anchor-rule descriptions; MCP test updated
- [x] 3.3 Tests: qualified + bare anchors; Go path matching both modes

## 4. Validation

- [x] 4.1 incremental==full on all six pinned repos (schema v3 rebuild); record index-size delta vs ≤2× bound
- [x] 4.2 Spot-checks recorded: laravel `dependents <a base class>`; this repo `dependents graph` + `dependents codeindex/internal/graph`; nest extends; flask import
- [x] 4.3 `/impact` output shows dependents on a real case; plugin note/README + FINDINGS updated; `openspec validate dependency-edges`; core-indexing-engine 8.3 marked satisfied
