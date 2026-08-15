# PHP runtime-evidence lab (Excimer -> cxprof)

This lab proves that codeindex can bridge PHP hook dispatch with runtime
evidence. The app is a minimal WordPress-style hook framework:
`add_action('invoice.settled', 'handle_invoice_settled')` registers handlers
in a string-keyed registry and `do_action('invoice.settled', ...)` fires
them. That indirection is invisible to static analysis — no call edge in the
source connects the dispatcher to the business handlers. Sampling the app
under Excimer produces stacks like

```
index.php:21 -> hooks.php:28 (do_action's $handler(...)) -> handlers.php:13 (handle_invoice_settled)
```

which codeindex ingests as **observed edges**, clustering the dispatcher and
handlers together at search time.

Everything PHP runs inside a container (php:8.3-cli + pecl excimer); no host
PHP required.

## Layout

- `Dockerfile` — php:8.3-cli with the Excimer sampling profiler.
- `app/` — the profiled mini app: `hooks.php` (registry), `handlers.php`
  (three CPU-heavy business handlers), `index.php` (registers handlers,
  dispatches events for ~2s via `run_event_loop`).
- `adapter.php` — wraps the run in ExcimerProfiler (5ms period, wall clock,
  depth 100) and converts the ExcimerLog to cxprof v1 JSONL
  (see `docs/cxprof-format.md`), written atomically to `/spool/`.
- `run.sh` — builds the image, runs the adapter with `app/` mounted at
  `/app` and `spool/` at `/spool`, then copies the profile into
  `app/.codeindex/runtime/`. Fails unless the profile has >=5 stack records.
- `verify.sh` — host side: builds `/tmp/codeindex-selfheal` if missing, then
  `codeindex build app`, `codeindex ingest app` (must report observed
  edges), and `codeindex search app "..."` (must show `[observed ...]`).

## Run it

```sh
bench/selfheal/php/run.sh && bench/selfheal/php/verify.sh
```

## Excimer notes (verified in-container, php 8.3 + excimer 1.2.x)

- `ExcimerLogEntry::getTrace()` returns frames **innermost-first** (index 0
  is the leaf) — the opposite of cxprof's innermost-last, so the adapter
  reverses each stack.
- Each frame is `['file' => ..., 'line' => ..., 'function' => ...]`;
  internal frames can lack `file`/`line`, so the adapter guards for that.
- `getEventCount()` is the number of sampling events an entry represents;
  the adapter sums it while aggregating identical stacks.
- Paths inside the container are `/app/...`. The adapter strips the `/app/`
  prefix so frames are repo-relative (`handlers.php:13`) and resolve against
  the host-side index; frames outside `/app` (the adapter itself, internals)
  are dropped.
