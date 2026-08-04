# codeindex graph API

`codeindex serve` exposes the project's **symbol** call graph as a headless,
read-only JSON API over loopback HTTP. It hosts no static content. Every graph
response carries a top-level `schemaVersion` so external consumers are insulated
from internal shape changes.

**Current `schemaVersion`: `"1"`**

## Endpoints

### `GET /api/health`

Liveness + build identity.

```json
{ "status": "ok", "version": "<build>", "root": "<indexed repo root>" }
```

### `GET /api/graph?symbol=<name>&parent=<optional>`

The neighborhood of a focus symbol: the focus plus its direct callers and
callees. `symbol` is required (omitting it returns HTTP 400); `parent`
optionally disambiguates a method by its enclosing type.

```json
{
  "schemaVersion": "1",
  "focus": "sym:A",
  "nodes": [ /* Node */ ],
  "edges": [ /* Edge */ ]
}
```

### `GET /api/graph/full`

The whole symbol graph: every tier-0 symbol that participates in a resolved call
edge, plus those edges. Isolated leaf symbols (no resolved call) are omitted so
the call structure is not buried. Nodes carry a `group` (package directory) for
clustering. The response is the whole graph in one payload; there is currently no
pagination.

```json
{
  "schemaVersion": "1",
  "focus": "",
  "nodes": [ /* Node */ ],
  "edges": [ /* Edge */ ]
}
```

## Types

### Node (symbol-only)

```json
{
  "id": "sym:Pkg.Name",
  "kind": "symbol",
  "label": "Pkg.Name",
  "file": "internal/pkg/file.go",
  "line": 42,
  "signature": "func Name(x int) int",
  "group": "internal/pkg"
}
```

- `kind` is always `"symbol"`.
- `file`, `line`, `signature`, `group` are omitted when empty.
- In `/api/graph`, node ids are `sym:<qualified-name>`; in `/api/graph/full`,
  node ids are `sym#<internal-id>` (stable within a single response).

### Edge

```json
{ "source": "sym:A", "target": "sym:Helper", "kind": "calls", "conf": "high" }
```

- `kind` is always `"calls"`.
- `conf` (confidence) is present on neighborhood edges when known; omitted otherwise.

## Versioning

`schemaVersion` is a string. A backward-incompatible change to the node/edge
shape bumps it. Additive, optional fields do not.
