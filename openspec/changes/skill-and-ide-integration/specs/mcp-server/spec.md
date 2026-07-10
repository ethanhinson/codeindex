## ADDED Requirements

### Requirement: MCP server exposing the anchor-rule tools

The engine SHALL provide `codeindex mcp <repo-root>` — a stdio MCP server
(official Go MCP SDK, pinned version) exposing `impact`, `callers`, and
`callees` tools that map 1:1 onto the existing query functions, with tool
descriptions embedding the anchor rule (use when branching out from a known
symbol; not for locating).

#### Scenario: Tool call answers with fresh data

- **WHEN** an MCP client calls `callers` with a symbol name
- **THEN** the server applies the incremental update first (same fresh-on-query
  behavior as the CLI) and returns the reference-based result with ambiguity
  flags

#### Scenario: Descriptions carry the trigger discipline

- **WHEN** a client lists the server's tools
- **THEN** each description states the anchor rule and names the negative case
  (locating/definition-finding) as out of scope for the tool

### Requirement: Concurrency safety for a long-lived server

The MCP server SHALL serialize index-mutating work (re-check patches) so
concurrent tool calls never violate SQLite's single-writer constraint.

#### Scenario: Concurrent tool calls during an edit

- **WHEN** two tool calls arrive while the working tree has pending changes
- **THEN** exactly one performs the incremental patch while the other waits,
  and both answer from the patched index without error

### Requirement: Client configuration shipped

The change SHALL ship copy-paste configuration for Cursor, Claude Desktop, and
VS Code (MCP client settings) in the plugin/README.

#### Scenario: Configuring an IDE client

- **WHEN** a user follows the README snippet for their client
- **THEN** the client lists the three tools and can answer an impact query
  against a local repository
