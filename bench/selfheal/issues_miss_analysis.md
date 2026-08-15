# Miss analysis — issues-closed v2 fixtures (search hit@5 misses)

Scope: all 55 search misses (gin 35/39, flask 20/30) from
`bench/results/issues-v2-{gin,flask}.json`, scored with
`/tmp/codeindex-selfheal search --flat --limit 5`.

Evidence per miss collected by `bench/selfheal/miss_evidence.py`
(`.miss_evidence.json`): accept ranks in top-15, string literals containing
distinctive title words within/adjacent to accept spans, presence of accept
names in the fix diff, title/accept lexical overlap. Buckets assigned with
precedence **A > D > B > C > E** (a rank-6..15 miss is counted as D even if
literal evidence also exists).

## Bucket counts

| bucket | gin | flask | total |
|---|---|---|---|
| A ground-truth-still-wrong | 2 | 0 | 2 |
| B symptom-literal-present | 5 | 2 | 7 |
| C multi-hop | 16 | 7 | 23 |
| D retrievable-but-outranked (6-15) | 4 | 7 | 11 |
| E other | 8 | 4 | 12 |
| **total** | **35** | **20** | **55** |

## A — ground-truth-still-wrong (2)

Both are v2 mapping artifacts of the same kind: **comment-only diffs
attributed to the enclosing symbol** (hunk xfuncname context does not
distinguish code from comment lines).

- gin "Fix case of GitHub" -> accept `RouterGroup.handle` etc. Commit
  d8fb18c33b changes only doc comments: `- // Use adds middleware to the
  group, see example code in github.` -> `GitHub.` No behavior changed;
  the accept set is not an answer to anything.
- gin "Add and fix the explanation of `HandleContext`" -> accept
  `Engine.ServeHTTP`. Doc-comment edit adjacent to ServeHTTP; the symbol's
  code is untouched.

Fix idea: drop hunks whose added/removed lines are all comment/blank.

## B — symptom-literal-present (7)

The user-visible words of the title live in string literals inside (or
directly beside) the accept symbols — literal-aware cards would likely
retrieve these.

- gin "console logger HTTP status code bug fixed" -> `responseWriter.WriteHeader`;
  literal at response_writer.go:67: `"[WARNING] Headers were already
  written. Wanted to override status code"`.
- gin "fix: print headers without Authorization header on broken pipe" ->
  `CustomRecoveryWithWriter`; literals recovery.go:66 `'broken pipe'`,
  recovery.go:78 `'Authorization'`.
- gin "fix(router): catch-all conflicting wildcard" -> `node.insertChild`;
  literal tree.go:303: `"only one wildcard per path segment is allowed,
  has: '"`.
- gin "Fix conflict between param and exact path" -> `node`/`node.getValue`;
  literal tree.go:360: `"' conflicts with existing path segment '"`.
- gin "write: broken pipe  in func (r Data) Render" -> `RecoveryWithWriter`
  (ends recovery.go:48); literal `'broken pipe'` at recovery.go:66 in the
  sibling `CustomRecoveryWithWriter` it delegates to (adjacent, same file).
- flask "Fixes issue #2824 flask --version" -> `get_version`; literals
  cli.py:275-277: `'Python {platform.python_version()}\n'`,
  `'Flask {flask_version}\n'`.
- flask "Enable template auto-reloading in app.run()" -> `Flask`; literals
  app.py:206 `"TEMPLATES_AUTO_RELOAD"`, app.py:402 `'auto_reload'`.

## C — multi-hop (23)

Title describes surface behavior; the fix is plumbing with no lexical or
literal bridge. Largest bucket. gin: 16 — dominated by router-tree
internals (`node.getValue` / `node.insertChild`, 6 misses) and form-binding
internals (`setWithProperType`, `mapFormByTag`, 3 misses). flask: 7.

- gin "404 when using dynamic routing" -> `node.getValue`: nothing in or
  near the radix-tree walker says "404" or "dynamic routing".
- flask "sessions not saved when streaming" -> `RequestContext`: the fix
  keeps the request context open during streamed responses; no words shared.
- (also instructive) gin "Add escape logic for header" -> verified correct
  GT `redirectTrailingSlash`: the header being escaped is
  `X-Forwarded-Prefix` inside redirect handling — invisible from the title.

## D — retrievable-but-outranked (11)

Accept symbol present at rank 6-15; recall is fine, ranking is not.

- gin "fix(binding): dereference pointer to struct when validating structs"
  -> `defaultValidator.ValidateStruct` at rank 8 (top-5 filled with
  `Bind*`/`validate` test helpers).
- flask "`static_folder` can be a `pathlib.Path` object" -> `Flask` members
  at ranks 8 and 9.
- gin ranks: 8, 8, 10, 11; flask ranks: 6, 8, 9, 9, 10, 15 — most sit just
  outside the cutoff, so hit@10 would convert most of this bucket.
- Caveat: flask "Clarify the after_request argument" (rank 8) is
  mechanically D but the title names `after_request` — a locate-class leak
  (see E) that survived because v2 mapped only the class symbol `Flask`.

## E — other (12)

Two recurring shapes, both corpus-hygiene rather than retrieval problems:

1. **Meta/cosmetic commit titles with no behavioral intent** (7):
   - gin "Fix some golint warnings in gin.go" / "Fix golint warnings in
     utils.go" / "Fix 'errcheck' linter warnings" ("linter"/"golint" evade
     the `\blint\b` deny filter).
   - gin "Attempt to fix #1927" (title carries zero signal).
   - flask "Move python properties to decorator syntax",
     "fix super call in list comprehension" (internal refactor language),
     flask "Explicitly state that the jsonify method changes on request"
     (docs commit).
2. **Lexically related yet unretrieved, or locate-class leakage** (5):
   - gin "context.JSON adds a new line to end of JSON response" ->
     `WriteJSON`: shares "JSON" yet absent from top-15.
   - gin "fix bug, return err when failed binding bool" -> `setBoolField`:
     shares "bool", absent from top-15.
   - gin "recovery: fix issue about syscall import on google app engine" ->
     `RecoveryWithWriter`: top-5 shows `Recovery`, which the stricter
     qualified accept does not match.
   - gin "Fix the value of ginSupportMinGoVer constant by semantic":
     names a const that is not a tier-0 symbol (locate-class in spirit).
   - flask "@app.teardown_request function doesn't called in debug mode
     [NOT-A-BUG]": names a symbol; leaked because v2 mapped only `Flask`.

## Takeaways

- Ground truth is now largely sound: only 2/55 misses (both comment-only
  diffs) are mapping errors, vs the pervasive line-drift of v1.
- The actionable retrieval buckets are D (11: ranking, mostly rank 6-10)
  and B (7: literal-aware cards). Together they cover 33% of misses.
- C (23) is the honest hard core of the corpus: symptom language vs
  plumbing fixes needs graph or runtime reasoning, not better lexical
  matching.
- E (12) argues for tighter corpus hygiene: comment-only-hunk dropping
  (also fixes A), a "linter/refactor/docs-intent" title filter, and
  locate-class checks against ALL symbols named in the title (not just
  mapped ones).
