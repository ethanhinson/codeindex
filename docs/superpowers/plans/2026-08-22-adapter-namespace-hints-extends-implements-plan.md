<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0017 — Go subtype references carry namespace hints — fix the qualifier discard and KindImports Source](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0017-adapter-namespace-hints-extends-implements.md)**
<!-- docket:backlink:end -->

# Go subtype references carry namespace hints — implementation plan

Change 0017. Spec:
`docs/superpowers/specs/2026-08-22-adapter-namespace-hints-extends-implements-design.md`
(on `origin/docket`). Reconciled 2026-08-22 against `origin/main` @ `2c8b9c3` —
every line reference in the spec still resolves exactly.

> **Plan-skill degradation.** `superpowers:writing-plans` is not invocable on this
> machine. Per the docket convention's *Skill layer* missing-skill rule, the plan
> role degraded to `auto` and this file was authored by `docket-implement-next`
> directly. Artifact and stop-point are unchanged.

## The one-paragraph statement of the work

Go subtype (struct-embedding) edges carry **no namespace hint**, so the resolution
ladder's import-mediated rung can never fire for them. Three edits fix it: the Go
adapter's `import_spec` starts setting `RawDep.Source`; `embeddedTypeName` stops
discarding the `qualified_type` package operand so the embed site can resolve it
through `aliases` into an import path and set that as the dep's `Source`; and
`store.go:373` starts preferring that edge-local `Source` over the file-level
`bind` map, mirroring the calls path at 352–354. Edit 3 is what makes edits 1 and 2
non-inert — landing any subset is a no-op.

## Binding constraints (read before writing any code)

These are the failure modes, restated because each has already been argued once.

1. **Acceptance is the DISAMBIGUATION bar**, over the 23 measured addressable
   qualified-embed edges on the `prometheus` bench member: `ambiguous →
   unambiguous` **with a verified-correct target**. Never promise `unresolved →
   resolved` — all 96 unresolved Go subtype edges target names absent from the
   index, and hints narrow candidates, they never create symbols. **`dst_ns`
   movement alone counts for nothing.**
2. **Pinned exemplars that must move:** `chunkenc.Chunk` (wrong package → correct
   package) and the **4** (not 5) `refresh.Discovery` embeds (22 candidates → 1
   correct). **`storage.Appender` is PARTIAL by record** — right package, still
   ambiguous among 3 same-name in-package symbols — and is **never claimable as a
   win**; it gets a characterization test instead (task 7).
3. **`Target` stays the bare type name** (`B`, never `pkg.B`). It is what the plain
   rungs query and what `DumpNormalized` selects. Only `Source` gets the qualifier.
4. **There are no committed graph goldens.** Verified: `git ls-files | grep -iE
   'golden|snapshot|\.snap'` is empty. Every `DumpNormalized` consumer is an
   incremental-vs-full **rebuild-equivalence** check under the same code, so both
   sides move identically and neither can see a resolution regression; the three
   `*TextGolden` tests in `internal/query/query_test.go` are inline CLI-output pins,
   not graph snapshots. The in-suite no-regression protection is therefore **tasks
   6a and 6b and nothing else** — do not lean on a snapshot that does not exist.
5. **Hint rungs PREEMPT the srcNS same-scope rung.** In `resolve`
   (`internal/graph/store.go:1052`) the `nsHint != ""` `boundIDs` steps run
   *before* the `srcNS` step and before the plain-name steps. A newly non-empty
   hint therefore changes which rung decides an edge — it does not merely narrow
   one. This is why tasks 6a/6b are tests and not arguments.
6. **The alias-default miss class is accepted, not fixed.** `import_spec` registers
   `aliases[lastPathSegment] = ipath`. Any import whose final segment differs from
   its declared package name (`gopkg.in/yaml.v2` registers `yaml.v2` while source
   writes `yaml.MapSlice`) misses the lookup and yields `Source == ""` — exactly
   today's behavior, **not a regression**. Resolving the real package name is
   cross-file work this change does not take on. Task 4 asserts the miss.
7. **Interface embedding / `implements` is OUT of scope with zero current
   coverage.** Go emits no `KindImplements` dep anywhere, and interface embedding
   parses as `interface_type > type_elem > qualified_type` — never
   `field_declaration` — so it emits no dep at all. Do not add a `type_elem` emit
   site: that is a missing-EDGE change and it would move the denominator the
   acceptance bar is stated over. Despite the change's title, this delivers **no**
   implements coverage.
8. **Do not widen `DumpNormalized` to select `dst_ns`.** Explicitly out of scope.
9. **Suite:** `go test -tags nollama -count=1 ./...` (the pinned
   `FINALIZE_TEST_COMMAND`), green on `origin/main`. Plain `go test ./...` fails 10
   packages for environmental reasons and is **not** the gate.

## Why an import-path hint matches a Go namespace at all

`DeriveNamespace` (`internal/graph/types.go`) gives a Go file its **repo-relative
directory** as its namespace, so the hint
`github.com/prometheus/prometheus/tsdb/chunkenc` never *equals* the candidate
namespace `tsdb/chunkenc`. It matches because `nsMatch` (`store.go:1130`) does a
**suffix** comparison in either direction. This is the load-bearing fact that makes
the `chunkenc.Chunk` exemplar predictable, and it is why the raw import path is the
right thing to store in `Source`.

---

## Tasks

Each task: focused test first, then implementation, then verification, then one
commit. Tasks 1–3 are the code; 4–7 are the coverage the spec's verification plan
requires; 8–10 are the gate and the evidence.

### Task 1 — `import_spec` sets `Source` (Edit 1)

**File:** `internal/adapter/golang/golang.go`

- Extend the `rawDeps` anonymous struct (line 53) and the `addDep` closure
  (line 59) with a `source string` field/parameter. There are **exactly two**
  `addDep` call sites — `import_spec` (line 112) and `field_declaration`
  (line 130) — and both are edited by this plan, so after the change no site
  passes a bare literal `""` (the `field_declaration` site passes a value that is
  `""` when the operand has no alias entry).
- At `import_spec`: `addDep(n, graph.KindImports, ipath, ipath)`.
- At the `RawDep` construction (~line 195): carry `Source: d.source`.
- Leave the `aliases` population untouched, including the `_` and `.` exclusions.

**Test (in `internal/adapter/golang/golang_test.go`)** — spec item 1: an import dep
carries `Source == ipath` for both an implicit import and an explicit-alias import;
`_` and `.` imports remain excluded from `aliases` (assert via the absence of a
hint on a dependent embed, or directly if a seam exists).

### Task 2 — `embeddedTypeName` stops discarding the qualifier (Edit 2)

**File:** `internal/adapter/golang/golang.go`

- Change `embeddedTypeName` (line 207) to return **two** values: the bare type name
  and the `qualified_type` package operand (empty when there is none). The
  `pointer_type`, `generic_type`, and `type_identifier` legs are unchanged; the
  `qualified_type` leg captures `ChildByFieldName("package")` **before** descending
  into `name`.
- At `field_declaration` (line 125): resolve the captured operand through the
  existing `aliases` map and pass the resolved import path as the dep's `source`.
  An operand with no alias entry yields `source == ""`.
- `aliases` is complete by the time any `field_declaration` is visited — it is
  populated at `import_spec` in the same walk and Go requires imports to precede
  declarations. This is the same mechanism the Go *call* path already uses
  (`aliases[name]` at line 160 feeding `c.nsHint`).
- **Keep `Target` bare** (constraint 3).

**Tests** — spec items 2 and 3:
- A qualified embed `chunkenc.Chunk` emits a `KindExtends` dep with `Target ==
  "Chunk"` and `Source == "<resolved import path>"`.
- An unqualified embed `B` and a generic embed `G[T]` still emit `Target` bare with
  `Source == ""`.
- **The pointer test must be the QUALIFIED pointer embed `*al.Thing`** (a real
  prometheus shape), whose `type` *is* a `qualified_type` and which must therefore
  carry `Source`. A bare `*B` does **not** exercise the `pointer_type` leg at all —
  tree-sitter-go parses the embedded field's `type` as a plain `type_identifier`
  with a sibling `*`, so that leg is dead on this path and a bare-`*B` test would
  be vacuous.

### Task 3 — `store.go:373` prefers the edge-local `Source` (Edit 3)

**File:** `internal/graph/store.go`

Replace `hint := bind[d.Target] // extends/implements/import targets bind too` with
the calls-path shape from lines 352–354:

```go
hint := normalizeHint(d.Source, d.Target, pf.Path) // edge-local source wins
if hint == "" {
    hint = bind[d.Target] // file-level import binding, as before
}
```

Precedence matches the calls path **deliberately**: an edge-local `Source` is
strictly more specific than a file-level binding, and having the two sites disagree
is precisely the drift shape the learnings ledger's `one-invariant-many-sites-drifts`
finding warns about. Keep the two sites' doc comments consistent with each other —
that finding's cheap tell is that the sites' comments start arguing.

`normalizeHint` is **reused, not re-implemented**: its `./`-relative and
backslash-suffix legs are inert for Go import paths, so this is a pass-through.

Near-inert for existing edges: for import deps `normalizeHint(d.Source, d.Target,
pf.Path)` is by construction the same expression that populated `bind[d.Target]` at
line 343; for every non-import dep in the other three adapters `Source` is `""`
(all five `Source:` assignments across the four adapters are on `KindImports`
DepSites — `python.go:92,97`, `php.go:102`, `tsjs.go:115,121`).

### Task 4 — Adapter test: the hint-miss classes (spec item 4)

Assert **today's behavior is preserved** for both miss classes:
- The dot-import / `_`-import case (operand has no `aliases` entry) → `Source == ""`.
- **The segment ≠ package-name case**: `import "gopkg.in/yaml.v2"` with an embed of
  `yaml.MapSlice` → `Source == ""`, because `aliases` holds the key `yaml.v2`, not
  `yaml`. Comment this as the accepted miss class of constraint 6, not a bug.

### Task 5 — Store test: the disambiguation test (THE TOOTH) (spec item 5)

`internal/graph` (new test file or `store_test.go` — the package already has the
`putFile` helper and `Open` scaffolding used by `openexisting_test.go` and
`wsreaders_test.go`).

Two symbols named `Chunk` in **different** namespaces, plus a Go `ParsedFile` that
imports one of them and carries a `KindExtends` dep with `Target: "Chunk"` and
`Source: "<the import path of the correct one>"`. Assert the edge resolves to the
**correct** symbol and its confidence is `unambiguous`.

**Same-shaped negative:** the identical setup with no `Source` → the edge stays
`ambiguous`. Without both halves the change is untested — constraint 4 means the
goldens genuinely cannot see it.

### Task 6 — Store test: the last-write-wins delta (spec item 6)

Two imports in one file sharing a `Target` with **different** `Source`s. Today
`bind`'s last-write-wins gives both edges the second hint; after Edit 3 each import
edge gets its own. Assert the per-edge hints. This is a strict correctness
improvement and is the one intended behavior delta.

### Task 6a — Store test: the call-collision regression (spec item 6a)

This **replaces the absent golden guard** — see constraint 4.

Edit 1 newly populates `bind` for Go files, one entry per import keyed on the full
import path. Go calls at line 353 fall back to `bind[c.Callee]`, and the collision
surface is **wider than "a function named like an import path"**: `calleeName` also
yields selector/method names, so `x.log()`, `p.path()`, `t.context()` inside a file
importing `"log"`, `"path"`, `"context"` all collide — lowercase single-segment
stdlib import paths are exactly the shape of unexported Go method names. `nsMatch`
widens it again (candidate namespace `internal/log` matches hint `log` by suffix).
Per constraint 5 a newly non-empty hint **preempts** the rungs that decide these
edges today.

**Test:** a Go file importing `"log"` that calls a method `x.log()`, with a symbol
named `log` present in a namespace ending `/log`. Assert the call edge resolves the
**same as before this change** — the newly populated `bind` must not capture it.

**If it does capture it**, that is a real regression this change must fix, and the
remedy is **to STOP populating `bind` from Go import deps at all**. Do **not** try
to filter `bind` by whether the target looks like a path: the colliding entry
(`import "log"` → `Target == "log"`) is itself a non-path target, so a non-path
filter would preserve exactly the wrong entries. `bind[d.Target]` is dead for Go on
the subtype path anyway (an import's `Target` is the full slash-bearing path, which
never equals a bare embedded type name) and Edit 3 makes the edge-local `Source`
the live channel — so Go gains nothing from `bind` in the first place.

*(This paragraph is the critic's corrected remedy. An earlier draft had it
inverted; the corrected version above is the binding one.)*

### Task 6b — Store test: the import-edge regression (spec item 6b)

The mirror shape of 6a. A single-segment import (`"log"`) — `d.Target` contains no
`/`, so `store.go` *does* call `resolve(...)` on it, now with a non-empty hint —
with a same-named symbol in a namespace ending `/log`. Assert the import edge's
resolved dst is **unchanged** from pre-change behavior.

### Task 7 — Characterization test for `storage.Appender` (spec item 7)

Per the ledger's `known-limitations-need-a-characterization-test`: assert what the
code **does** — correct package, still `ambiguous` among 3 in-package same-name
symbols — named so it cannot be misread as an aspiration (the ledger's shape is
`TestKNOWNLIMITATION<Behavior>`). Include the **prerequisite** in the comment:
in-package disambiguation is the thing that must land first, and closing this
without it is worse than the gap. Prose alone gets "fixed" into a regression.

### Task 8 — Suite gate (spec item 8)

`go test -tags nollama -count=1 ./...` green — **zero new failures** against
`origin/main`. Run foreground.

### Task 9 — Rebuild-diff evidence, every line accounted for (spec item 9)

On the `bench/repos/prometheus` member, build the index at `origin/main` and at the
change branch into two separate `.codeindex/graph.db` files and diff a
`DumpNormalized`-shaped dump of each.

**The rule is NOT "the diff must be empty."** `DumpNormalized`'s edge string embeds
the confidence token and the resolved dst, so every disambiguation win task 10
demands — including the `chunkenc.Chunk` wrong-package→correct-package flip —
**necessarily** shows up as a diff line. An empty-diff stop-rule would fire on
success. **The rule: every diff line must be accounted for by a row in task 10's
before/after table; any line outside that table is a stop-and-investigate.**

This is manual evidence for the write-up. The *in-suite* regression protection is
tasks 6a/6b (constraint 4).

### Task 10 — The acceptance measurement (the bar)

Rebuild the `prometheus` bench member index before and after and emit a per-edge
before/after table over the **23 addressable** qualified-embed edges (of 25
ambiguous; the other 2 are bare generic `T`): edge, candidate count, chosen target,
confidence. Record it in `bench/engine/FINDINGS-workspace-graph.md`.

**PASS requires the pinned exemplars moving `ambiguous → unambiguous` with
verified-correct targets** — `chunkenc.Chunk` and the 4 `refresh.Discovery` embeds.
`storage.Appender` is logged **PARTIAL** and excluded from the win count.

Verified fact carried from the critic, treated as fact and not re-derived: the
`scrape/target.go` edges resolve today to `scrape/helpers_test.go` (the same-scope
srcNS rung), **not** to `cmd/prometheus/main.go`.

## Definition of done

- Three edits landed; `Target` still bare everywhere.
- Adapter tests 1–4 and store tests 5, 6, 6a, 6b, 7 present and green.
- `go test -tags nollama -count=1 ./...` green, zero new failures.
- Task 9's diff has every line accounted for by task 10's table.
- Task 10's table recorded in `bench/engine/FINDINGS-workspace-graph.md`, with the
  pinned exemplars moved and `storage.Appender` logged PARTIAL.
