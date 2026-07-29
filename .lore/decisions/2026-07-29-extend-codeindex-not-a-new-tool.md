---
id: dec-01KYR17XECA6GT2VX6QCGGRXKK
title: Extend codeindex rather than build a sibling binary or separate repo
status: active
date: 2026-07-29
anchors:
    - path: cmd/codeindex/
    - path: internal/mcpserver/
refs:
    - url: docs/superpowers/specs/2026-07-29-lore-engine-design.md
---
Lore is a second domain inside the codeindex binary: `codeindex lore *`
subcommands, a `lore_*` tool family on the existing MCP server, records
indexed alongside the symbol graph. One install, one MCP config per host.

The deciding factor is the join: lore records carry symbol/file anchors, so
impact queries can return attached decisions and open items — code
blast-radius and decision blast-radius in one answer. No memory product has
this because none of them own a symbol graph.

## Alternatives considered

**Sibling binary, same repo.** Clean identities, but two MCP servers and two
plugins per host, and the symbol join becomes cross-tool integration.

**Separate repo.** Rebuilds all delivery machinery (MCP server, plugin,
release); no join in v1. Only right if lore is a separate product line, which
dogfooding has not yet shown.
