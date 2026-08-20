---
id: 12
slug: workspace-freshness-re-resolves-whole-workspace
title: Workspace freshness re-resolves the whole workspace, not the incident edge set
status: Accepted
date: 2026-08-19
supersedes: []
reverses: []
relates_to: [5, 6]
change: 14
---

## Context

Design D2 of the workspace-graph work states the freshness contract as: for each member run the
per-repo freshen, then "for each member whose merkle root differs from its stamp, re-resolve only
overlay edges incident to that member."

Change 0014 implements that contract as `internal/wsfresh.Freshen`, built on top of change 0013's
frozen `wsresolve.Resolve(wsRoot) (Stats, error)`. The literal incident-scoped reading of D2 cannot
be implemented on that frozen API — and, more importantly, the incident set is not the correct unit
of re-derivation even if the API allowed scoping.

## Decision

When any member is dirty — its stamp is absent, or unequal to a freshly folded `MemberMerkleRoot` —
**or** the registry has drifted from the manifest, `Freshen` runs exactly **one whole-pass**
`wsresolve.Resolve`. There is no per-member or incident-scoped re-resolution path.

The clean branch — no dirty member and no drift — performs **zero overlay CONTENT writes**, which is
where D2's observable "unchanged members are cheap" contract is actually satisfied.

Two independent reasons the incident set is not closed under re-derivation:

1. **The dirty member is not the only member whose derivation changed.** An edge `S -> M` is derived
   from source `S`'s unresolved-edge list resolved against `M`'s definitions; the deriving call is
   scoped by **source `S`**, not by the dirty member `M`. Every member that could source an edge into
   `M` must re-run.
2. **The blast radius escapes the incident set.** If a hint in `S` that previously resolved uniquely
   in clean member `O` now also resolves in dirty `M`, the ladder's answer changes from an exact
   `S -> O` cross-edge to an ambiguity. The row that must change has endpoints `S` and `O` — neither
   dirty.

A third, weaker note rules out only the **naive** loop, not scoping in general:
`overlay.ReplaceMemberEdges` deletes rows incident to a member on either endpoint, so a
derive-and-write-per-member loop deletes what the previous iteration just wrote — the self-clobbering
shape change 0013 already shipped and fixed.

## Consequences

- **Cost.** The dirty case is a full overlay rebuild. D2's own risk note explicitly bounds that worst
  case, and bounds it by the unresolved-edge count rather than the symbol count.
- **The clean case is not free.** Its standing cost is honestly a per-member `query.Fresh` plus a full
  `MemberMerkleRoot` fold — NOT literally "one stamp comparison" as D2's prose says. Cleanliness is
  unknowable without folding. This is the number the D7 latency measurement should use.
- **Never-thin semantics and crash self-healing are preserved unchanged**, because the re-resolution
  path IS change 0013's pass: stale ambiguity records are deleted whole and re-derived, and stamps are
  written last, so a pass that dies mid-write leaves affected members stampless — which the next
  `Freshen` reads as dirty.
- **Scoping stays available as a later, measured optimization.** A source-closure re-resolution was
  rejected *now*, not forever: it needs a reachability model over declared deps that the ladder's
  later rungs may exceed — new correctness surface for an unmeasured win. The D7 gate is where its
  cost would be justified.
- **Rejected alternatives.** The per-dirty-member `ReplaceMemberEdges` loop (self-clobbering); a
  computed dirty-source-closure scoped derive (sound in principle, unmeasured); widening `Resolve`'s
  frozen signature to accept pre-resolved state (re-litigates 0013).
- **Accepted duplication.** `Freshen` re-derives root kind, manifest, presence, and the member opens
  that `Resolve` then redoes — accepted because `Resolve`'s signature is frozen; the duplicated opens
  cost only on the dirty path.
