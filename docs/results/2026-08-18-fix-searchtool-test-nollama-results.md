<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0011 — Make TestSearchToolAndPrompt honest under nollama builds](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0011-fix-searchtool-test-nollama.md)**
<!-- docket:backlink:end -->

# Make TestSearchToolAndPrompt honest under nollama builds — results

Change: #0011 · Branch: feat/fix-searchtool-test-nollama · PR: (opened at close of this run) · Plan: docs/superpowers/plans/2026-08-18-fix-searchtool-test-nollama-plan.md · ADRs: 11

## Verify (human)

- [ ] On an **embedding-capable** machine (vendored llama.cpp headers present, plain `go test ./...`
      builds), run `go test ./internal/mcpserver/ -run TestSearchToolAndPrompt -v` and confirm the
      test takes the **no-marker** branch — i.e. it asserts `Helper` surfaces semantically and does
      **not** print the `lexical-only build (...)` log line. This branch is unreachable in the
      environment this change was built in, so it is verified only by inspection and by mutation
      testing of the reason classifier. It is the one assertion this change is meant to protect.

## Findings

- **The reported failure was a test-honesty gap, not a product bug — root cause discharged.**
  The change file directed an abort if lexical-only search *should* match `helper` → `Helper` and
  didn't. It does match. Verified with a `-tags nollama` binary against the same fixture:
  `find <repo> helper` → `Helper  func  a.go:2  callers=2  [exact]`, while
  `find <repo> "helper increment function"` → 0 matches plus the built-in guidance
  *"this looks like a concept query; use `search` for feature/topic questions"*.
  `matchQuality` (`internal/search/find.go`) lowercases and stems both sides, so matching is
  case-insensitive; the ladder is **conjunctive**, requiring every query stem, so a 3-token
  conceptual phrase correctly fails to match a symbol named `Helper`. No product change was made —
  the branch touches one test file and this repo's own docs.

- **Review caught a false negative in the first cut of the fix (important).** Keying the capability
  branch on the mere *presence* of the `[lexical-only: ` marker would have silently absorbed the
  `"query embedding failed: <err>"` degradation — a build that IS embedding-capable whose embed call
  broke at runtime — into the "honest degradation, skip and log" path. That is precisely the
  regression the test exists to catch. Fixed in `dbcdd1f`: the skip is now gated on the degradation
  **reason**, and any unexpected reason fails the test. Proven by mutation.

- **Became ADR-0011** — *Capability-dependent tests assert against the product's own degradation
  disclosure*. Records the runtime-detection decision over a `//go:build nollama` test split, the
  rule that the disclosure's reason is load-bearing rather than its presence, and the rule that a
  degraded branch must still assert something build-independent. Alternatives (build-tag split,
  whole-test `t.Skip`) and their rejection are captured there.

## Follow-ups

- **Hints are conjunctive, not independent identifier guesses** (observed while root-causing;
  deliberately left out of scope per the change's *Out of scope* — "Improving lexical-fallback
  ranking or matching"). `search.Semantic` joins hints into one space-separated query
  (`hintQ := strings.Join(opts.Hints, " ")`, `internal/search/semantic.go`) and feeds it through the
  same conjunctive ladder, so a multi-hint call only matches symbols carrying **every** hint token.
  In the failing fixture, hints `["helper","increment"]` therefore matched nothing even though
  `helper` alone matches `Helper` exactly. Whether hints should be scored independently and merged
  is a real product question worth its own change. `auto_capture` is disabled in this repo, so this
  is recorded here rather than minted as a stub.

- **Reason classification is substring-based.** A future fourth `degraded =` site whose wording
  happens to contain "no embedding provider" or "index has no embeddings" would silently re-open the
  skip. The three current sites are enumerated in a code comment to keep that visible, and the cost
  is recorded in ADR-0011's consequences.

- **Environmental, untouched by this change:** plain `go test ./...` still fails ~10 packages on
  every ref for want of vendored llama.cpp headers. `go test -tags nollama ./...` is the honest local
  gate and is pinned in `.docket.local.yml`; restoring the CGO embed build was explicitly out of
  scope here.
