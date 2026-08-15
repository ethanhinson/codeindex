---
description: Explore a feature/concept end-to-end — semantic search for entry points, then blast radius on the best one.
argument-hint: <feature or concept phrase>
allowed-tools: Bash(codeindex *)
---

## Semantic search for `$1`

!`codeindex search "$CLAUDE_PROJECT_DIR" "$1" --limit 20`

## Task

Explore the `$1` feature from the feature map above:

1. If the first line says `[lexical-only: ...]`, tell the user semantic
   matching is off and why (usually: run `codeindex build` once to embed).
2. Pick the most relevant cluster. Its **entry** (highest callers) is where
   the feature starts; re-run the search with `--hints "<3-6 identifier-style
   token guesses>"` if the clusters look off-topic.
3. Map the winner's blast radius:
   `codeindex impact "$CLAUDE_PROJECT_DIR" "<EntryPoint>" --limit 30`
4. Answer with: the feature's entry point(s), the call chain that implements
   it (path:line references), and which clusters are related vs. incidental.
5. Trust the output — it is complete and fresh; do not re-read files to
   verify, except entries flagged `[ambiguous]`.
