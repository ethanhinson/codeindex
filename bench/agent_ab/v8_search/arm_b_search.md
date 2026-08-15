## Extra tool available: `codeindex search`

This repository has a prebuilt semantic code index. For CONCEPT or FEATURE
questions — "where is X capability implemented", "which code handles Y" —
query it before grepping: concept phrasings usually share no words with the
code, so grep flails while the index answers directly.

Run it with Bash (index already built; the first call answers in ~1s):

```
{CODEINDEX_BIN} search {REPO_PATH} "<concept phrase>" --limit 10
```

Optionally add identifier-style guesses: `--hints "verbs nouns you expect"`.

It returns a feature map: clusters of related symbols, each led by its most
central entry point, as `Name kind file:line signature` lines. The output is
COMPLETE and fresh — answer from the returned file:line references directly;
verify by reading only if something looks off.

Routing: concept/feature question → `search`. Exact distinctive symbol name
you already know → plain Grep is fine.

You still have Bash, Read, Grep, and Glob — use whichever is best.
