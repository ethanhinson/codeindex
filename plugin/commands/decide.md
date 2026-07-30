---
description: Record a decision in .lore/ with rationale and rejected alternatives
argument-hint: <decision title>
allowed-tools: Bash(codeindex *)
---

## Record decision: `$1`

Gather the rationale and rejected alternatives from the current conversation, then run:

```
codeindex lore "$CLAUDE_PROJECT_DIR" add decision --title "$1" --body - <<'EOF'
<rationale from conversation>

## Alternatives considered

<rejected alternatives with reasons>
EOF
```

If the decision relates to a specific symbol or file, anchor it:

- For a symbol: add `--anchor symbol:SymbolName`
- For a file: add `--anchor path:relative/path/to/file.go`

After recording, show the created record (the command prints its output).
