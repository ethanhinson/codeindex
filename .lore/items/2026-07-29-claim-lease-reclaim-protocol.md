---
id: itm-01KYR5Z1KBTSWSS1K07WNZQRHT
title: Claim/lease/reclaim protocol on items (parallel-agent draining)
status: open
date: 2026-07-29
priority: p1
blocked_by: [itm-01KYR17XECP2YV5YBQ7VT87NQF]
refs:
    - url: https://github.com/danielhanold/docket
    - url: .lore/notes/docket-comparison-and-adoptable-ideas.md
---
Adopted from docket. Add execution-state fields to items — `claimed_at`
(UTC ISO-8601 lease stamp) and `branch:` — plus the coordination protocol:
claim = compare-and-swap via git (commit status flip, push; on non-fast-
forward re-read and re-claim; the arbiter is the re-read, not the push),
heartbeat = re-stamp `claimed_at` on phase boundaries, reclaim = flip back
to open when the lease is expired AND no matching branch ref resolves
(crashed-worker self-healing; never reclaim without positive evidence).
Surface eligible reclaims in `lore doctor`; auto-reclaim stays opt-in.

This is what makes "sharding out effort" across parallel/remote agents safe:
two implementers can never hold the same item.
