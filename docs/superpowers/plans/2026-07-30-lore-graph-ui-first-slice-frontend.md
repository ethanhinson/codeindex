# Lore Graph UI — First Slice (Frontend): graph-native SPA Implementation Plan

> **For agentic workers:** built on the backend slice (`internal/webserver` `/api/graph`, `/api/health`). Steps use checkbox syntax.

**Goal:** A graph-native React SPA that lets you enter a focus (symbol or lore id), see its neighborhood as an interactive Cytoscape graph joining code + lore, and navigate by expanding/pivoting nodes — with a command palette, typed visual language, an inspector drawer, and traversal breadcrumbs.

**Architecture:** Vite + React + TypeScript app in `web/`, building into `internal/webserver/dist/` (embedded by the Go binary). Consumes the read-only `/api/graph?focus=<id>` endpoint. Cytoscape.js (+ fcose layout) renders the graph; navigation state accumulates the explored sub-graph and drives an inspector. Playwright drives a real browser against `codeindex serve` for verification.

**Tech Stack:** React 18, Vite 5, TypeScript, cytoscape, cytoscape-fcose, @playwright/test. No backend changes.

## Global Constraints

- Build output MUST go to `internal/webserver/dist/` (that's what `//go:embed dist` serves). `web/node_modules` git-ignored; `internal/webserver/dist/` committed.
- API contract (fixed by the backend): `GET /api/graph?focus=<id>` → `{ focus: string, nodes: Node[], edges: Edge[] }`; `Node = { id, kind: "symbol"|"decision"|"item"|"note"|"path", label, file?, line?, signature?, status?, priority? }`; `Edge = { source, target, kind: "calls"|"anchors"|"blocked_by", conf? }`. `GET /api/health` → `{ status, version, root }`.
- Node id scheme: `sym:<QName>`, `dec-…`/`itm-…`/`note-…`, `path:<path>`. Focus routing is by that id.
- Read-only: the SPA never POSTs; no mutation UI.
- Typed visual language: each node kind and edge kind has a distinct, consistent color/shape.
- Verification is Playwright against the real Go server serving the real built SPA — not mocked fetches.

## File Structure

- `web/package.json`, `web/vite.config.ts`, `web/tsconfig.json`, `web/index.html` — project + build config (outDir `../internal/webserver/dist`, empty base so assets are root-relative).
- `web/src/main.tsx` — React root.
- `web/src/types.ts` — TS mirror of the API types + kind unions.
- `web/src/api.ts` — `getGraph(focus)`, `getHealth()`.
- `web/src/graph/style.ts` — Cytoscape stylesheet: node-kind and edge-kind visual language.
- `web/src/graph/GraphCanvas.tsx` — Cytoscape mount; renders elements; fcose layout; emits `onNodeTap(id)`.
- `web/src/useExploration.ts` — hook: current focus, accumulated elements (deduped), breadcrumbs; `focusOn(id)` (replace) and `expand(id)` (merge neighborhood).
- `web/src/CommandPalette.tsx` — ⌘K / search box to set focus by id or label.
- `web/src/Inspector.tsx` — focused-node detail drawer.
- `web/src/App.tsx` — shell wiring canvas + palette + inspector + breadcrumbs.
- `web/tests/e2e.spec.ts`, `web/playwright.config.ts` — browser verification against `codeindex serve`.
- `.gitignore` — add `web/node_modules`, `web/test-results`, `web/playwright-report`.

## Tasks

### Task 1 — Scaffold + build into embed dir
Create the Vite/React/TS project; `npm install`; Vite `build.outDir=../internal/webserver/dist`, `emptyOutDir=true`, `base=""`. App renders a placeholder that calls `/api/health` and shows status. `npm run build` produces `internal/webserver/dist/{index.html,assets/*}`. Confirm `go build ./cmd/codeindex && go test ./...` still pass (embed picks up the built assets). Commit (including committed `dist/`).
Verify: `npm run build` succeeds; `dist/index.html` exists; Go tests green.

### Task 2 — Types + API client
`types.ts` (Node/Edge/Graph + unions) and `api.ts` (`getGraph`, `getHealth`). App fetches health on mount and renders `ok <version>`.
Verify: `npm run build` clean; tsc no errors.

### Task 3 — Cytoscape canvas + typed styles
`style.ts` stylesheet (symbol=blue ellipse, decision=amber diamond, item=green round-rect, note=grey rect, path=slate hexagon; edges: calls=solid grey directed, anchors=dashed amber, blocked_by=dotted red). `GraphCanvas.tsx` mounts Cytoscape with fcose, takes `elements`, calls `onNodeTap`. App hardcodes a focus (e.g. `sym:Neighborhood`) via `getGraph` to prove rendering.
Verify: build clean; Playwright (Task 7 harness, run early) shows ≥1 node.

### Task 4 — Exploration state (focus / expand / pivot / breadcrumbs)
`useExploration.ts`: holds focus id, a deduped element map, and a breadcrumb trail. `focusOn(id)` replaces the view with that neighborhood; `expand(id)` merges the tapped node's neighborhood into the current elements (dig deeper). Node tap → `expand`; breadcrumb click → `focusOn`. App uses the hook.
Verify: build clean.

### Task 5 — Command palette
`CommandPalette.tsx`: a search input (focused on ⌘K/`/`) that accepts a focus id (or label) and calls `focusOn`. Seeded suggestions optional.
Verify: build clean.

### Task 6 — Inspector drawer
`Inspector.tsx`: shows the focused/tapped node — kind, label, file:line, signature (symbols) or status/priority (lore) — with its neighbors listed as clickable chips that `focusOn`.
Verify: build clean.

### Task 7 — Playwright browser verification
`playwright.config.ts` (webServer: build the Go binary and run `codeindex serve` on a test port against this repo root; baseURL that port). `e2e.spec.ts`:
1. loads `/`, health ok;
2. focus `sym:Neighborhood` via palette → canvas renders multiple nodes;
3. tap a neighbor → node count grows (expand) and breadcrumbs update;
4. inspector shows the focused node's label.
Capture screenshots into `web/test-results/`.
Verify: `npx playwright test` green; screenshots produced.

## Verification / done
`npm run build` clean, `go test ./...` green, `npx playwright test` green, and a manual screenshot of the running app showing a code+lore neighborhood.
