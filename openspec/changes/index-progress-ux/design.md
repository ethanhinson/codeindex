## Context

Build: walk → concurrent parse → sequential PutFile loop → ReResolveNames
(the long tail on big repos). All timing lives in engine/store; CLI and MCP
are thin. Bench calls engine directly and must stay overhead-free.

## Goals / Non-Goals

**Goals**: single source of progress truth in the engine; every surface
(TTY, JSONL, sidecar, MCP annotation) renders the same events; zero
overhead when no reporter is attached.
**Non-Goals**: TUI libraries; interactive UI; progress for queries.

## Decisions

**D1 — Reporter interface in internal/progress, composed not inherited.**
`Report(Event{Phase, Done, Total})` + `Finish(summary)`. TTY renderer
(spinner ⠋⠙⠹…, bar █░, rate, ETA, 80ms throttle, \r redraw, final ✓ line),
JSONL renderer (stdout, phase transitions always + 100ms throttle), and a
sidecar writer are independent reporters behind a multi-reporter. Engine
takes the interface; nil means today's behavior exactly.

**D2 — New engine entrypoints, old ones wrap.** BuildWithProgress /
PatchWithProgress(root, db, rep); Build/Patch delegate with nil. Store gains
a progress-callback variant of ReResolveNames (per-name granularity — it
dominates big-repo builds). Bench keeps calling the nil path: no regression
risk by construction.

**D3 — Sidecar lives next to the db and is best-effort.** status.json
written on phase transitions + 200ms throttle; failures to write never fail
a build. Terminal states: `fresh` (files, symbols, indexed_at) or removed on
schema rebuild. `codeindex status` merges sidecar + db facts (PRAGMA
user_version, counts, size) and never triggers indexing — detection must be
side-effect-free (the extension's contract).

**D4 — TTY policy.** Pretty rendering only when stderr is a character
device and TERM != dumb; otherwise throttled plain lines ("parsed
4213/11005 files"). --progress JSONL goes to stdout and suppresses pretty
output (machine feed owns the stream); human summary stays on stderr.

**D5 — Freshness annotation via Fresh's return.** query.Fresh returns
{Built, FilesParsed, Duration}. CLI prints one stderr line when Built or
FilesParsed>0; MCP prepends one line to the tool result only when Built
(cold build is the surprising case; routine patches stay quiet in MCP to
protect the token budget we exist to save).

## Risks / Trade-offs

- **ANSI on Windows terminals** → modern Windows 10+ handles VT; fallback
  is the non-TTY plain path. Recorded, not blocking.
- **Sidecar staleness after crash** → `building` with a dead started_at;
  status verb flags entries older than 10min as stale.
- **JSONL consumers vs schema drift** → events carry a `v:1` field.

## Migration Plan

None — additive. No schema change.

## Open Questions

None.
