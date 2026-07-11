## 1. Schema + store

- [x] 1.1 Add `symbols.parent` TEXT and `edges.dst_qualifier` TEXT; index `symbols(name, parent)`; `PRAGMA user_version = 2` with delete-and-rebuild on mismatch (stderr notice)
- [x] 1.2 `graph.Symbol.Parent`, `graph.RawCall.Qualifier`, `graph.Edge.DstQualifier`; PutFile persists both
- [x] 1.3 Resolver: `resolve(name, qualifier)` — qualified-first (1 hit → unambiguous; >1 → deterministic + ambiguous; 0 → plain fallback); ReResolve recomputes per (name, qualifier) pair
- [x] 1.4 Snapshot normalization includes parent + dst_qualifier + resolved-target parent
- [x] 1.5 Store tests: collision collapsed by qualifier; wrong hint falls back identically; re-resolution stability

## 2. Adapters (parent + qualifier, all lexical)

- [x] 2.1 Go: receiver type → method parent (strip `*`); receiver-variable calls → qualifier; tests incl. pointer receivers and no-qualifier plain calls
- [x] 2.2 TS/JS: enclosing class → method parent; `this.x()` → enclosing class qualifier; uppercase-identifier receiver → candidate qualifier; tests
- [x] 2.3 Python: enclosing class name (not just bool) threaded; `self.x()`/`cls.x()` → qualifier; uppercase-identifier receiver → candidate; tests
- [x] 2.4 PHP: enclosing class/interface/trait parent; `$this->x()` → qualifier; scoped calls `Foo::x()` → `Foo`, `self::`/`static::` → enclosing, `parent::` → none; tests

## 3. Query surface

- [x] 3.1 Qualified display: caller/callee names as `Parent.name`; def lines include qualified name
- [x] 3.2 Qualified anchors: parse `Type.method` / `Type::method` in callers/callees/impact (CLI + MCP); filter defs to parent and callers/callees to edges resolving into the filtered set; unqualified behavior unchanged
- [x] 3.3 Tests: qualified anchor returns only the matching parent's defs/callers; bare anchor unchanged; MCP integration test updated

## 4. Downstream parsers

- [x] 4.1 Plugin hook caller-file regex + note text tolerate qualified names; enclosing output unchanged check
- [x] 4.2 A/B grader (`grade_caller_attribution`) normalizes qualified names to final segment; unit fixtures updated

## 5. Re-test — engine + precision

- [x] 5.1 Re-run `codeindex bench` on gin, prometheus, kubernetes, nest, flask, laravel: incremental==full MUST pass everywhere with the new schema
- [x] 5.2 Precision metric: ambiguous-`calls`-edge counts before (v1 schema binary or archived numbers) vs after, per repo; record absolute + % reduction in `bench/engine/FINDINGS-resolution.md`
- [x] 5.3 Spot-check: `callers Builder.firstOrCreate` on laravel returns only Builder's def + its callers

## 6. Re-test — agent A/B v5 (laravel, plugin arm)

- [x] 6.1 Generate `tasks_v5.json`: laravel caller-attribution tasks from the new index (mix of unique-name targets and qualified-anchor targets), pre-registered expectation in header (branch-out ≥30% savings, success delta ≥ −5pp)
- [x] 6.2 Smoke (1–2 tasks), inspect transcripts, then full run (reps 2, budget $10)
- [x] 6.3 Grade + report + dashboard v5 section; record verdict and consequence

## 7. Close-out

- [x] 7.1 `go test ./...` green; gofmt clean; rebuild pinned binary
- [x] 7.2 Mark `core-indexing-engine` 2.1 parent portion satisfied; note the parent-TEXT deviation in `symbol-graph` spec basis; `openspec validate precise-resolution`
- [x] 7.3 Findings + READMEs updated (qualified anchors documented)
