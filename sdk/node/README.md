# @codeindex/agent (Node)

In-process CPU sampling via the built-in `node:inspector` V8 profiler — no
dependencies — spooled as [cxprof v1](../../docs/cxprof-format.md) into
`<repo>/.codeindex/runtime/`, where the next codeindex query ingests it
automatically.

```js
const agent = require('@codeindex/agent');
const stop = agent.start({ repo: process.env.REPO_ROOT });
// ... your app ... (stop() is also wired to beforeExit)
```

Contract: sampling only (~99Hz default), frames-only payloads, failures
swallowed and counted (`agent.agent.dropped`), `CODEINDEX_PROFILING=off`
disables everything, atomic temp+rename spool writes.
