---
slug: gitignore-negation-needs-per-level-reinclude
hook: "A `dir/` ignore rule makes every nested `!` negation unreachable — re-include per level or the un-ignore is inert."
topics: [git, gitignore, verification]
changes: [9]
created: 2026-08-19
updated: 2026-08-19
promotion_state: candidate
promoted_to:
---

## Apply

To commit exactly one file out of an otherwise-ignored directory tree, `dir/` followed by
`!dir/sub/file` **does not work**, and it fails silently. Git never descends into a directory
excluded by a rule ending in `/`, so no negation below it is ever evaluated. The plausible-looking
form is inert, and nothing warns you.

Use the re-include/re-exclude-per-level idiom — one pair per path segment, star the contents rather
than the directory:

```gitignore
bench/repos/*
!bench/repos/oss-ws/
bench/repos/oss-ws/*
!bench/repos/oss-ws/.codeindex/
bench/repos/oss-ws/.codeindex/*
!bench/repos/oss-ws/.codeindex/workspace.json
```

Two rules follow from this:

- **A spec or plan that prescribes the `dir/` + nested-negation form is wrong.** Implement the
  working idiom, and record the deviation — the authored artifact is not self-verifying here, and
  the reviewer of the diff will read the prescribed form as correct.
- **Verify the rule adversarially in both directions, never by eye.** `git check-ignore -v <path>`
  reports the exact rule and line that decided, which is the only reliable signal that a negation
  fired. Then prove the guard is narrow: fabricate decoy paths (siblings, nested lookalikes, a
  decoy under a *different* ignored tree) and confirm every one stays ignored, and deliberately
  over-broaden the rule (e.g. `!dir/sub/**`) to confirm the check actually reddens. An
  un-mutation-tested ignore rule is decoration in the same way an un-mutation-tested guard is.

## War story

- 2026-08-19 (#9, PR #8) — the workspace-manifest slice needed exactly one file,
  `bench/repos/oss-ws/.codeindex/workspace.json`, committed out of the gitignored
  `bench/repos/` corpus tree (which also holds private-repo checkouts that must never be
  committed). Both the design spec §4a.1 and the plan's task prescribed `bench/repos/` plus three
  `!bench/repos/oss-ws/...` negations. That form is inert: with the directory itself excluded, git
  never descends, and `git check-ignore -v` still attributed the manifest to the bare
  `bench/repos/` rule. The build substituted the per-level idiom, which is also strictly *narrower*
  than the intent required — everything inside `oss-ws` other than the one manifest stays ignored.

  Verified adversarially rather than by reading: 14 fabricated decoy checkouts — including one
  under the private `btt-ws-private/` tree and a nested `oss-ws/decoy/.codeindex/workspace.json` —
  were all confirmed still excluded, and deliberately defeating the guard with `!…/oss-ws/**`
  turned the check red at 4 wrongly-admitted paths. Without that second half, "the file is
  committed" would have been mistaken for "the rule is correct," on a tree where the failure mode
  is committing private-repo data.
