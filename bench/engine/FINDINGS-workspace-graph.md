# FINDINGS — workspace-graph D7 evidence gate

## 2026-08-21: pre-registered gate run — control matched the index (frontier model)

**Verdict: GATE FAIL at the frontier model. The grep-across control matched
the workspace index on recall and the efficiency bar was missed by a wide
margin.** Per the pre-registration (bench/workspace/README.md, bars verbatim
from design D7), this is a legitimate answer to the frontier hypothesis.

Setup: frozen 65-task rung-1 corpus (10 OSS members, 4 languages);
arm A = agent + shell with all member checkouts; arm B = A + workspace MCP
(branch binary from PR #14, `feat/workspace-query-surfaces-gated` @ 0d71cd2,
overlay fully re-resolved post-fix with stamps cleared). Model: claude CLI
default (frontier tier). Leak audit: PASS all four classes
(template_leakage, control_contamination id-paired 65/65, forced_tool,
grader_codesign) before grading.

Numbers (grades.jsonl, 132 grades):

| metric | arm A | arm B | bar | result |
|---|---|---|---|---|
| med cross-recall | 1.0 | 1.0 | B ≥ A | tie (not a win) |
| mean cross-recall | 0.940 | 0.924 | — | paired: B wins 2 / loses 2 / ties 61 |
| rung-1 med cross-recall | 1.0 | 1.0 | ≥ 0.9 absolute | pass |
| med precision | 1.0 | 1.0 | — | index answers were correct |
| med tokens | 21,238 | 21,661 | B ≥40% fewer | **fail** (3.4% median paired saving) |
| med tool calls | 8 | 7 | B ≥40% fewer | **fail** (median paired delta 0) |
| cost | $23.84 | $23.25 | — | — |

Interpretation: every corpus task is import-mediated — the symbol name is
literally greppable across checkouts — and a frontier-class agent with grep
is already at ceiling on that shape. The index was *correct* (precision 1.0,
the ws-xcallers-HTTPException-036 GT verified exactly) but saved ~nothing.
This mirrors the m5 finding: the MCP index lifted every model below the
frontier by +32–68pp while the frontier model was at ceiling.

Residuals (recorded, not excuses):
- Corpus is 100% rung-1 by construction (no organic rung-2 edges existed in
  the OSS corpus at freeze); grep-hostile shapes (indirect impact chains,
  cross-repo name collisions) are unrepresented. Change 0010 (corpus
  growth) owns widening this.
- Arm B pays a whole-manifest freshen on every MCP query entry (all 10
  members) — a fixed latency/token tax the reviewer pre-flagged.
- The `member:Symbol` anchor prefix works over MCP but is unadvertised in
  tool descriptions (owner ruling froze the tool surface); arm B agents
  may not discover the cheapest query form.
- Rung 2 produced 37,287 inferred edges on this corpus (vs 4,445 exact),
  including cross-language bare-name junk; it did not hurt measured
  precision, but it is the frozen rule's cost on a 10-member corpus.
- Engine-level fix landed mid-campaign, pre-registration intact (fixed
  before any scored run): merged 0013 `UnresolvedEdges` dropped
  module-scope (src_symbol_id=0) import edges — the entire import-mediated
  signal; imports/exact went 6 → 2386 after the fix. Scored runs used the
  fixed binary only.

Disposition of change 0016 / PR #14: held open by owner decision
2026-08-21 pending the registered small-model follow-up below — the owner
judges multi-repo critical to functionality and the m5 floor pattern
predicts the index pays below the frontier. The frontier-model verdict
above stands regardless of the follow-up's outcome.

## 2026-08-21: registered follow-up — adoption-floor sweep (small models)

**Registered BEFORE any scored small-model run.** Hypothesis (from the m5
floor sweep): the workspace index lifts small/cheap models on cross-repo
tasks even where the frontier model is at ceiling.

Setup: same frozen 65-task corpus, same arms, same harness, same leak-audit
pre-verdict gate; `--model claude-haiku-4-5-20251001` (floor tier). A
mid-tier point (sonnet) MAY be added with the same bars; each model is
judged independently.

Bars (all required, per tested model):
- Recall lift: B median cross-recall ≥ A + 10pp, OR B meets the original
  efficiency bar (≥40% fewer tokens or tool calls) with recall B ≥ A.
- Floor competence: B rung-1 median cross-recall ≥ 0.9 absolute (the index
  must make the small model *good*, not just better).
- Leak audit PASS over the new transcripts before grading.

Decision rule: a PASS at any tested small model = evidence the feature
serves the adoption floor; the owner then re-decides PR #14's merge on
that evidence (a further dated D7 amendment would record it). A FAIL at
all tested models closes 0016 per the original kill condition, with this
entry as the record.


## 2026-08-22: adoption-floor sweep verdict — FAIL at haiku; subtypes signal isolated

**Verdict: registered bars FAIL at claude-haiku-4-5.** Median recall lift
+0.0 (bar: +10pp); efficiency −5.8% tokens / −12.5% calls (bar: ≥40%
savings); floor competence passed (B rung-1 med 1.0 ≥ 0.9). Paired: B wins
10 / loses 12 / ties 43. Per the registration, this closes change 0016 and
PR #14 (unmerged, revivable on new evidence).

Leak-audit note (discipline record): the first two haiku arm-A runs were
QUARANTINED — class-2 contamination. Cause: `.codeindex/` index artifacts
(built for arm B) were visible inside the member checkouts; agents noticed
them (path-exclusion flags; two agents `ls`'d a renamed dir). No agent read
index contents or executed the binary, but observation itself fails the
class. Two blinding attempts failed (rename-in-place is visible; a
depth-limited find missed the three nest package roots). Run 4 with all 16
index dirs physically stashed outside `bench/repos/` audited PASS
(attempts=0) and is the scored baseline. The frontier arm-A run (attempts=0)
also had artifacts on disk — recorded as a residual on that verdict; its
agents never referenced them, and artifacts could only have helped the
control, which tied anyway. Lesson filed: renaming isn't hiding; verify
absence, don't assume it.

**The per-kind breakdown is the finding that matters** (haiku, mean
cross-recall, A vs B): ximpact 0.95/0.91; xcallers 0.73/0.68; xnew
0.79/0.80 — all greppable-by-construction, ties. **xsubtypes 0.29/0.43
(+14pp, n=7)** — the only structural shape in the corpus, the m5-shaped
lift appears exactly there, AND B's 0.43 is capped by the recorded adapter
gap: extends/implements references carry no namespace hint (Go `addDep`
never sets Source; other languages unverified), so rung 1 cannot fire for
subtype edges and the index is missing many of the links this task shape
needs.

**Owner decision 2026-08-22 (pivot):** close 0016/PR #14 per registration;
fix the adapter hint gap as its own change; grow the corpus with
structural, grep-hostile cross-repo tasks (impact chains, subtype maps,
name collisions — docket change 0010) mined with auditable GT; register a
NEW gate on that corpus. Revival of the query surfaces is that gate's
outcome, not a rescue of this one.

## 2026-08-22: change 0017 — Go subtype namespace hints, rebuild-diff evidence and the acceptance measurement

Plan tasks 9 (rebuild-diff accounting) and 10 (the acceptance bar), measured
together because task 9's stop-rule is stated in terms of task 10's table and
both consume the same pair of before/after indexes.

**VERDICT: PASS.** Both pinned exemplar sets moved `ambiguous → unambiguous`
with verified-correct targets. `storage.Appender` is logged **PARTIAL** and
excluded from the win count. One confidence downgrade and one behavioural
class outside the table are reported below in full; neither is suppressed.

### Method — exactly what was built

| | |
|---|---|
| bench member | `bench/repos/prometheus` @ `7086161a9` (pinned OSS) |
| "before" code | `origin/main` @ `2c8b9c3`, extracted with `git archive` and built standalone |
| "after" code | `feat/adapter-namespace-hints-extends-implements` @ `4423e20` |
| build | `go build -tags nollama ./cmd/codeindex` from each side, then `codeindex build <member>` (a from-scratch rebuild: `runBuild` unlinks the db first) |
| dump | `Store.DumpNormalized()`, called from a throwaway `cmd/tmpdump` built from **each side's own tree** — the "before" dump is produced entirely by pre-change code |
| `DumpNormalized` | byte-identical on both sides; **not** widened (constraint 8 — `dst_ns` is still not selected) |

Both indexes: 797 files, 9006 symbols, 81111 edges. Neither the symbol table
nor the edge count moved.

`dst_ns` (the hint) is not in the dump, so it was read directly out of both
`graph.db` files for the analysis below. To make the candidate counts
trustworthy the resolution ladder (`resolve` / `boundIDs` / `nsMatch` /
`DeriveNamespace`) was re-implemented against the two databases and checked
against every recorded edge: **0 mismatches in 256 checks** (128 extends edges
× 2 sides, confidence *and* chosen symbol id). The counts below are therefore
the resolver's own, not an estimate.

### Task 10 — the acceptance table: 23 addressable qualified-embed edges

Denominator: 25 Go `extends` edges were `ambiguous` before. Two are not
addressable and are excluded, leaving **23**.

Targets are `file:parent.name:line`.

| # | embed site | embedded type | cand (before → after) | chosen target (before → after) | confidence | verdict |
|---|---|---|---|---|---|---|
| 1 | `discovery/aws/ec2.go:149` `EC2Discovery` | `refresh.Discovery` | 22 → 1 | `discovery/azure/azure.go:.Discovery:176` → `discovery/refresh/refresh.go:.Discovery:37` | ambiguous → unambiguous | **WIN** (pinned) |
| 2 | `discovery/aws/lightsail.go:128` `LightsailDiscovery` | `refresh.Discovery` | 22 → 1 | `discovery/azure/azure.go:.Discovery:176` → `discovery/refresh/refresh.go:.Discovery:37` | ambiguous → unambiguous | **WIN** (pinned) |
| 3 | `discovery/ovhcloud/dedicated_server.go:55` `dedicatedServerDiscovery` | `refresh.Discovery` | 22 → 1 | `discovery/azure/azure.go:.Discovery:176` → `discovery/refresh/refresh.go:.Discovery:37` | ambiguous → unambiguous | **WIN** (pinned) |
| 4 | `discovery/ovhcloud/vps.go:68` `vpsDiscovery` | `refresh.Discovery` | 22 → 1 | `discovery/azure/azure.go:.Discovery:176` → `discovery/refresh/refresh.go:.Discovery:37` | ambiguous → unambiguous | **WIN** (pinned) |
| 5 | `notifier/notifier.go:714` `alertmanagerLabels` | `labels.Labels` | 25 → 9 | `model/labels/labels.go:.Labels:29` → unchanged | ambiguous → ambiguous | narrowed only — **not a win** |
| 6 | `promql/histogram_stats_iterator.go:23` `histogramStatsIterator` | `chunkenc.Iterator` | 4 → 4 | `promql/engine.go:histogramStatsSeries.Iterator:3801` (method) → `tsdb/chunkenc/chunk.go:.Iterator:123` (the type) | ambiguous → ambiguous | PARTIAL — **not a win** |
| 7 | `promql/promqltest/test.go:186` `test` | `testutil.T` | 7 → 1 | `prompb/io/prometheus/write/v2/custom.go:Sample.T:20` → `util/testutil/directory.go:.T:74` | ambiguous → unambiguous | **WIN** |
| 8 | `scrape/scrape.go:962` `metaEntry` | `metadata.Metadata` | 5 → 1 | `model/metadata/metadata.go:.Metadata:19` → unchanged | ambiguous → unambiguous | **WIN** (confidence only) |
| 9 | `scrape/target.go:320` `limitAppender` | `storage.Appender` | 2 → 3 | `scrape/helpers_test.go:nopAppendable.Appender:40` → `storage/fanout.go:fanout.Appender:120` | ambiguous → ambiguous | **PARTIAL** — excluded |
| 10 | `scrape/target.go:341` `timeLimitAppender` | `storage.Appender` | 2 → 3 | `scrape/helpers_test.go:nopAppendable.Appender:40` → `storage/fanout.go:fanout.Appender:120` | ambiguous → ambiguous | **PARTIAL** — excluded |
| 11 | `scrape/target.go:360` `bucketLimitAppender` | `storage.Appender` | 2 → 3 | `scrape/helpers_test.go:nopAppendable.Appender:40` → `storage/fanout.go:fanout.Appender:120` | ambiguous → ambiguous | **PARTIAL** — excluded |
| 12 | `scrape/target.go:400` `maxSchemaAppender` | `storage.Appender` | 2 → 3 | `scrape/helpers_test.go:nopAppendable.Appender:40` → `storage/fanout.go:fanout.Appender:120` | ambiguous → ambiguous | **PARTIAL** — excluded |
| 13 | `storage/remote/read.go:271` `seriesFilter` | `storage.Series` | 18 → 1 | `model/textparse/nhcbparse.go:NHCBParser.Series:117` (method) → `storage/interface.go:.Series:445` | ambiguous → unambiguous | **WIN** |
| 14 | `storage/remote/write_handler.go:549` `timeLimitAppender` | `storage.Appender` | 3 → 3 | `storage/remote/storage.go:Storage.Appender:192` → `storage/fanout.go:fanout.Appender:120` | ambiguous → ambiguous | **PARTIAL** — excluded |
| 15 | `tsdb/block.go:574` `blockTombstoneReader` | `tombstones.Reader` | 4 → 1 | `tsdb/chunks/chunks.go:.Reader:582` → `tsdb/tombstones/tombstones.go:.Reader:63` | ambiguous → unambiguous | **WIN** |
| 16 | `tsdb/db.go:1207` `dbAppender` | `storage.Appender` | 4 → 3 | `tsdb/blockwriter.go:BlockWriter.Appender:86` → `storage/fanout.go:fanout.Appender:120` | ambiguous → ambiguous | **PARTIAL** — excluded |
| 17 | `tsdb/head_read.go:386` `wrapOOOHeadChunk` | `chunkenc.Chunk` | 3 → 1 | `prompb/types.pb.go:.Chunk:998` → `tsdb/chunkenc/chunk.go:.Chunk:69` | ambiguous → unambiguous | **WIN** (pinned) |
| 18 | `tsdb/head_read.go:512` `safeHeadChunk` | `chunkenc.Chunk` | 3 → 1 | `prompb/types.pb.go:.Chunk:998` → `tsdb/chunkenc/chunk.go:.Chunk:69` | ambiguous → unambiguous | **WIN** (pinned) |
| 19 | `tsdb/head_read.go:596` `stopIterator` | `chunkenc.Iterator` | 5 → 4 | `tsdb/head_read.go:safeHeadChunk.Iterator:518` (method) → `tsdb/chunkenc/chunk.go:.Iterator:123` (the type) | ambiguous → ambiguous | PARTIAL — **not a win** |
| 20 | `tsdb/ooo_head_read.go:173` `multiMeta` | `chunkenc.Chunk` | 3 → 1 | `prompb/types.pb.go:.Chunk:998` → `tsdb/chunkenc/chunk.go:.Chunk:69` | ambiguous → unambiguous | **WIN** (pinned) |
| 21 | `util/teststorage/storage.go:77` `TestStorage` | `tsdb.DB` | 2 → 1 | `tsdb/agent/db.go:.DB:226` → `tsdb/db.go:.DB:239` | ambiguous → unambiguous | **WIN** |
| 22 | `web/api/v1/errors_test.go:154` `errorTestQueryable` | `storage.ExemplarQueryable` | 2 → 1 | `storage/interface.go:.ExemplarQueryable:181` → unchanged | ambiguous → unambiguous | **WIN** (confidence only) |
| 23 | `web/web_test.go:52` `dbAdapter` | `tsdb.DB` | 2 → 1 | `tsdb/agent/db.go:.DB:226` → `tsdb/db.go:.DB:239` | ambiguous → unambiguous | **WIN** |

**14 wins of 23** (`ambiguous → unambiguous`, every target re-read in the
prometheus source and confirmed to be the type the embed actually names).
6 rows are the `storage.Appender` PARTIAL, 2 are the same-shaped
`chunkenc.Iterator` PARTIAL, 1 (`labels.Labels`) is narrowing only.

Aggregate over all 119 Go `extends` edges: `unambiguous` 79 → 92,
`ambiguous` 25 → 12, `unresolved` **15 → 15**.

#### Correction to a plan prior — the 2 excluded edges are not "bare generic `T`"

The plan states the 2 non-addressable edges of the 25 are "bare generic `T`".
Measured, they are:

- `discovery/file/file_test.go:43` `testRunner` embeds `*testing.T` — a
  *qualified* embed that does receive the hint `testing`, but `testing` is a
  stdlib package with no namespace in the index, so `boundIDs` returns nothing
  and the edge falls through to the plain rung unchanged (7 candidates,
  `prompb/.../custom.go:Sample.T:20`). Not addressable because the target
  package is not indexed, not because the embed is bare.
- `tsdb/index/postings_test.go:1177` `postingsFailingAfterNthCall` embeds a
  bare, unqualified `Postings` — no qualifier, so no hint, resolved by the
  `srcNS` rung to `tsdb/index/index.go:Reader.Postings:1679`, unchanged.

Neither is generic. The count of 23 addressable edges is unaffected; only the
characterisation was wrong.

#### Pinned exemplars — the bar

- **`chunkenc.Chunk`: PASS.** All 3 instances (rows 17, 18, 20) flipped
  `ambiguous → unambiguous` and moved from the **wrong package**
  (`prompb/types.pb.go` `Chunk`, a protobuf message) to the **correct**
  `tsdb/chunkenc/chunk.go:69` `Chunk` interface. This is the
  wrong-package→correct-package flip the plan predicted, and it works for the
  reason the plan gave: `nsMatch` suffix-compares, so the hint
  `github.com/prometheus/prometheus/tsdb/chunkenc` matches the derived
  namespace `tsdb/chunkenc`.
- **`refresh.Discovery`: PASS, and the pinned count of 4 is confirmed.**
  Exactly 4 `refresh.Discovery` embeds were ambiguous before (rows 1–4), each
  with **22 candidates**, each now resolving to the single correct
  `discovery/refresh/refresh.go:37`. (21 further `refresh.Discovery` embeds
  exist; they were *unambiguous-but-wrong* before, so they are not in this
  table — see class D1 below.)
- **`storage.Appender`: PARTIAL, never a win.** Recorded, excluded from the 14.

#### `storage.Appender` — PARTIAL record

6 edges (rows 9–12, 14, 16). The hint
`github.com/prometheus/prometheus/storage` is **correct** and does move the
answer into package `storage`, but the package contains three symbols named
`Appender`:

```
storage/interface.go    type   Appender             :257   <- the correct target
storage/fanout.go       method fanout.Appender      :120   <- what the resolver picks
storage/fanout_test.go  method errStorage.Appender  :227
```

`boundIDs` narrows to a namespace and never within one, so it returns all
three and the caller reports its deterministic first pick as `ambiguous`.
Right package, wrong pick, honest confidence. This is **not** claimable as a
win under any framing, and closing it requires in-package disambiguation
(a real discriminator — for an embed, that the syntactic position selects a
type) to land first. Pinned in-suite by
`TestKNOWNLIMITATIONHintedEmbedStaysAmbiguousAmongInPackageSameNameSymbols`.

Note also that 4 of the 6 (`scrape/target.go`) resolved *before* to
`scrape/helpers_test.go:nopAppendable.Appender:40` via the same-scope `srcNS`
rung — the carried fact from the critic, corroborated by this measurement.
They never pointed at `cmd/prometheus/main.go`.

### Task 9 — rebuild-diff accounting: 45 lines, all accounted for

`diff` of the two `DumpNormalized` dumps: **45 changed edge lines** (43 hunks),
**0 symbol lines**, and no added or removed lines — every changed line is a
`c` replacement of an existing edge. By kind:

| kind | edges | changed |
|---|---|---|
| `calls` | 73643 | **0** (byte-identical, including `dst`) |
| `imports` | 7334 | **0** (see the named class below) |
| `implements` | 6 | 0 (none are Go — constraint 7) |
| `extends` | 128 | **45** |

The stop-rule is *"every diff line must be accounted for by a row in task 10's
table; any line outside that table is a stop-and-investigate."* Applying it:

- **22 lines are table rows.** The 23rd table row (#5, `labels.Labels`) changes
  only its candidate count (25 → 9); its confidence and resolved dst are
  unchanged, so `DumpNormalized` cannot see it and it contributes no line.
- **23 lines fall outside the table.** The stop-rule fired on all 23. Each was
  investigated; they form three named classes, and none is unexplained.

**Class D1 — `refresh.Discovery` self-resolution corrected: 21 edges.**
`unambiguous → unambiguous`, target moved, all verified correct. Before the
change these embeds were decided by the `srcNS` same-scope rung and bound to
the embedding package's *own* `Discovery` — in 14 of the 21 cases the struct
resolved its `*refresh.Discovery` embed **to itself**
(`discovery/azure/azure.go:177 Discovery` → `discovery/azure/azure.go:176
Discovery`). They now resolve to `discovery/refresh/refresh.go:37`. These are
confidently-wrong → confidently-right corrections. They are **not** counted in
the 14 wins, because the acceptance bar is `ambiguous → unambiguous` and these
never were ambiguous. Sites: `discovery/{azure,digitalocean,dns,eureka,gce,
http,linode,marathon,nomad,puppetdb,triton,uyuni,vultr}`, `hetzner/{hcloud,
hetzner,robot}`, `ionos/server`, `moby/{docker,dockerswarm}`,
`scaleway/{baremetal,instance}`.

**Class D2 — one further wrong-target correction: 1 edge.**
`promql/engine.go:3794 histogramStatsSeries` embeds `storage.Series`. Before:
`unambiguous` → `promql/value.go:.Series:67` (the promql `Series` struct —
wrong). After: `unambiguous` → `storage/interface.go:.Series:445` (the
interface actually embedded — correct).

**Class D3 — the one confidence downgrade in the entire diff: 1 edge.**
`promql/engine_test.go:294 hintRecordingQuerier` embeds `storage.Querier`.

```
before: unambiguous  promql/engine_test.go:noopHintRecordingQueryable.Querier:289  (a METHOD in the same file)
after:  ambiguous    storage/fanout.go:fanout.Querier:74                           (a method, right package)
```

This is the only `unambiguous → ambiguous` transition anywhere in the diff.
It is the `storage.Appender` limitation in its other guise: the hint correctly
moves the edge out of the same-file `srcNS` rung and into package `storage`,
where 5 symbols are named `Querier` (`storage/interface.go:113` is the correct
one; the other four are methods), so the resolver reports `ambiguous`. The
before state was a confidently-wrong answer pointing at a same-file method;
the after state is an honestly-uncertain answer in the right package. It is
recorded here as a real, measured cost of the change, **reported and not
fixed** — this was a measurement task, and the fix is the same in-package
disambiguation prerequisite that gates `storage.Appender`.

**Nothing in the 45 is unaccounted for. Stop-rule satisfied.**

Constraint 1 is confirmed empirically rather than argued: **0** Go subtype
edges went `unresolved → resolved`, and 0 went the other way. The 15
unresolved Go `extends` edges (`sync.Mutex`, `sync.RWMutex`, `net.Conn`,
`http.RoundTripper`, `http.ResponseWriter`, `mock.Mock`, …) all now carry a
correct hint and remain unresolved, because their targets are third-party or
stdlib names that are simply absent from the index. Hints narrow candidates;
they never create symbols.

### The named class: Go single-segment import edges — measured 0, then CLOSED by construction

> **SUPERSEDED — corrected after the measurement.** This section as originally
> written described a live class that merely happened to be empty on
> prometheus. Two review-driven fixes landed after the measurement was written
> (`8e9ee63`, `9d84218`) and falsified it. The class is no longer live at all.
> The corrected account is below; the original reasoning is retained because it
> is what the fix was a response to.

**What the original measurement said, and what is still true of it.** The plan
warned that single-segment Go imports would move: `import "log"` now
carries `Source == "log"`, `store.go` calls `resolve` on it (no `/` in the
target), and `nsMatch` suffix-matches a candidate namespace like
`internal/log` against the hint `log` — so an import edge that used to be
decided by the `srcNS` rung gets decided by the `boundIDs` rung instead.

**On the `prometheus` member this class had size 0 as measured.** Enumerated,
not assumed:

- 5108 Go `imports` edges exist. **As measured, all 5108 gained a non-empty
  `dst_ns`** (before: 0 — Go import deps carried no `Source`). *This no longer
  holds:* after the fixes below the count is **0 of 5108** — Go import edges
  carry `dst_ns = ""` again. The 2226 pre-existing hinted import edges are all
  TypeScript, from `web/ui`, and are unaffected by either state.
- 1991 of the 5108 are single-segment (no `/` in the target) and are therefore
  actually passed to `resolve`. Their confidence split is **identical** before
  and after: 64 `unambiguous`, 93 `ambiguous`, 1834 `unresolved`.
- Compared edge-by-edge on (confidence, resolved dst), ignoring `dst_ns`:
  **0 of 7334 import edges changed**.

The mechanism did not fire because `nsMatch` requires a **separator** before
the suffix (`strings.HasSuffix(candNS, "/"+hint)`). Querying the index for any
namespace that equals — or ends in `/` followed by — a single-segment Go
import name used in this repo returns the **empty set**. The near misses are
instructive:

| import | same-named symbols in the index | their namespaces | matches hint? |
|---|---|---|---|
| `log` (4 edges) | 3 | `tsdb`, `tsdb/agent`, `tsdb/wlog` | no — `tsdb/wlog` ends in `wlog`, not `/log` |
| `hash` (6 edges) | 2 | `rules`, `scrape` | no |
| `sync` | 2 | `notifier`, `scrape` | no |
| `bytes` | 1 | `tsdb/chunkenc` | no |

So the 4 `import "log"` edges stay `ambiguous` on `tsdb/agent/db.go`, exactly
as before.

At the time of measurement the honest statement of size was: *the class is real
and reproducible, its measured size on the one member measured here is 0 edges,
and it remains unmeasured on every other member* — a property of prometheus's
directory layout (it has no `internal/log`-shaped package), not evidence the
class was closed. Any repo with such a package would have shown the movement.

**That caveat no longer applies: the class was closed structurally.** A
review-driven fix (`9d84218`) applies the self-binding skip **at the dep site
as well as** the original site — one predicate, two sites. A Go import dep
therefore no longer carries a `Source` hint at all, so `resolve` is never
handed one and the `boundIDs` rung can never take over from `srcNS` for a Go
import edge. The class is removed **by construction**, not by an argument about
one repo's layout: it cannot fire on *any* repo, `internal/log`-shaped or not.
The measured 0 on prometheus is consistent with this, but it is no longer the
evidence — the evidence is the skip.

The honest, corrected statement is therefore: *the class was real; it was
measured at 0 edges on prometheus; it was then closed structurally by a
review-driven fix, and is now guarded in-suite.*

The guard is `TestGoImportEdgeKeepsItsPreChangeResolution` in
`internal/graph/importcollision_test.go` — a **no-regression** guard asserting
a Go import edge keeps its pre-change resolution. Plan task 6b asked for
exactly that guard and, before the fix, could not deliver it: with the class
live, the pre-change resolution was not something the code preserved. An
earlier claim in this document that "task 6b pins exactly that movement
in-suite" was true of the pre-fix test and is **false now** — there is no
movement left to pin, and the test pins its absence instead.

Note that the import delta was invisible to `DumpNormalized`, which does not
select `dst_ns`; that is why `imports` reads **0** changed in the task 9 table
both before and after these fixes, and why that table is unaffected by this
correction.

### `dst_ns` movement alone — counted as nothing, as required

For completeness, and explicitly **not** as evidence: 70 of the 119 Go
`extends` edges gained a non-empty hint. 45 of those produced a resolution
change (the diff above); the other **25 changed `dst_ns` and nothing else**.
None of the 25 is counted anywhere in this write-up, and `DumpNormalized` does
not select `dst_ns`, so none of them appears in the diff either.

### Reproduction

```sh
git archive 2c8b9c3 | tar -x -C /tmp/before-src
(cd /tmp/before-src && go build -tags nollama -o /tmp/ci-before ./cmd/codeindex)
go build -tags nollama -o /tmp/ci-after ./cmd/codeindex

R=bench/repos/prometheus            # pinned OSS member @ 7086161a9
rm -rf $R/.codeindex && /tmp/ci-before build $R && cp $R/.codeindex/graph.db /tmp/before.db
rm -rf $R/.codeindex && /tmp/ci-after  build $R && cp $R/.codeindex/graph.db /tmp/after.db
# dump each with a Store.DumpNormalized() caller built from its own side, then diff.
```

No `.db` file and no bench-repo content is committed; the member's original
`.codeindex/` was restored after the run.
