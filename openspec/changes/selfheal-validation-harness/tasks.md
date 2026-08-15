# Tasks: selfheal-validation-harness

## 1. Parallel construction (subagents)

- [x] 1.1 PHP Excimer container lab: Dockerfile (php:8.3 + pecl excimer), generated hook-framework app, excimer→cxprof adapter emitting repo-relative frames, run.sh + verify.sh proving the full host-side loop
- [x] 1.2 Issue-corpus miner: deepen pinned clones, mine fix commits, map diffs→symbols by span overlap, fetch titles (budgeted/cached), emit curated-format fixtures, score closed-issue class for gin + flask
- [x] 1.3 Self-healing harness: scenario matrix (node-registry, go-sdk, node-symlink, optional php-excimer), assertion suite, remediation ladder, learned.json replay, runs.jsonl journal

## 2. Integration & findings (main session)

- [x] 2.1 Integrate agent deliverables; run the full matrix including php-excimer; fix integration seams only
- [x] 2.2 Record findings in `bench/engine/FINDINGS-selfheal-validation.md`: scenario outcomes, remediations exercised, issue-class scores vs curated scores, and what the corpus says about the residual buckets
- [x] 2.3 File follow-ups discovered by the lab (core bugs → backlog or their own changes; never silently patched here)
