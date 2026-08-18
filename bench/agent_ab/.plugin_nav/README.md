# codeindex plugin for Claude Code

Blast-radius navigation for refactoring: when you know a function/method/type
and need **who calls it, what it calls, and what breaks if it changes**.

**What it is NOT:** a search tool. Locating definitions and finding files is
measurably cheaper with grep (A/B: codeindex on locate-questions cost **+17%**;
on branch-out questions it saved **−73%**). The skill encodes exactly that
boundary; the plugin's value depends on respecting it.

## Requirements

- The `codeindex` binary on PATH (or `CODEINDEX_BIN` set):
  `go build -o /usr/local/bin/codeindex ./cmd/codeindex` (CGO; needs a C toolchain)
- A repository in a supported language: Go, TS/JS, Python, or PHP. The index
  self-builds and self-refreshes on every query — no manual build step.

## Install

```
claude --plugin-dir /path/to/code-indexer/plugin
# or add to your plugin config permanently
```

## What you get

| Piece | What it does |
|---|---|
| **Prompt note** (UserPromptSubmit hook) | Injects a ~155-token availability note per prompt in supported repos: the anchor rule + trust instruction. This is what drives adoption (measured: always-visible beats lazy skill). |
| **Post-edit hook** | After the agent edits a function (any supported language) that has callers elsewhere, injects a ≤150-token note: symbol, caller count, where — once per symbol per session. |
| **`/codeindex:impact <symbol>`** | Counts-first blast-radius summary (callers + callees) — run before modifying a symbol. |
| **`/codeindex:explore <concept>`** | Feature exploration: semantic `search` feature map, then `impact` on the winning entry point. |

(The v3 A/B gate measured the earlier skill + primitive-command apparatus as
net-negative — ~3.1k-token footprint — so v4 deliberately ships without them.)

## Hook controls

- Disable per-repo: `touch .codeindex/hook-disabled`
- Disable globally: `CODEINDEX_HOOK_DISABLE=1`
- The hook is silent on any failure and never blocks edits.

## Honest limits

- Languages: Go, TS/JS, Python, PHP (name-based resolution across all —
  `[ambiguous]` flags mark name collisions; verify those by file before
  trusting). Anonymous/lambda functions are not indexed as symbols.
- Agent A/B evidence covers Go and PHP (the laravel/framework reproduction
  held the sign on every task type, with correctness UP on the hardest two).
  TS and Python pass engine validation and the deterministic navigation
  bench, but have no agent A/B of their own yet.
- `/impact` covers call + import/extends/implements edges (type-usage
  references still excluded, disclosed in output).
- Vendored dependencies: `codeindex depmap <dir> --namespace <ns> --version <v>
  -o <out.db>` builds a dependency map so calls into deps resolve with
  `[dep ns@ver]` provenance.

## MCP server (IDEs)

`codeindex mcp <repo-root>` serves `impact`, `nav`, `callers`, `callees`,
`dependents`, `find`, `grep`, and `search` over stdio to any MCP client, plus
an `explore-feature` prompt (search → entry point → impact). Tool
descriptions carry the measured anchor rule, the trust instruction, and the
routing law (concept/feature question → `search`; known symbol →
`impact`/`callers`; distinctive exact name → plain text search), so IDE
agents inherit the discipline automatically. Verified against a real client
(Claude Code as MCP client). For clients that under-surface MCP prompts,
drop `docs/editor-rules-snippet.md` into `.cursor/rules` /
`.github/copilot-instructions.md` / `AGENTS.md`.

**Cursor** (`.cursor/mcp.json` in the repo, or global `~/.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "codeindex": {
      "command": "codeindex",
      "args": ["mcp", "/absolute/path/to/your/repo"]
    }
  }
}
```

**Claude Desktop** (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "codeindex": {
      "command": "/usr/local/bin/codeindex",
      "args": ["mcp", "/absolute/path/to/your/repo"]
    }
  }
}
```

**VS Code** (`.vscode/mcp.json`):

```json
{
  "servers": {
    "codeindex": {
      "type": "stdio",
      "command": "codeindex",
      "args": ["mcp", "${workspaceFolder}"]
    }
  }
}
```

**Claude Code** (per session):

```
claude --mcp-config '{"mcpServers":{"codeindex":{"command":"codeindex","args":["mcp","/path/to/repo"]}}}'
```

Concurrency-safe for a long-lived server: index updates are serialized
in-process (verified by test: concurrent tool calls during a pending edit).
