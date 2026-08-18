# Proposal: workspace-graph

## Why

The index's promise stops at the repo boundary. An agent working in a
multi-repo org (services + shared internal libraries) or a multi-project
monorepo must fall back to grep across sibling checkouts for exactly the
questions the index exists to answer — "who calls this across the
workspace," "what breaks if I change this shared signature." Cross-repo
blast radius is the registered untested frontier
(`bench/agent_ab/FINDINGS.md:57`: "hypothesis, not evidence") and the
class of task where grep is expensive or structurally insufficient.

This change is **specified now, built later** (after the C1/C2 fork of
the debate-amended roadmap). The reason to spec now is M3: M3 freezes
the edge provenance/confidence/coverage schema, and artifacts carry the
schema version — if that schema ships with no repo dimension, cross-repo
support later forces a second migration, which the artifact-import
architecture turns into a cold-build regression for every consumer
(ROADMAP-DEBATE.md, Advocate gap #7). The schema reservations in this
spec are the part M3 must honor; everything else waits for its gate.

## What Changes

- **Workspace manifest + member registry.** A workspace root directory
  carries `.codeindex/workspace.json` listing member projects (id, root
  relative to the workspace, resolution namespaces, optional declared
  dependencies). `codeindex init-workspace <root> --scan` generates it;
  members' namespaces are auto-discovered (go.mod module path, package
  name(s), composer name, Python top-level modules) and overridable.
  Identity stays filesystem-based — no git remotes, no network.
- **Same verbs, wider root.** Every existing query verb (`callers`,
  `callees`, `find`, `grep`, `nav`, `impact`) accepts a workspace root
  wherever it accepts a repo root today, and answers over the union
  graph. The answer schema is today's schema plus a `repo` provenance
  field per reference and workspace-relative paths. No new query
  dialect; the MCP server pointed at a workspace root serves the whole
  graph unchanged. A monorepo is the degenerate case: a workspace whose
  members are subdirectories of one checkout.
- **Overlay, not copy.** Per-repo `.codeindex/graph.db` files remain the
  sources of truth (and remain individually artifact-importable). A
  workspace overlay DB stores only the member registry, cross-repo
  edges, and per-member freshness stamps. No symbol is duplicated;
  the "output is COMPLETE and always fresh" contract extends across the
  boundary via the stamps (a stale member is re-freshened or declared
  in the coverage field — never silently served).
- **Cross-repo resolution ladder**, conservative by construction:
  unresolved import edges whose namespace hint maps to a member's
  declared namespace resolve as exact-class; bare-name cross-repo
  matches are never silently exact — unique-across-members is inferred,
  multiple candidates are ambiguous with a candidate count.
- **M3 schema reservations** (the load-bearing deliverable): provenance
  mechanism values `cross_repo_import` / `cross_repo_name`, and a
  workspace clause in the coverage field (members consulted / stale /
  outside-workspace unknown).
- **Evidence gate, pre-registered** before any build: a cross-repo
  impact bench where the control arm is an agent with shell access to
  all member checkouts (grep-across-repos — an honest control, not a
  blinded one). Bars in design D7.

## Capabilities

### New Capabilities

- `workspace-graph`: manifest + registry, single-graph query semantics,
  overlay storage + freshness, the cross-repo resolution ladder, and
  the coverage/provenance contract for workspace answers.

### Modified Capabilities

- None. `semantic-search` across a workspace is an explicit non-goal of
  this change (vectors stay per-repo); it gets its own change with its
  own gate if workspace-graph passes its gate.

## Impact

- New: `internal/config` (workspace manifest), `internal/graph`
  (overlay store), `internal/engine` (workspace freshen), workspace
  paths in `internal/query`; `cmd/codeindex` root-kind detection;
  `internal/mcpserver` repo-field plumbing. Per-repo schema untouched
  except the M3-coordinated provenance fields. Sequencing: schema
  reservations land with M3; implementation starts only on a GO at the
  C2 fork; bench precedes merge.
