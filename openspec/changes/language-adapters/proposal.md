## Why

The engine, plugin, and MCP server are validated Go-only. Every consumption
surface we shipped (hook, prompt note, MCP tools) hard-codes that limit, and
the user's target stack includes TypeScript/JavaScript, Python, and PHP. The
adapter architecture was designed for exactly this expansion and is proven:
the Go adapter (tree-sitter → name-based symbols + call edges) passed
incremental==full-rebuild on kubernetes and powers the measured −62–73% agent
savings. Adding the three adapters multiplies the addressable repos without
touching the validated engine core.

## What Changes

- **Three new language adapters** following the Go adapter pattern
  (`internal/adapter/golang` is the template — `Parse(path, src)` →
  symbols + raw name-based call sites):
  - **TypeScript/JavaScript** (`.ts`, `.tsx`, `.js`, `.jsx`): functions,
    classes, methods, exported const arrow-functions; `call_expression`
    call sites (identifier + member-expression final name).
  - **Python** (`.py`): `function_definition` (module-level and methods
    inside `class_definition`), classes; `call` sites.
  - **PHP** (`.php`): functions, methods, classes; `function_call`,
    `member_call`, `scoped_call` sites.
- **Registry-driven repo walk**: `merkle.Walk` and `engine.CountLines`
  currently filter `.go` — they switch to the adapter registry's extension
  set so new languages index without touching the walk again.
- **Per-language fixtures + tests** mirroring the Go adapter tests, plus the
  engine's incremental==full-rebuild proof run against at least one real repo
  per language.
- **Real-repo validation**: nest (TS, already pinned) plus newly pinned
  Python and PHP reference repos in `bench/repos.json`; `codeindex bench`
  numbers recorded per language.
- **Consumption surfaces updated**: plugin `post_edit` hook (extension set),
  `prompt_context` note (language gating + wording), MCP tool descriptions,
  and READMEs stop saying "Go only" and say exactly what is supported.

Non-goals: precise/scope-aware resolution (stays deferred — name-based with
confidence flags, same as Go); .NET/C# (dropped from scope by the user);
re-running the agent A/B gates for new languages (the consumption mechanics
are language-independent; engine-level proof suffices for this change — noted
honestly in surfaces that cite measured numbers, which remain Go-derived).

## Capabilities

### New Capabilities

- `multi-language-adapters`: The three adapters, the registry-driven walk,
  per-language symbol/call extraction rules, per-language correctness proof
  (incremental == full rebuild), real-repo validation, and the consumption-
  surface updates (hook/note/MCP descriptions) that widen language claims
  truthfully.

### Modified Capabilities

None. (`core-indexing-engine`'s specs already describe the adapter interface
generally; its TS/JS task 4.3 is satisfied by this change. No requirement-
level behavior of existing capabilities changes — Go behavior is untouched.)

## Impact

- New packages `internal/adapter/{tsjs,python,php}`; grammar dependencies from
  the already-pinned `smacker/go-tree-sitter` module (javascript, typescript,
  tsx, python, php subpackages — no new module).
- `internal/merkle` walk and `internal/engine` line-count become
  registry-driven (behavioral change: repos now index more files; index size
  and build time grow accordingly on polyglot repos).
- `bench/repos.json` gains pinned Python and PHP reference repos.
- Plugin hook/note + MCP descriptions + READMEs updated; `core-indexing-engine`
  tasks 4.3 (TS/JS adapter) and part of 6.1 marked satisfied by this change.
