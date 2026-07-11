## Context

resolve() is a deterministic ladder (qualified project > qualified dep > plain
project > plain dep) evaluated at insert time and reproduced by re-resolution
— incremental==full is the gate on every touch. Ambiguity baselines
(post-lexical, post-depmap): kubernetes 355,442 / laravel 64,593 / prometheus
25,647 / nest 2,455 / flask 1,065 / gin 1,668. Import edges exist per file
(kind=imports, file-level src). PutFile knows the calling file; adapters know
aliases at parse time.

## Goals / Non-Goals

**Goals**: collapse cross-package ambiguity with scope preference and
import binding; keep resolution a pure function of persisted data (equivalence
gate); measure per stage against recorded baselines.

**Non-Goals**: type inference, multi-hop re-export chasing, wildcard-import
expansion, agent A/B (engine metric per v5/dependency-edges precedent).

## Decisions

**D1 — Two stages, second contingent on measurement.** Stage 1
(scope-preference) needs no adapter changes and plausibly collapses most
same-package calls — the cheapest big win. Stage 2 (import-binding) is built
against Stage 1's measured residue, not speculation. Both stages behind the
equivalence gate.

**D2 — Derived project namespaces, filled in PutFile.** tier-0 symbols get
`namespace` = dir(path) for Go (package ≈ directory), dotted module path for
Python (`a/b/c.py` → `a.b.c`), the file path for TS/JS (module = file), and
PHP's declared `namespace X;` when the adapter captures it (fallback: dir).
Derivation lives in PutFile (it owns repo-relative paths), keyed by extension;
adapters stay parse-only. Reuses the existing `symbols.namespace` column
(dep-only until now) — no schema change for Stage 1.

**D3 — Ladder insertion, deterministic.** New project-tier steps between
qualified and plain: (a) same-file candidates, (b) same-namespace candidates.
Full ladder: qualified-t0 → qualified-t1 → import-bound (Stage 2) →
same-file → same-namespace → plain-t0 → plain-t1. Single-candidate steps
resolve unambiguous; multi-candidate steps resolve deterministic-first +
ambiguous (unchanged semantics, narrower candidate sets).

**D4 — resolve() gains caller context; re-resolution groups by file.**
Signature: resolve(tx, name, qualifier, srcFile, srcNS). Insert time has both
(pf.Path). Re-resolution currently updates all edges per (name, qualifier) in
one statement — under scope-awareness edges in different files resolve
differently, so ReResolveNames regroups per (name, qualifier, src_file).
Cost stays bounded by references-to-affected-names; measured in bench.

**D5 — Stage 2 persistence: `dst_ns` column (schema v5).** Import edges store
their source there (TS specifier, Go import path, Python module, PHP use-path)
— today that context is discarded at insert. Call edges store the Go
import-alias hint there when the selector operand matches an alias
(`util.Foo()` → `k8s.io/.../pkg/util`). Persisting both keeps resolution a
pure function of the database. Namespace mapping at resolve time: TS relative
specifiers resolve against the importing file's directory (./utils →
dir/utils.{ts,tsx,js}); Go internal paths match candidate namespaces by path
suffix (no go.mod parsing needed); Python dotted modules match derived dotted
namespaces; PHP use-paths match declared namespaces by suffix.

**D6 — Import-bound step semantics.** If the calling file has an import edge
whose bound name equals the callee (TS/Py named imports, PHP use final
segment) or whose alias hint matches (Go), candidates are constrained to the
mapped namespace. Exactly one → unambiguous; several → deterministic +
ambiguous; zero → continue down the ladder (binding never makes results worse
— same total-fallback principle as lexical qualifiers).

**D7 — Metric + gates per stage.** After each stage: six-repo
incremental==full, ambiguity counts vs baseline, full bench (query p95 —
ladder steps add per-call SQL; watch kubernetes), spot-checks (a known
same-package call now unambiguous; laravel Builder anchors unchanged).

## Risks / Trade-offs

- **Wrong-but-confident same-namespace matches** (caller actually meant an
  imported symbol shadowing a package-local name) → real languages resolve
  local-first too (Go package scope, Python LEGB after imports... Python
  imports actually shadow module-level in-file only if imported INTO the file;
  from-import binds locally — Stage 2's import-bound step sits ABOVE
  same-file for exactly this reason). Residual risk accepted and visible:
  confidence stays honest, equivalence stays enforced.
- **Ladder SQL cost per call edge** (now up to ~6 candidate probes) → each is
  an indexed lookup; build-time cost measured in bench; k8s cold build budget
  has 8× headroom.
- **Re-resolution regrouping blows up on hot names** → grouping only splits
  when scope steps could differ (same name across files); still bounded by
  reference count; measured.
- **PHP namespace capture fails on odd files** → fallback to directory
  namespace; recorded.

## Migration Plan

Stage 1: no schema change (namespace column exists); index content changes →
version bump v5 forces rebuild so all repos re-derive (same mechanism as
prior bumps). Stage 2 rides the same v5 bump (dst_ns added together up front
to avoid two rebuilds).

## Open Questions

- Whether TS index files (`./utils` → `utils/index.ts`) need resolution in v1
  — attempt, fall back to unbound (measured residue will say if it matters).
