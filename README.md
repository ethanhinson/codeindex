# codeindex

codeindex is a code navigation index for AI agents. It answers questions like "who calls this function?", "what does it depend on?", and "what breaks if I change it?" with compact `file:line` references and signatures, instead of making an agent grep and read whole files.

The point is saving tokens and time. In our Go-repo experiments, answering caller and dependency questions through the index used far fewer tokens than grepping (up to 100x to 500x fewer on large files), and agents using it branched out to affected code 62% more often. Full evidence lives in `bench/`.

## What it is good at, and what it is not

codeindex is a blast-radius tool. Use it when you know a symbol and need to see who calls it, what it calls, and what depends on it.

It is not a search tool. For locating definitions or finding files, plain grep is measurably cheaper. The Claude Code plugin encodes this boundary so agents use each tool where it wins.

## Supported languages

Go, TypeScript, JavaScript, Python, and PHP.

Call resolution is name-based. When two symbols share a name, results carry an `[ambiguous]` flag so you know to verify by file before trusting them. Anonymous functions and lambdas are not indexed as symbols.

Note: the measured token savings above come from Go repositories. The other languages use the same mechanics and pass the same engine validation, but have not been through the same agent experiments yet.

## Install

You need Go 1.24 or newer and a C toolchain (the tree-sitter and SQLite bindings use CGO). On macOS, the Xcode command line tools provide this.

```sh
git clone https://github.com/ethanhinson/codeindex.git
cd codeindex
go build -o /usr/local/bin/codeindex ./cmd/codeindex
```

Run the tests if you want to confirm the build:

```sh
go test ./...
```

## Quick start with your own repo

Point codeindex at any repo root. The first query builds the index automatically, so you can skip straight to asking questions:

```sh
codeindex callers /path/to/your/repo SomeFunction
codeindex impact  /path/to/your/repo SomeFunction
codeindex find    /path/to/your/repo handler --kind function
```

If you prefer to build up front (useful for big repos, so the first query is not slow):

```sh
codeindex build /path/to/your/repo --progress
codeindex status /path/to/your/repo
```

The index lives in `<repo>/.codeindex/graph.db`. You will probably want to add `.codeindex/` to your gitignore.

Every query checks for changed files first and patches the index before answering, so results are always fresh. There is no manual refresh step in normal use.

## Commands

```
codeindex build <repo>                    build or rebuild the index
codeindex refresh <repo>                  patch the index for changed files
codeindex status <repo>                   index stats (add --json for machine output)

codeindex callers <repo> <symbol>         who calls this symbol
codeindex impact <repo> <symbol>          blast radius: callers plus callees, counts first
codeindex dependents <repo> <anchor>      what depends on this file or package
codeindex deps <repo> <anchor>            what this file or package depends on
codeindex find <repo> <query>             symbol search (--kind, --path, --limit)
codeindex grep <repo> <pattern>           pattern search over indexed symbols
codeindex enclosing <repo> <file> <a>:<b> which symbol encloses these lines

codeindex export <repo> <out.db>          compact index artifact for sharing
codeindex import <repo> <artifact.db>     install an artifact, then patch local drift

codeindex mcp <repo>                      serve the index over MCP (stdio)
codeindex serve <repo> [--addr host:port] headless JSON graph API (default 127.0.0.1:7676)
codeindex bench <repo> [out.json]         throughput benchmark and incremental-vs-full check
```

Most query commands take `--limit N` (default 50).

## Graph API: query the symbol graph over HTTP

`codeindex serve <repo>` exposes the project's symbol call graph as a headless,
read-only JSON API over loopback HTTP — no static hosting, just data other tools
can consume. It freshens the index on start and binds to `127.0.0.1:7676` by
default (`--addr host:port` to change it). Every graph response carries a
top-level `schemaVersion` so external consumers are insulated from internal
shape changes.

```sh
codeindex serve /path/to/your/repo
# GET /api/health            -> { "status": "ok", "version": "<build>", "root": "<repo>" }
# GET /api/graph?symbol=Foo  -> Foo's neighborhood: focus + direct callers + callees
# GET /api/graph/full        -> the whole symbol graph (nodes grouped by package dir)
```

The full request/response contract — node and edge shapes, the `parent`
disambiguator, and the versioning policy — is documented in
[docs/graph-api.md](docs/graph-api.md).

## Using it with Claude Code

The `plugin/` directory ships a Claude Code plugin with three pieces:

- A per-prompt note that tells the agent the index exists and when to use it.
- A post-edit hook that warns the agent when it edits a function with callers elsewhere (once per symbol per session, never blocks edits).
- An `/codeindex:impact <symbol>` command for a quick blast-radius summary before changing something.

Install:

```sh
claude --plugin-dir /path/to/code-indexer/plugin
```

The plugin needs the `codeindex` binary on your PATH (or set `CODEINDEX_BIN`). To silence the post-edit hook, run `touch .codeindex/hook-disabled` in a repo, or set `CODEINDEX_HOOK_DISABLE=1` globally.

See `plugin/README.md` for details and the measurement history behind the design.

## Using it with Cursor, VS Code, or Claude Desktop

`codeindex mcp <repo>` serves `impact`, `callers`, and `callees` to any MCP client over stdio. The tool descriptions carry the usage guidance, so IDE agents pick up the discipline automatically.

Cursor (`.cursor/mcp.json` in the repo, or `~/.cursor/mcp.json` globally):

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

VS Code (`.vscode/mcp.json`):

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

Claude Desktop uses the same shape in `~/Library/Application Support/Claude/claude_desktop_config.json` with an absolute path to the binary.

## File type detection

Built-in extensions cover Go, TS/JS (including `.mjs` and `.cts`), Python (including `.pyi`), and PHP (including `.phtml`). On top of that, content detection routes files by PHP open tags and php/python/node shebangs, so `.inc` files, `.module` files, and extensionless scripts work with zero config. A Drupal clone indexes correctly out of the box.

For explicit control, commit a `.codeindex.json` at the repo root:

```json
{"associations": {"*.theme": "php", "legacy/*.tpl": "php"}}
```

Associations beat extensions, and extensions beat content sniffing. An unknown language name fails the build loudly rather than being silently skipped.

## Dependencies

Calls into vendored or installed dependencies can be resolved by building a
dependency map and indexing it:

```sh
codeindex depmap /path/to/dep --namespace <ns> --version <v> -o dep-map.db
```

Resolved dependency symbols show `[dep namespace@version]` provenance, and
locally modified dependency files overlay the attached map and are marked
`modified`.

## Teams and CI

Build the index once in CI, publish the artifact, and let everyone import it instead of building cold:

```sh
codeindex export <repo> index-artifact.db     # in CI
codeindex import <repo> index-artifact.db     # on each machine
```

On the Kubernetes repo, that turns an 82.5 second cold build into a 1.5 second import. See [docs/ci.md](docs/ci.md) for a full setup.

## Repository layout

```
cmd/codeindex        the CLI
internal/adapter     language adapters (tree-sitter based)
internal/graph       SQLite store, data model, name resolution
internal/merkle      file walking, content hashing, change detection
internal/engine      build and incremental patch orchestration
internal/query       query layer (auto-build and patch-on-query)
internal/readmodel   symbol read model behind the graph API
internal/webserver   headless JSON graph API (codeindex serve)
internal/mcpserver   MCP server
internal/depmap      dependency map generation
plugin/              Claude Code plugin (hooks and /impact command)
editors/vscode       VS Code integration
bench/               benchmarks and A/B experiment findings
docs/                CI setup, the graph API contract, and design docs
```
