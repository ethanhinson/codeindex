# @codeindex Go agent

In-process CPU sampling (stdlib `runtime/pprof`) spooled as
[cxprof v1](../../../docs/cxprof-format.md) into `<repo>/.codeindex/runtime/`,
where the next codeindex query ingests it automatically.

```go
import "codeindex/sdk/go/agent"

func main() {
    stop := agent.Start(agent.Options{Repo: os.Getenv("REPO_ROOT")})
    defer stop()
    // ... your app ...
}
```

Contract: sampling only (~99Hz default), frames-only payloads, bounded
buffers with whole-profile drop, failures swallowed and counted
(`agent.Dropped`), `CODEINDEX_PROFILING=off` disables everything.
