<!-- docket:backlink:start (generated — do not hand-edit) -->
> ↩ **[Change 0009 — Workspace manifest load/validate + init-workspace --scan](https://github.com/ethanhinson/codeindex/blob/docket/docs/changes/active/0009-workspace-manifest-init-scan.md)**
<!-- docket:backlink:end -->

# Workspace manifest load/validate + `init-workspace --scan` — results

Change: #0009 · Branch: feat/workspace-manifest-init-scan · PR: (opened by this run) · Plan: docs/superpowers/plans/2026-08-18-workspace-manifest-init-scan-plan.md · ADRs: none (the spec's settled design explicitly calls for no new ADR)

## Verify (human)

- [ ] **The bench manifest is checkout-state-dependent and has no automated guard.** `bench/repos/oss-ws/.codeindex/workspace.json` was filled by running the new verb against the 10 corpus checkouts, which are gitignored and therefore not reproducible from a clean clone. If any member is re-pinned to a different upstream commit, the committed namespaces silently drift. Confirm you are content with that, and with the manifest's contents, by eye.
- [ ] **The D7 merge-gate interpretation is this branch's merge authorization.** `openspec/changes/workspace-graph/design.md` now carries a dated 2026-08-18 amendment reading the frozen SHALL (`specs/workspace-graph/spec.md:125`) as gating *query-behavior* slices, so this slice may merge ahead of the evidence gate while the gate still hard-blocks §3.3+ and §4. If you disagree with that reading, the whole branch is blocked, not just that commit.
- [ ] **`.gitignore` narrowness.** The negation admits exactly one path. Re-confirm with `git status --porcelain -uall bench/repos/` — expect no member checkout, and nothing from `bench/repos/btt-ws-private/`.

## Findings

**Prefix-containment verification of the bench bootstrap (spec §4a.2 pins vs. the filled manifest).** All 10 members pass; exactly the 2 the spec predicted fail *element*-containment, confirming Assumption 9 against real data:

| member | pin | element? | discovered namespaces |
|---|---|---|---|
| symfony | `Symfony\` | **no** | `Symfony\Bridge\Doctrine\`, `Symfony\Bridge\Monolog\`, `Symfony\Bridge\PsrHttpMessage\`, `Symfony\Bridge\Twig\`, `Symfony\Bundle\`, `Symfony\Component\`, `Symfony\Runtime\Symfony\Component\`, `symfony/symfony` |
| drupal | `Drupal\` | **no** | `Drupal`, `Drupal\Component\`, `Drupal\Core\`, `drupal/core` |
| laravel | `Illuminate\` | yes | `Illuminate\`, `Illuminate\Support\`, `laravel/framework` |
| nest-common | `@nestjs/common` | yes | `@nestjs/common` |
| nest-core | `@nestjs/core` | yes | `@nestjs/core` |
| nest-microservices | `@nestjs/microservices` | yes | `@nestjs/microservices` |
| werkzeug | `werkzeug` | yes | `werkzeug` |
| flask | `flask` | yes | `flask` |
| client_golang | `github.com/prometheus/client_golang` | yes | `github.com/prometheus/client_golang` |
| prometheus | `github.com/prometheus/prometheus` | yes | `github.com/prometheus/prometheus` |

The bare `Drupal` entry is not a defect — `drupal/package.json` declares `"name": "Drupal"` (Drupal core ships a JS build), a legitimate node-probe hit.

**Plan/spec deviation — the `.gitignore` block as specified is inert.** Spec §4a.1 and plan Task 9 both prescribe `bench/repos/` followed by three `!bench/repos/oss-ws/...` negations. That form does not work and was verified not to: with `bench/repos/` excluding the directory, git never descends into it, so the negations are unreachable and `git check-ignore -v` still reports the manifest as ignored by `.gitignore:53`. The implemented form uses the re-include/re-exclude-per-level idiom (`bench/repos/*`, `!bench/repos/oss-ws/`, `bench/repos/oss-ws/*`, `!bench/repos/oss-ws/.codeindex/`, `bench/repos/oss-ws/.codeindex/*`, `!bench/repos/oss-ws/.codeindex/workspace.json`), which is strictly narrower than the intent required — everything inside `oss-ws` other than `workspace.json` stays ignored. Verified adversarially: 14 fabricated decoy checkouts (including a `btt-ws-private` decoy and a nested decoy `oss-ws/decoy/.codeindex/workspace.json`) were all correctly excluded, and deliberately defeating the guard with `!bench/repos/oss-ws/**` turned it red at 4 admitted paths. **Spec §4a.1 and its acceptance-checklist wording should be corrected to the working form** — the spec lives on the `docket` branch and was not editable from the feature worktree.

**Transitive dependency movement beyond the spec's stated footprint.** Spec §2c / Assumption 14 reasons only about the two new *direct* requires (`golang.org/x/mod`, `gopkg.in/yaml.v3`) being pure Go and ADR-0003-safe. `go mod tidy` additionally bumped `golang.org/x/sys` 0.41.0 → 0.47.0 and `golang.org/x/tools` 0.42.0 → 0.49.0. Almost certainly benign, but `x/sys` sits under the sqlite/tree-sitter stack, so it is on the record rather than silent.

**Test-only seam introduced in production code.** `internal/workspace/namespaces.go` now carries `var namespacesProbe = Namespaces`, used by `membersAndEscaped` and `namespacesAt` so a counting stub can pin the double-probe fix. It is never reassigned outside a `t.Cleanup`-restored test stub. If a reviewer objects to the pattern, only the test needs removing.

**Guard-2 micro-divergence.** Switching guard 2's marker presence test from `readMarker` to `os.Stat` means a *directory* named exactly like a marker (e.g. a dir called `go.mod`) is now simply not a marker, where it previously surfaced an EISDIR error out of `Members`. Non-ENOENT stat errors still propagate. No test exercised the old behavior and it was arguably a bug.

**Suite command.** `go test ./...` cannot pass in this environment for a pre-existing, environmental reason unrelated to this change: the vendored `llama.cpp` headers are absent, so the CGO shim in `internal/embed` fails to build and takes 10 packages down with it — including `internal/query`, which holds the three golden tests the regression bar names. Measured on both this branch and `origin/main` (bb076aa): `go test -tags nollama ./...` yields exactly one failure, `TestSearchToolAndPrompt`, on both refs. The branch adds zero new failures and `internal/query` passes under the tag. **Recommended: pin `go test -tags nollama ./...` as `finalize.test_command` in `.docket.yml`**, so the merge gate and every future build agree rather than each re-deriving it. Left unset deliberately — that is a repo-level owner decision, not an agent's.

## Follow-ups

- **Correct spec §4a.1's `.gitignore` block** (on the `docket` branch) to the working re-include idiom, and the matching acceptance-checklist item.
- **Discovery does not emit out-of-root members.** `go.work use ../shared` and composer `{"type":"path","url":"../shared-lib"}` are canonical multi-repo forms, and D1 sanctions `../` roots — but *discovery* deliberately drops candidates that resolve outside the workspace root, since emitting them would change the committed bench manifest and reaches past this slice. This run made the omission honest rather than silent (the empty-result error now distinguishes "nothing declared" from "everything declared resolved outside the root," and names an escaping path), and documented it on `Members`. Whether discovery *should* emit them is an open design question for change 0010 or a successor.
- **`filepath.Glob` has no `**` support** and pnpm `!`-negation entries no-op. `packages/**` and `apps/*/` are routine in pnpm/yarn declarations, so nested members are silently missed. Documented on `Members`; organic coverage is deferred to change 0010 (bench corpus monorepo growth).
- **Empty-namespace members are now possible in a manifest.** Guard 3 no longer deletes a discovered member whose namespace set is empty (it previously did, because the empty set is a strict subset of every non-empty one). No corpus member triggers it today, but any consumer assuming every member contributes at least one namespace now has an input that violates that.
- **`DetectRootKind` has no non-test call site** in this slice, by design — openspec §4.2 owns the CLI wiring behind the byte-identical golden gate. A future `unused`-style linter run could flag `hasIndexableSource` until then.
