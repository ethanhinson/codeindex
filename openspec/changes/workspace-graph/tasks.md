# Tasks: workspace-graph

Sequencing: section 1 rides into M3 now. Sections 2–5 are gated on a GO
at the C2 fork (ROADMAP-DEBATE.md path C) and are not started before it.

## 1. M3 schema reservations (now, with M3's schema freeze)

- [ ] 1.1 Reserve provenance mechanism values `cross_repo_import` /
      `cross_repo_name` in the M3 edge schema enum
- [ ] 1.2 Reserve the `workspace: {members_consulted, members_stale,
      boundary}` coverage clause shape; record its graph-vs-retrieval
      layer per verb in the M3 coverage-layer policy doc
- [ ] 1.3 Note the reservations in the M1 epistemics page (confidence =
      resolver-visibility, extended to member namespaces)

## 2. Bench first (post-GO, before any engine code)

- [ ] 2.1 Assemble the cross-repo corpus: 3–5 member workspace, ≥2
      languages, one shared lib with ≥2 consumers, ≥30 organic tasks
      (extend the M2 miner; per-member quota recorded)
- [ ] 2.2 Wire arms: A = shell + all checkouts (grep-across control),
      B = A + workspace MCP; `--setting-sources project,local` isolation
      (bench-hook-leak rule); grader-blind formatting; leak-audit all
      four classes
- [ ] 2.3 Register the bars from design D7 in the residuals backlog
      before the first scored run

## 3. Workspace core (post-GO)

- [x] 3.1 Manifest load/validate + `init-workspace --scan`
      (namespace auto-discovery per language + monorepo member
      discovery via go.work / pnpm-workspace.yaml / composer path
      repos / lerna.json / package.json workspaces, manifest overrides)
- [x] 3.2 Overlay store: registry, cross-edges by stable key, member
      stamps; overlay schema version independent of graph.db version
- [x] 3.3 Cross-repo resolution ladder per design D3 (import-mediated
      exact only; ambiguity with candidate counts; member-over-dep
      suppression with skew recording)
- [ ] 3.4 Workspace freshen: per-member freshen + stamp-gated
      incident-edge re-resolution; `workspace-status` verb
- [ ] 3.5 Unit tests: ladder order, stable-key survival across member
      rebuild, stamp gating, single-member-workspace ≡ single-repo

## 4. Query + surfaces (post-GO)

- [ ] 4.1 Union-graph paths for callers/callees/impact/nav; fan-out for
      find/grep; workspace-relative paths + `repo` field;
      `<member-id>:` anchor prefix
- [ ] 4.2 CLI root-kind detection (workspace manifest vs repo) with
      byte-identical single-repo goldens
- [ ] 4.3 MCP: workspace root support, `repo` in result schemas, no new
      tools; plugin note untouched
- [ ] 4.4 Golden tests: workspace answers pinned; freshness scenario as
      an executable property test

## 5. Gate

- [ ] 5.1 Run the pre-registered gate (design D7); iterate within the
      registered budget only
- [ ] 5.2 Record verdict + residuals in
      `bench/engine/FINDINGS-workspace-graph.md` — including a control
      win, which closes the change per spec
