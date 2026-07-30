---
id: itm-01KYTCQDP4DBJR8TE8GKBP4HRD
title: Bound first-scan survival credit to the record's lifetime
status: open
date: 2026-07-30
priority: p2
tags: [signals]
discovered_from: [itm-01KYR17XECP2YV5YBQ7VT87NQF]
anchors:
    - path: internal/lore/index/reindex.go
---
Found by the Plan 3 final review. The survival pass has no date bound: on a
first scan (empty last_scanned_commit → newest 500 commits) a record created
yesterday collects survived credit for months of history predating it — an
anchored record in an active directory can reach confidence 1.0 on day one,
undermining "evidence accumulates." Also: survival credit currently accrues
to superseded records; the spec says credit only while the decision stands.

Fix direction: skip commits older than the record's date (commit timestamps
are available via an extra %ct format field), or initialize
last_scanned_commit to HEAD on first contact; add a status != superseded
guard on the credit path.
