# Sharing the index: build once in CI, import everywhere

The codeindex database is a portable derived artifact: repo-relative paths,
self-describing schema version, per-file content hashes. That means CI can
pay the cold build once per commit and everyone else patches.

Measured on kubernetes (11,005 files, 3M lines): cold build **82.5s**;
importing a CI artifact into a fresh checkout with one locally edited file —
every mtime invalid, so every file is hash-verified — **1.5s**.

## Developer side

```sh
codeindex import <repo-root> path/to/graph.db
# imported graph.db; drift patched: 1 files re-parsed, 0 deleted, 66 symbols
```

Import verifies the artifact's schema version against your binary (a
mismatch fails loudly — re-export with a matching codeindex or just run
`codeindex build`), installs it as `.codeindex/graph.db`, and incrementally
patches whatever differs in your working tree. The drift report shows
exactly what the artifact did not cover. Every query path afterward keeps
the index fresh automatically, as always.

## CI side (GitHub Actions)

```yaml
name: codeindex
on:
  push:
    branches: [main]
jobs:
  index:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Build codeindex
        run: go install github.com/ethanhinson/codeindex/cmd/codeindex@latest
      - name: Build and export index
        run: codeindex export . graph.db
      - uses: actions/upload-artifact@v4
        with:
          name: codeindex-${{ github.sha }}
          path: graph.db
```

Fetch the artifact for your commit (e.g. `gh run download`) and
`codeindex import . graph.db`. If your checkout is a few commits ahead or
behind the artifact, import still works — the patch just covers more files.

## Guarantees

- **Import-then-patch == full rebuild.** Proven by the same equivalence gate
  that guards the incremental engine: an artifact exported at tree state A
  and imported at state B (files edited, added, deleted in between) is
  snapshot-identical to building from scratch at B.
- **mtime-only drift is free.** A fresh clone changes every mtime; content
  hashing catches that nothing changed — zero files re-parsed.
- **Stale artifacts can't hurt you.** Wrong schema version → rejected before
  install, both versions named.
