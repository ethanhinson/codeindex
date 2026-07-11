## 1. Stage 1 — scope preference

- [x] 1.1 Schema v5 (bump + `edges.dst_ns` added now to avoid a second rebuild); PutFile derives tier-0 `symbols.namespace` by extension (Go dir / Py dotted / TS path / PHP dir-fallback)
- [x] 1.2 PHP adapter: capture `namespace X;` declaration → file namespace override
- [x] 1.3 resolve(tx, name, qualifier, srcFile, srcNS): insert same-file and same-namespace steps; ReResolveNames regroups per (name, qualifier, src_file)
- [x] 1.4 Tests: same-package collapse; scope-never-worse fallback; equivalence under rename across scopes
- [x] 1.5 Gate + metric: six-repo inc==full; ambiguity counts vs baselines recorded; decide Stage 2 scope from residue

## 2. Stage 2 — import binding (scoped by Stage-1 residue)

- [x] 2.1 Adapters persist import sources in dst_ns (TS specifier, Go path, Py module, PHP use-path); Go selector-alias calls emit ns hints
- [x] 2.2 Import-bound ladder step: binding lookup from the calling file's import edges; namespace mapping (TS relative→file, Go suffix→dir, Py dotted, PHP suffix); total fallback
- [x] 2.3 Tests: TS named-import binding; Go alias scoping; binding-never-worse; equivalence
- [x] 2.4 Gate + metric: six-repo inc==full; ambiguity counts vs Stage 1; full bench (query p95 within budget)

## 3. Close-out

- [x] 3.1 FINDINGS-import-resolution.md (staged table, spot-checks incl. laravel anchors unchanged); dashboard note if warranted
- [x] 3.2 All tests green; rebuild pinned binary; `openspec validate`; core-indexing-engine cross-references; commit + push
