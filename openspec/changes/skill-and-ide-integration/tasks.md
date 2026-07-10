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

- [x] 3.1 edit_impact task type + tasks_v3.json with pre-registered gate thresholds
- [x] 3.2 run_ab --plugin-arm (real plugin via --plugin-dir; hook fires logged per run)
- [x] 3.3 report per-type breakdown + gate evaluation; dashboard v3 section
- [x] 3.4 Smoke caught adoption collapse (lazy skill, v3a archived); registered iteration applied (UserPromptSubmit visibility note + exact-command hook tip + keyword description); post-iteration smoke showed full chain working; full run executed
- [x] 3.5b v4 re-run (user-approved after FAIL analysis): stripped plugin (cut skill + /callers + /callees; kept post-edit hook + 155-token trust-carrying prompt note + /impact) — **GATE PASS, all four thresholds** ($4.37, shared v3 control): locate −7.4% ✅, branch-out +62.3% ✅, hook 100% ✅, false fires 0 ✅; accuracy B 100% vs A 93.8%; callattr behavior 2 turns/0 reads. Plugin ships in v4 shape. FINDINGS_v4.md
- [x] 3.5 v3 GATE: **FAIL** ($5.58, 64 runs). Locate −43.9% (cause: ~3.1k-token static plugin footprint in cache_creation — NOT mis-triggering; discipline held at 0% locate adoption). Branch-out −11.3% despite 100% adoption (cause: trust deficit — agent re-read ~6 files after querying; plus Skill round-trip). Hook thresholds PASSED (100% fire, 0 false) and edit tasks +28.9%; accuracy B 100% vs A 93.8%. Iteration budget spent — v4 (strip to hook + trust-instructing note, <500-token footprint) requires approval. Full analysis: bench/agent_ab/FINDINGS_v3.md

## 4. Phase 2 — MCP server

- [ ] 4.1 `codeindex mcp <repo-root>`: stdio server (pinned official Go MCP SDK) with `impact`/`callers`/`callees` tools, anchor-rule descriptions, fresh-on-query behavior
- [ ] 4.2 In-process mutex serializing re-check writes; test concurrent tool calls during a pending edit
- [ ] 4.3 Client config snippets (Cursor, Claude Desktop, VS Code) in plugin/README; verify against at least one real client manually
- [ ] 4.4 Integration test: MCP handshake + tools/list + one callers call against a fixture repo

## 5. Verification

- [ ] 5.1 `openspec validate skill-and-ide-integration`; mark core-indexing-engine 7.1/7.2 satisfied; update roadmap (rename mcp-and-plugin → this change)
- [ ] 5.2 README + dashboard updated with v3 results; all Go tests + hook script tests pass
