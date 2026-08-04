---
id: 9
slug: graph-api-schema-version-contract
title: Versioned symbol-graph JSON API contract for `serve`
status: Accepted
date: 2026-08-04
supersedes: []
reverses: []
relates_to: [7]
change: 3
---

## Context

Change 0003 turned `codeindex serve` from a lore-graph UI host into a headless
JSON graph API that external systems (a future viewer, other tooling) can query
without reading Go. Previously the read model coupled symbols with a lore
overlay and the web app was the only consumer, so there was no stable external
contract. With the web app deleted and `serve` becoming a decoupled query
surface, the response shape becomes a public interface that consumers will build
against, and internal read-model changes must not silently break them.

## Decision

Pin a top-level string `schemaVersion` (currently `"1"`) on every graph API
response, implemented as a `graphResponse` struct that embeds `readmodel.Graph`
so `schemaVersion` promotes to a sibling of `focus`/`nodes`/`edges` in the JSON.

The node/edge shape is symbol-only:

- `Node{ID, Kind:"symbol", Label, File, Line, Signature, Group}`
- `Edge{Source, Target, Kind:"calls", Conf}`

The three endpoints are:

- `GET /api/health`
- `GET /api/graph?symbol=<name>&parent=<optional>` (missing `symbol` → 400)
- `GET /api/graph/full`

Node ids are `sym:<qualified-name>` in the neighborhood endpoint and
`sym#<internal-id>` in the full-graph endpoint (stable within one response).

Versioning policy: a backward-incompatible change to the node/edge shape bumps
`schemaVersion`; additive optional fields do not. The contract is documented in
`docs/graph-api.md`.

## Consequences

External consumers are insulated from internal read-model refactors behind a
single explicit version token; the contract is documented so tooling can be
built without reading Go. Cost: every response carries the envelope, and shape
changes now require a conscious version decision. Because no external consumers
exist yet (the only prior consumer, the web app, was deleted in this change),
versioning is cheap insurance adopted now rather than a compatibility burden
retrofitted later.
