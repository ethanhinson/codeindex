## Extra tool available: `codeindex`

This repository has a prebuilt code index. Before you reach for Grep or read
whole files to answer "where is X defined?", "who calls / references X?", or
"what would changing X affect?", query the index — it returns compact
`file:line` references instead of forcing you to read files.

Run it with Bash (it is already built):

```
{CODEINDEX_BIN} query {REPO_PATH} <SymbolName> --limit 40
```

It prints the symbol's definition(s) and its callers as `path:line` lines. Example:

```
$ {CODEINDEX_BIN} query {REPO_PATH} CreateTestContext --limit 3
def  test_helpers.go:10  func CreateTestContext(w http.ResponseWriter) (c *Context, r *Engine)
callers (137):
  context_test.go:82  TestContextFormFile
  context_test.go:103  TestContextMultipartForm
  ... (+134 more; use --limit)
```

Interpretation: `CreateTestContext` is defined at `test_helpers.go:10` and is
referenced from 137 sites (the top ones shown). That answered "where is it and
who uses it" without reading a single file.

When to use it:
- Locating a definition → one query beats grepping then opening the file.
- Finding callers / references / blast radius of a symbol.
- Deciding which files an issue touches → query the symbols the issue names.

Caveats (be aware, still useful):
- Go symbols only; resolution is by name, so a common method name may list
  same-named matches flagged `[ambiguous]` — cross-check with the file:line.
- Raise `--limit` if you see `(+N more)` and need the full list.

You still have Bash, Read, Grep, and Glob — use whichever is best. The index is
just usually the cheapest way to navigate.
