<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0017 — Go subtype references carry namespace hints — fix the qualifier discard and KindImports Source](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0017-adapter-namespace-hints-extends-implements.md)**
<!-- docket:backlink:end -->

# Go subtype references carry namespace hints — design

Change 0017. Scope: **Go only** (owner ruling 2026-08-22, Option C). The
PHP/Python/TS aliased-import defect and the missing-EDGE parse gaps are change
0018's territory and are not touched here.

## Problem

Go is a total absence of namespace hints on subtype edges: `client_golang` has
**0** hinted subtype edges. Two adapter causes plus one store cause:

1. `internal/adapter/golang/golang.go` `import_spec` (~line 108) emits a
   `KindImports` dep with no `Source` — the Go adapter's local `addDep` closure
   (line 59) has no source field at all, and `graph.RawDep.Source` is therefore
   never populated for Go (line ~195 constructs `RawDep` without it).
2. `embeddedTypeName` (line 207) reduces `pkg.B` to `B` by descending into
   `qualified_type`'s `name` field, **discarding the package operand** — the only
   place the namespace was available.
3. `internal/graph/store.go:373` computes the subtype edge's hint as
   `hint := bind[d.Target]`, where `bind` is keyed on the *import dep's Target*.
   For Go, an import's `Target` is the full slash-bearing path, which never
   equals a bare embedded type name — so `bind` can never supply a hint for a Go
   subtype edge, and `d.Source` (the only other channel) is discarded.

Consequently the resolution ladder's import-mediated rung can never fire for Go
subtype edges. Measured on the `prometheus` bench member: 25 ambiguous qualified
embed edges, **23 addressable** (the other 2 are bare generic `T`).

## Acceptance bar — disambiguation, not resolution

Owner ruling round 2 (2026-08-22), binding and non-negotiable:

- `unresolved → resolved` is **provably impossible** for a hint-only change and
  **must not be promised**. All 96 unresolved Go subtype edges target names
  absent from the index; hints narrow candidates, they do not create symbols.
- The bar is **`ambiguous → unambiguous` with a verified-correct target**, over
  the 23 measured addressable qualified-embed edges.
- Pinned exemplars that must move: `chunkenc.Chunk` (currently resolves to the
  wrong package → must resolve to the correct one) and the 4 `refresh.Discovery`
  embeds (22 candidates → 1 correct). Note the `Discovery` ambiguous count is
  **4**, not 5.
- `storage.Appender` is recorded as **PARTIAL** — right package, still ambiguous
  (3 same-name symbols in-package) — and is **never claimable as a win**.
- **`dst_ns` movement alone counts for nothing.** A hint that appears on an edge
  without changing which symbol the edge resolves to is not evidence.

## Design — three edits, all required

Edit 3 is what makes edits 1 and 2 non-inert. Landing any subset is a no-op.

### Edit 1 — `import_spec` sets `Source`

`internal/adapter/golang/golang.go`:

- Extend the `addDep` closure (line 59) and its `rawDeps` anonymous struct with a
  `source string` field, and give `addDep` a `source` parameter. There are
  exactly **two** `addDep` call sites — `import_spec` (line 112) and
  `field_declaration` (line 130) — and both are changed by this design, so after
  the change no site passes `""` literally (the `field_declaration` site passes a
  value that is `""` when the operand has no alias entry).
- At `import_spec`, call `addDep(n, graph.KindImports, ipath, ipath)`.
- At the `RawDep` construction (~line 195), carry `Source: d.source`.

### Edit 2 — `embeddedTypeName` stops discarding the qualifier

- Change `embeddedTypeName` to return **two** values: the bare type name and the
  package operand of a `qualified_type` (empty when there is none). The pointer,
  generic, and type_identifier legs are unchanged; the `qualified_type` leg
  captures `ChildByFieldName("package")` before descending into `name`.
- At `field_declaration` (line 128), resolve the captured operand through the
  existing `aliases` map (`aliases[pkgOperand] -> import path`) and pass the
  resolved import path as the dep's `source`. An operand with no alias entry
  (dot-imports, `_`, or an unknown identifier) yields `source == ""` — exactly
  today's behavior.
- **`Target` must stay the bare type name** (`B`, never `pkg.B`). It is what the
  plain resolution rungs query and what `DumpNormalized` selects; widening it
  would break both.

**Why an import-path hint matches a Go namespace at all.** `DeriveNamespace`
(`internal/graph/types.go:48`) gives a Go file its **repo-relative directory** as
its namespace, so the hint `github.com/prometheus/prometheus/tsdb/chunkenc` never
equals the candidate namespace `tsdb/chunkenc`. It matches because `nsMatch`
(`store.go:1130`) does a suffix comparison — `strings.HasSuffix(hint, "/"+candNS)`.
This is the load-bearing fact that makes the `chunkenc.Chunk` exemplar predictable
and it is why the raw import path is the right thing to store as `Source`.

**The alias default is the last path segment, not the Go package name.** The
existing `import_spec` code registers `aliases[lastSegment] = ipath` when there is
no explicit alias. For imports whose final segment differs from the declared
package name — `gopkg.in/yaml.v2` registers `yaml.v2` while source writes
`yaml.MapSlice`, and `go-foo`-style segments behave the same — the operand lookup
misses and `Source` is `""`, i.e. exactly today's behavior. This is **not a
regression**, but it does shrink the population a hint can reach. It does not
threaten the acceptance bar: the pinned exemplars (`chunkenc`, `refresh`,
`storage`) all have segment == package name. Widening the alias default to the
real package name would require reading the imported package's own
`package` clause, which is cross-file work this change does not take on.

`aliases` is populated at `import_spec` during the same walk, and Go requires
imports to precede declarations, so the map is complete by the time any
`field_declaration` is visited. This is the same mechanism the Go *call* path
already uses (`aliases[name]` at line 160 feeding `c.nsHint`).

### Edit 3 — `store.go:373` prefers the edge-local `Source`

Replace `hint := bind[d.Target]` with the calls-path shape at lines 352–354:

```
hint := normalizeHint(d.Source, d.Target, pf.Path) // edge-local source wins
if hint == "" {
    hint = bind[d.Target] // file-level import binding, as before
}
```

Precedence matches the calls path deliberately: an edge-local `Source` is
strictly more specific than a file-level binding, and having the two sites
disagree is the drift shape the learnings ledger's
`one-invariant-many-sites-drifts` finding warns about.

## Why edit 3 is near-inert for existing edges

- For **import** deps, `normalizeHint(d.Source, d.Target, pf.Path)` is *by
  construction* the same expression that populated `bind[d.Target]` at line 343,
  so the hint is unchanged.
- For every **non-import** dep in the other three adapters, `Source` is `""` —
  verified: all five `Source:` assignments across the four adapters are on
  `KindImports` DepSites (`python.go:92,97`, `php.go:102`, `tsjs.go:115,121`).
  Those edges fall through to the `bind` fallback unchanged.
- The **one behavior delta**: two imports in a file sharing a `Target` with
  different `Source`s. `bind`'s last-write-wins currently gives both edges the
  second hint; after this change each import edge gets its own. This is a strict
  correctness improvement and gets a test.

## Named risk — edit 1 newly populates `bind` for Go files

Before this change, Go's `bind` map is always empty (no `Source` anywhere). After
edit 1, a Go file's `bind` gains one entry per import, keyed on the full import
path. Two consequences to verify rather than assume:

- **Go import edges themselves** now carry `dst_ns`. For a single-segment import
  (`"fmt"`, `"errors"`) `d.Target` contains no `/`, so `store.go` *does* call
  `resolve(...)` on it, now with a non-empty hint. Verify no import edge changes
  which symbol it resolves to.
- **Go calls** at line 353 fall back to `bind[c.Callee]`. This collision surface
  is **wider than "a function named like an import path"** and must be treated as
  a real shape, not a curiosity:
  - `calleeName` also yields **selector/method** names, so `x.log()`, `p.path()`,
    `t.context()`, `e.errors()` inside a file importing `"log"`, `"path"`,
    `"context"`, or `"errors"` all collide. Lowercase single-segment stdlib
    import paths are exactly the shape of unexported Go method names.
  - `nsMatch` widens it again: candidate namespace `internal/log` matches hint
    `log` via `HasSuffix(candNS, "/"+hint)`, so no exact namespace equality is
    needed.
    - Qualified calls are unaffected — `c.NsHint` already wins at line 352.
  - **Severity note:** in `resolve` (`store.go:1052`) the `nsHint != ""`
    `boundIDs` steps run *before* the `srcNS` same-scope step and before the
    plain-name steps. A newly non-empty hint therefore **preempts** rungs that
    currently decide these edges — it does not merely narrow them. This is why
    the collision needs a test rather than an argument.
- The **import-edge leg** has the mirror shape: for `import "log"`, `resolve` is
  now hint-first and `boundIDs` will match any symbol named `log` in any
  namespace ending `/log`.

**There is no snapshot guard for either risk — this must be tested, not
assumed.** The repo contains **no graph goldens**: `git ls-files | grep -iE
'golden|snapshot|\.snap'` returns nothing. Every `DumpNormalized` consumer
(`internal/engine/engine_test.go:34`, `cmd/codeindex/main.go:530`) is an
*incremental-vs-full-rebuild equivalence* check over the same tree under the same
code, so both sides move identically under this change and neither side can
detect a resolution regression. The three `*TextGolden` tests in
`internal/query/query_test.go` are inline CLI-output pins, not graph snapshots.
Separately, `DumpNormalized` (`store.go:1196`) selects `confidence` and the
resolved dst (`file`/`parent`/`name`/`start_line`) but never `dst_ns`, so it is
blind to hints in any case. Both named risks therefore ship unguarded unless the
verification plan adds store-level tests for them — which it does (items 6a/6b).

## Verification plan

Adapter-level (`internal/adapter/golang`):

1. Import dep carries `Source == ipath`, for both implicit and explicit-alias
   imports; `_` and `.` imports remain excluded from `aliases`.
2. A qualified embed `chunkenc.Chunk` emits a `KindExtends` dep with
   `Target == "Chunk"` and `Source == "<resolved import path>"`.
3. An unqualified embed (`B`) and a generic embed (`G[T]`) still emit `Target`
   bare with `Source == ""`. Note that a bare pointer embed `*B` does **not**
   exercise `embeddedTypeName`'s `pointer_type` leg — tree-sitter-go parses the
   embedded field's `type` as a plain `type_identifier` with a sibling `*`, so
   that leg is dead code on this path. The pointer case worth testing is the
   **qualified** pointer embed `*al.Thing`, whose `type` *is* a `qualified_type`
   and which must therefore carry `Source` (a real prometheus shape).
4. An embed whose operand has no alias entry yields `Source == ""` — cover both
   the dot/`_` import case and the segment≠package-name case
   (`gopkg.in/yaml.v2` + `yaml.MapSlice`), asserting today's behavior is
   preserved.

Store-level (`internal/graph`):

5. **The disambiguation test (the tooth):** two symbols named `Chunk` in
   different namespaces, plus a Go file importing one of them and embedding
   `chunkenc.Chunk`. Assert the edge resolves to the *correct* symbol and is
   unambiguous. Without this, the change is untested — the goldens cannot see it.
   Same-shaped negative: without the import, the edge stays ambiguous.
6. The last-write-wins delta: two imports sharing a `Target` with different
   `Source`s get their own per-edge hints.

6a. **Call-collision regression test** (replaces the absent golden guard): a Go
    file importing `"log"` that calls a method `x.log()`, with a symbol named
    `log` present in a namespace ending `/log`. Assert the call edge resolves the
    same as it does before this change — i.e. the newly populated `bind` does not
    capture it. If it does capture it, that is a real regression this change must
    fix, and the remedy is **to stop populating `bind` from Go import deps at
    all**. Do not try to filter `bind` by whether the target looks like a path:
    the colliding entry (`import "log"` → `Target == "log"`) is itself a
    non-path target, so a non-path filter would preserve exactly the wrong
    entries. `bind[d.Target]` is dead for Go on the subtype path by cause #3 in
    the Problem section, and Edit 3 makes the edge-local `Source` the live
    channel, so Go gains nothing from `bind` in the first place.

6b. **Import-edge regression test:** a single-segment import (`"log"`) with a
    same-named symbol in a namespace ending `/log`. Assert the import edge's
    resolved dst is unchanged from pre-change behavior.
7. **Characterization test for the known limitation** (per the ledger's
   `known-limitations-need-a-characterization-test`): `storage.Appender`'s shape
   — correct package, still ambiguous among 3 in-package same-name symbols —
   asserted as what the code *does*, with a comment naming in-package
   disambiguation as the prerequisite that would change it. Prose alone gets
   "fixed" into a regression.

Suite and no-regression evidence:

8. `go test -tags nollama -count=1 ./...` green. This is the pinned command
   (`FINALIZE_TEST_COMMAND`); plain `go test ./...` fails 10 packages for
   environmental reasons and is not the gate.
9. **The "single-repo goldens byte-identical" claim in the change body refers to
   an out-of-band manual diff, not to a committed artifact** — the repo has no
   graph goldens (see the named-risk section). Reproduce it explicitly rather than
   trusting the phrase: on a single-repo bench member (`bench/repos/prometheus`),
   build the index at `origin/main` and at the change branch into two separate
   `.codeindex/graph.db` files, and diff a `DumpNormalized`-shaped dump of each.
   **The rule is NOT "the diff must be empty."** `DumpNormalized`'s edge string
   embeds the confidence token and the resolved dst, so every disambiguation win
   item 10 demands — including the `chunkenc.Chunk` wrong-package→correct-package
   flip — necessarily shows up as a diff line. An empty-diff stop-rule would fire
   on success. The rule: **every diff line must be accounted for by a row in item
   10's before/after table; any line outside that table is a
   stop-and-investigate.** This is manual
   evidence recorded in the results/findings write-up; the *in-suite* guards for
   the two named risks are items 6a and 6b, which is where the regression
   protection actually lives.

Bench measurement (the acceptance bar):

10. Rebuild the `prometheus` bench member index before and after, and emit a
    per-edge before/after table over the 23 addressable qualified-embed edges:
    edge, candidate count, chosen target, confidence. Record it in
    `bench/engine/FINDINGS-workspace-graph.md`. PASS requires the pinned
    exemplars moving `ambiguous → unambiguous` with verified-correct targets;
    `storage.Appender` is logged PARTIAL and excluded from the win count.

## Out of scope

- Resolver / ladder changes. Edit 3 is insert-time hint *selection*, not the
  ladder; the owner has admitted it as unavoidable (it is the edit that makes the
  other two non-inert).
- Aliased-import resolution for PHP/Python/TS — change 0018.
- The missing-EDGE parse gaps (Python `class X(mod.Y)`, TS `extends ns.Foo`, PHP
  group `use`) — change 0018's territory.
- Go **interface** embedding, which today emits **no dep at all** — verified
  against the pinned grammar, an interface embedding parses as
  `interface_type > type_elem > qualified_type`, never `field_declaration`, and
  parsing `interface { chunkenc.Iface; io.Reader }` produces zero deps. Combined
  with the fact that `golang.go` emits no `KindImplements` anywhere, this change
  — despite the stub's "extends-implements" title — adds **no implements
  coverage**, and Go interface-embedding coverage stays at zero rather than
  "partial." The entire measured 25-edge population comes from struct embedding
  through the single `addDep` site at line 130. Adding a `type_elem` emit site is
  a missing-EDGE change and belongs in its own change, not here — it would also
  move the denominator the acceptance bar is stated over.
- Widening `DumpNormalized` to select `dst_ns`. Tempting, but it would rewrite
  every golden and destroy the byte-identical guard this change depends on.
- Workspace query surfaces (killed 0016) and corpus growth (change 0010).

## Assumptions

Every decision below was defaulted autonomously; the owner's two rulings and the
change file's `### Design already established` are inputs, not assumptions.

1. **Where the qualifier is resolved to an import path.** Chosen: capture the
   operand in `embeddedTypeName` (returning two values) and resolve it through
   `aliases` at the `field_declaration` call site. Rejected: resolving inside
   `embeddedTypeName` by passing it the `aliases` map — couples a pure string
   reducer to walk state and makes it harder to test. Rejected: returning
   `pkg.B` as `Target` — explicitly forbidden by the established design (breaks
   the plain rungs and `DumpNormalized`).
2. **How `addDep` grows a source channel.** Chosen: add a `source string` field
   to the existing anonymous struct and a `source` parameter to the closure.
   There are only two call sites and both are edited. Rejected: a second
   `addDepSrc` closure (two
   near-identical append paths is the drift shape); rejected: promoting the
   anonymous struct to a named type — a real readability improvement but a
   refactor this change does not need, and the compiler catches any mismatch in
   the two-site anonymous spelling.
3. **Hint precedence at `store.go:373`.** Chosen: edge-local `Source` wins,
   `bind[d.Target]` is the fallback. Rejected: `bind` first — it would make the
   subtype site disagree with the calls site at 352–354 for no benefit, and the
   file-level binding is the less specific fact.
4. **`normalizeHint` is reused, not re-implemented.** Its `./`-relative and
   backslash-suffix legs are inert for Go import paths, so passing Go's `Source`
   through it is a pass-through; reusing it keeps the two hint sites identical.
   The pass-through hint is *usable* — not merely safe — because of the
   `DeriveNamespace` / `nsMatch` suffix relationship documented above.
5. **Hint misses are wider than dot/`_` imports and are all accepted as
   no-ops.** Excluded from `aliases`: dot and `_` imports. Missed by the
   last-segment alias default: any import whose final path segment differs from
   its declared package name. All of them fall through to `Source == ""`, i.e.
   today's behavior. Rejected: special-casing dot-imports (no measured
   population, needs file-scope name injection); rejected: resolving the real
   package name (cross-file work). The pinned exemplars are unaffected, so the
   acceptance bar stands.
6. **The disambiguation check is a new store-level test, not a widened golden.**
   Rejected: adding `dst_ns` to `DumpNormalized` (see Out of scope).
7. **The acceptance measurement is a recorded before/after table, not an
   automated gate.** It runs against a bench member index, not in the unit suite;
   `FINDINGS-workspace-graph.md` is the existing home for exactly this record.
   Because the repo has no graph goldens, this measurement is the only evidence
   of the *gain* — and it is deliberately paired with in-suite tests 5, 6a and
   6b, which carry the *no-regression* half rather than leaving it to a snapshot
   that does not exist.
8. **Dependency state:** `depends_on` is empty; `related: [13, 10, 18]` are
   informational. Nothing blocks the build.
9. **Interface-embedding emit sites are left alone, and coverage there is zero,
   not partial** (see Out of scope). The measured population flows entirely
   through `field_declaration` struct embedding; interface embedding parses as
   `type_elem` and emits nothing, and no `KindImplements` dep is emitted
   anywhere in the Go adapter. Expanding emit coverage would change the
   denominator the acceptance bar is stated over, so it is a separate change.
   The stub's title overstates the delivered scope; the change body is corrected
   to say so.
10. **The stub's tier-1 motivation is struck, not designed around.** `AttachMap`
    (`internal/graph/depmaps.go:81`) attaches by an operator-supplied map path;
    hints only narrow *within* already-attached tier-1 rows via `boundIDs`, and
    both bench indexes have zero tier-1 symbols. The change body is corrected
    accordingly.
11. **Critic gate record.** The adversarial critic returned `wrong but fixable
    from available context` on the first pass (call-site miscount; missing
    `DeriveNamespace`/`nsMatch` justification; the segment≠package-name miss
    class; the non-existent graph goldens; interface-embedding coverage stated as
    partial when it is zero; an understated `bind[c.Callee]` collision surface; a
    vacuous pointer-embed test). All were folded in. The bounded re-check leg
    marked five items `sound` and returned two further `wrong but fixable`
    findings *with its own prescribed end-state*: verification item 9's
    empty-diff stop-rule contradicted the acceptance bar, and item 6a's remedy
    parenthetical was inverted. Both were applied exactly as the critic
    prescribed — adopting the adversary's stated correction, not a new design
    round. No `needs human context` verdict was returned at any point.
12. **Critic's verified corrections are treated as fact, not re-derived:** the
    `scrape/target.go` edges resolve today to `scrape/helpers_test.go` (same-scope
    srcNS rung), **not** `cmd/prometheus/main.go`; the `Discovery` ambiguous count
    is 4; 23 of the 25 ambiguous edges are addressable (2 are bare generic `T`).
