---
slug: ordering-claims-must-survive-the-error-return
hook: "An ordering claim defended only by 'nothing clobbers anything' is unproven — the case that decides it is the error return, where the partial pass the ordering exists for actually happens."
topics: [review, invariants, crash-safety, spec-fidelity, data-integrity]
changes: [15]
created: 2026-08-20
updated: 2026-08-20
promotion_state: candidate
promoted_to:
---

## Apply

A pass with a "records first, marker last" (or stamp-last, sentinel-last, commit-marker-last) rule
has that rule for exactly one reason: **a pass that does not finish must leave the marker
contradicting the records**, because the surviving marker is the only thing that will trigger a
retry. So any argument about *where* in the pass a marker write or delete goes is an argument about
crash and error behavior — never about readability, however it is phrased.

The failure shape to watch for is a spec assumption that places an operation early, calls the
placement "a readability choice, not a correctness one", and supports that with a **clobbering**
analysis: "steps 10 and 11 only touch members drawn from `available`, and the pruned member is not
in `available`, so nothing later overwrites this." That analysis is sound and answers the wrong
question. Clobbering is what happens when the pass *succeeds*. The ordering exists for the pass that
*fails*.

Concretely, in 0015: deleting a pruned member's stamp at step 9a instead of step 11a was safe
against clobbering and catastrophic against an ordinary error return. An error out of the write
phase would leave the pruned member's rows **and** stamp gone, every available member's rows cleared
by step 9 but not rewritten, and every available member still carrying a stamp that still matched
its unchanged content. Result: `Dirty` empty, registry already replaced so no drift, and the
staleness trigger destroyed along with the stamp — a permanently clean gate over an overlay holding
no cross-edges at all. The fix was to split the prune into two loops over one prune list: records at
9a, stamp delete at 11a, after all writes succeed.

## The review question that catches it

When a spec or diff justifies an ordering, read the supporting argument and ask which failure modes
it enumerated. If the enumeration is only about writes overwriting each other, the argument is
incomplete regardless of how confident its conclusion sounds — and an incomplete supporting argument
is a **gap**, not a weighed decision being re-litigated. That distinction matters at the merge gate:
a gap is fair to reopen; a decision the spec weighed and rejected is not.

Then run the enumeration yourself, once per early exit: for each error return between the marker
write and the end of the pass, name what the next pass sees. If any of them sees a clean gate, the
marker is in the wrong place.

Corollary: **the same ordering rule usually has two scales.** 0013 established records-first /
stamp-last *per member*; 0015 needed it *per pass*. Establishing a rule at one scale does not
establish it at the other, and the code will look locally correct at both.

## Provenance

- **#0015, PR #13** — `wsresolve` stamp pruning. Spec assumption 4 placed the whole prune (records
  and stamp) at step 9a and called the placement a readability choice. Review found the supporting
  argument analysed only clobbering; the build split it 9a (records) / 11a (stamps), and the owner
  ratified records-first/stamp-last at pass scale as the binding semantic. No ADR — the spec had
  already decided this ordering belongs in `Resolve`'s doc comment next to the code it constrains,
  and extending it to pass scale lands in the same place.
