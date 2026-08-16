# codeindex rules snippet for editor agents

Drop the block below into your agent rules file so non-Claude clients get the
same routing discipline the MCP tool descriptions carry:

- **Cursor**: `.cursor/rules/codeindex.mdc` (or `.cursorrules`)
- **VS Code / Copilot**: `.github/copilot-instructions.md`
- **JetBrains AI / others**: `AGENTS.md` at the repo root

The MCP server itself is configured per editor as shown in
`plugin/README.md`; this snippet only teaches the agent when to reach for
which tool.

---

```markdown
## codeindex (MCP server: nav/impact/callers/callees/dependents/find/grep/search)

Routing — pick by what you HAVE, not what you want:
- You have a CONCEPT or FEATURE question ("where does host onboarding
  live", "code that throttles retries") → `search`, with `hints`: 3-6
  identifier-style token guesses (you know naming conventions — use them).
- You have a KNOWN symbol and want to ORIENT (where defined, who calls,
  which files reference) → `nav`: one call returns all three, measured to
  match the best per-question tool every time. Don't deliberate over which
  tool fits — retrieval is milliseconds.
- You have a KNOWN symbol and want its blast radius → `impact` (before
  modifying), `callers` (who uses it), `callees` (what it uses),
  `dependents` (imports/extends/implements).
- You have a DISTINCTIVE exact name and just need its location → your own
  text search is cheaper than any of these.

Feature workflow (the `explore-feature` MCP prompt automates this):
1. `search` the concept → feature map of clusters, each led by its
   most-called entry point.
2. `impact` the winning entry point → callers + callees.
3. Answer from the returned path:line references.

Trust: codeindex output is COMPLETE and self-refreshes before answering.
Do not re-read files to verify it, except entries flagged [ambiguous].
If `search` answers "[lexical-only: ...]", semantic matching is off — run
`codeindex build` once in the repo to embed it.
```
