## 1. Phase 0 — engine prerequisites

- [x] 1.1 Wire fresh-on-query: `query`/`callers`/`callees` run the incremental patch first and auto-build a missing index (satisfies core-indexing-engine 7.1/7.2)
- [x] 1.2 Implement `codeindex enclosing <repo> <file> <start>:<end>`: overlapping symbols with total and external caller counts; empty result exits 0
- [x] 1.3 Tests: query-after-edit reflects the edit; query with no index builds it; enclosing maps ranges correctly (inside symbol / outside any symbol)

## 2. Phase 1 — Claude Code plugin (`plugin/`)

- [x] 2.1 Plugin manifest (`.claude-plugin/plugin.json`) and README (install, settings, what it does/doesn't do)
- [x] 2.2 Skill (`skills/codeindex-impact/SKILL.md`): the anchor rule, positive + explicit negative triggers, the refactoring workflow (anchor → /impact → edit → hook confirms)
- [x] 2.3 Commands: `/impact` (composed counts-first summary, labels covered edge kinds), `/callers`, `/callees` — thin wrappers over the CLI
- [x] 2.4 PostToolUse hook (`hooks/hooks.json` + script): stdin JSON → .go file → git diff -U0 hunks → enclosing → inject ≤150-token note when external callers exist; per-symbol session dedup under `.codeindex/`; silent on any failure; disable setting
- [x] 2.5 Unit-test the hook script against fixture stdin payloads (symbol edit → injection; comment-only edit → silence; untracked file → silence; repeated edit → deduped)

## 3. v3 gate (integration-validation)

- [ ] 3.1 Extend `build_tasks.py` with the edit-flavored task type and a `--types` mix for v3; generate `tasks_v3.json` with pre-registered thresholds in the header
- [ ] 3.2 Extend `run_ab.py`: arm B mode using `--plugin-dir` with the real plugin (no appended system prompt); record hook injections from transcripts
- [ ] 3.3 Extend `report.py`/`dashboard.py`: per-type breakdown (locate vs branch-out vs edit), mis-trigger rate, hook fire-rate, gate evaluation
- [ ] 3.4 Smoke (2–3 tasks), inspect transcripts, fix wording/parsing, then full v3 run within budget
- [ ] 3.5 Evaluate the gate; if YELLOW, one registered iteration on skill/hook wording and re-run; record verdict + consequence in this change and the dashboard

## 4. Phase 2 — MCP server

- [ ] 4.1 `codeindex mcp <repo-root>`: stdio server (pinned official Go MCP SDK) with `impact`/`callers`/`callees` tools, anchor-rule descriptions, fresh-on-query behavior
- [ ] 4.2 In-process mutex serializing re-check writes; test concurrent tool calls during a pending edit
- [ ] 4.3 Client config snippets (Cursor, Claude Desktop, VS Code) in plugin/README; verify against at least one real client manually
- [ ] 4.4 Integration test: MCP handshake + tools/list + one callers call against a fixture repo

## 5. Verification

- [ ] 5.1 `openspec validate skill-and-ide-integration`; mark core-indexing-engine 7.1/7.2 satisfied; update roadmap (rename mcp-and-plugin → this change)
- [ ] 5.2 README + dashboard updated with v3 results; all Go tests + hook script tests pass
