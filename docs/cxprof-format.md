# cxprof v1 — the codeindex runtime profile format

One page, on purpose. If your program can write JSON lines, it can emit
cxprof. Our SDKs are conveniences — the format is the contract, and it is
free to implement.

## Shape

A cxprof file is UTF-8 JSONL: one **header** record, then any number of
**stack** records.

```jsonl
{"cxprof":1,"lang":"php","unit":"samples","hz":99,"start":1770000000,"end":1770000060,"commit":"abc123","tag":"dev"}
{"st":[["src/Hooks.php",210],["src/Billing/Invoice.php",44]],"n":17}
{"st":[["index.php",12]],"n":3}
```

### Header fields

| field | required | meaning |
|---|---|---|
| `cxprof` | yes | format major version; this document describes `1` |
| `lang` | yes | emitting runtime: `go`, `node`, `python`, `php`, ... |
| `unit` | yes | always `samples` in v1 |
| `hz` | yes | sampling frequency (approximate is fine) |
| `start`,`end` | yes | unix seconds covered by this profile |
| `commit` | no | VCS revision of the profiled code, if known |
| `tag` | no | free-form deployment tag (`dev`, `prod`, ...) |

### Stack records

- `st`: frames as `[file, line]`, **innermost last** (leaf is the final
  element). `file` is repo-relative when the emitter knows the repo root,
  otherwise absolute (ingestion re-roots against the index).
- `n`: how many samples observed this exact stack (≥1).

Emitters SHOULD aggregate identical stacks; ingesters MUST tolerate
duplicates (counts add).

## Rules

1. **Frames only.** No argument values, no environment, no request data —
   nothing but file, line, count. Conformance checking rejects extra
   per-frame payloads.
2. **Unknown fields are ignored**, never errors. Evolution inside v1 is
   additive-only; anything breaking bumps the major.
3. **Truncated files are usable**: each line stands alone; ingesters keep
   every parseable line and report what they skipped.

## Emitting into codeindex

Drop files matching `*.cxprof.jsonl` into `<repo>/.codeindex/runtime/`
(write to a temp name, then rename). The next codeindex query ingests them
automatically; `codeindex ingest <file|dir>` does it explicitly, and
`codeindex ingest --check <file>` validates conformance without ingesting.

Sampled truth caveat, stated once and inherited everywhere: presence of a
stack is evidence; absence of one proves nothing.
