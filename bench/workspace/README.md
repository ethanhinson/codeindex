# bench/workspace — the workspace-graph evidence gate

Pre-registered evidence gate for `openspec/changes/workspace-graph`
(design D7). Registered 2026-08-17, **before any engine code and before
the first scored run** (tasks.md §2.3; the C2 GO that unblocked this is
recorded in `ROADMAP-DEBATE.md` §6). The gate script must read the bars
from this file, never from its own source (m5 precedent).

## Corpus (open source, four language clusters)

**Amendment 2026-08-17, before any scored run:** the corpus was rebuilt
on open-source members only (owner decision — no private code or private
repo results in the repo) and widened from the registered "2+ languages"
to all four indexer languages. Member count grows accordingly (10 vs the
registered 3–5); the bars below are untouched.

One shared-lib member and its consumer member(s) per language; every lib
pin is the version its consumer actually declares. Machine-readable
pins: `corpus.json`; consumers are existing `bench/repos.json` pins.

| cluster | lib member (pin) | consumers (pin) | evidence of the edge |
|---|---|---|---|
| PHP | `symfony` (v7.2.2) | `drupal` (11.1.0), `laravel` (v11.38.0) | `use Symfony\Component\...` |
| TS | `nest-common` (v10.4.15) | `nest-core`, `nest-microservices` | `import {X} from '@nestjs/common'` — one checkout, the D1 monorepo degenerate case |
| Py | `werkzeug` (3.1.3, flask's floor) | `flask` (3.1.0) | `from werkzeug.x import Y` |
| Go | `client_golang` (v1.20.5, from prometheus go.mod) | `prometheus` (v3.1.0) | `import ".../client_golang/prometheus"` |

Workspace root: `bench/repos/oss-ws/` — a dedicated directory reaching
every member by relative path (design D1's sanctioned shape for
scattered repos). Shared-lib-with-≥2-consumers: `symfony` ← {`drupal`,
`laravel`} and `nest-common` ← {`nest-core`, `nest-microservices`}.

## Tasks

≥30 cross-repo impact/caller tasks mined per M2's discipline
(`build_tasks_ws.py`): ground truth is import-mediated plain-text
extraction across member checkouts — equally available to both arms,
never from codeindex. Tasks are cross-member scoped ("which files in
the OTHER members reference X" — the frontier question). All tasks are
rung-1; this corpus has no organic bare-name (rung-2) cross-edges —
recorded in the task header. GT size is capped at 40 files (larger
answers measure listing stamina, not finding). Per-member and
per-language quotas recorded in the task file header. Plus the existing
single-repo goldens for the non-regression bar.

## Arms

- **A (control):** agent + shell, all member checkouts on disk —
  grep-across-repos. Honest, not blinded.
- **B (treatment):** A + the workspace-graph MCP surface.

Isolation: every headless run uses `--setting-sources project,local`
(the bench-hook-leak rule — the global codeindex plugin contaminates
controls otherwise). Grader-blind formatting. Leak-audit all four
classes before any verdict (id-paired transcript audit,
`bench/agent_ab/leak_audit.py` pattern).

## Bars (copied verbatim from design D7 — all required)

- Cross-repo caller/blast-radius recall: B ≥ A, and B ≥ 0.9 absolute
  on import-mediated (rung-1) edges.
- Efficiency: B uses ≥40% fewer exploration tokens or ≥40% fewer
  tool/shell calls on the cross-repo tasks (mirrors the M5 gate form).
- Non-regression: single-repo golden suite byte-identical for non-
  workspace roots; workspace-mode answers on a single-member workspace
  match single-repo answers modulo the `repo` field.
- Freshness property: mutate one member, query from another without an
  explicit rebuild → the answer reflects the mutation or its coverage
  clause names the member stale. Silent staleness is a hard fail.
- Discipline rule all four leak classes, including grader-blind
  formatting.

**Kill condition:** if B does not beat A on recall or efficiency, the
result is published as a FINDINGS entry
(`bench/engine/FINDINGS-workspace-graph.md`) and the change closes —
the grep-across control winning is a legitimate answer to the frontier
hypothesis. Iteration happens within the registered budget only.
