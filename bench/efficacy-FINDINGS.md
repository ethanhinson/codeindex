# End-to-end efficacy — real GitHub issue, with vs without codeindex

**Date:** 2026-07-09
**Harness:** `bench/efficacy.py` (uses the real `codeindex` engine for the WITH
side and grep+read for the WITHOUT side)
**Token counter:** Anthropic `count_tokens` (exact for Claude, via `bench/.env`)
**Repo:** prometheus/prometheus (indexed with `codeindex build`)

## What this measures

To work an issue, an agent must first *understand* the code it touches — where
the referenced symbols are defined and who calls them. This measures the tokens
for that comprehension step two ways:

- **WITHOUT (grep + read):** grep each referenced symbol, read the matching files
  into context. `smart` = top-8 files by match count; `naive` = all matching files.
- **WITH (codeindex):** run `codeindex query <symbol>` and read its compact
  `path:line + signature` answer (definitions + callers).

It does **not** measure the whole solve — editing and reasoning tokens are
unaffected by the plugin. This is the navigation/comprehension phase the plugin
targets.

## Result — issue #11505

"remote write: When ingesting, check that labels are indeed sorted
lexicographically." The issue explicitly links `Labels` and
`func (ls Labels) HasDuplicateLabelNames()`.

| Symbol | WITH (codeindex) | WITHOUT smart (top-8 files) | savings | files mentioning |
| ------ | ---------------- | --------------------------- | ------- | ---------------- |
| `HasDuplicateLabelNames` | 382 | 87,402 | **229×** | 8 |
| `Labels` | 5,507 | 308,222 | 56× | 227 |
| **issue total** | **5,889** | **395,624** | **67× (smart) / 230× (naive)** | |

Issue #16525 (`Labels`): WITH 5,507 vs WITHOUT smart 308,222 → **56×**.

## Findings

1. **For a specific referenced symbol the win is huge and clean.**
   `HasDuplicateLabelNames` — exactly the kind of function an issue points at —
   costs **382 tokens** to fully locate+understand via the index versus **87,402**
   to grep and read the 8 files that mention it: **229× fewer**. The index answer
   also *directly* lists the 3 definitions and 11 callers, which grep+read only
   yields after the agent reads and parses the files itself.

2. **Even a common, ambiguous symbol wins by ~50×.** `Labels` is referenced in
   227 files; the compact answer (its defs + capped callers) is 5,507 tokens vs
   300k+ to read even the top 8 files.

3. **Name-based resolution is the current ceiling, and it shows here.** `Labels`
   resolves to 25 same-named definitions (every `Labels()` method across types),
   inflating its WITH answer. Precise resolution (`core-indexing-engine` change 2)
   would disambiguate to the specific `Labels` type and shrink that answer
   further — so 56× is a floor for the common-symbol case, not a ceiling.

## Honesty / caveats

- **Auto-extraction is noisy.** Extracting symbols from prose picks up false
  positives (`requirement` because the text reads "requirement (or assumption)";
  the common word `labels`). The headline uses the symbols the issue *explicitly
  links*. A production plugin skill would extract symbols from the code/agent
  context, not free prose — a real limitation to fix, not paper over.
- **The `smart` baseline reads 8 whole files.** A very disciplined agent reading
  only the relevant ranges would narrow the gap; `smart` (not `naive`) is the
  conservative bound and still shows 56–229×.
- **prometheus files are large**, which favors the index (consistent with the
  earlier finding that savings scale with file size). Small-file codebases would
  show smaller multiples, as `nest` did in the symbol-level spike.
- **Comprehension only.** This is the phase the plugin changes; total solve tokens
  (which include unchanged editing/reasoning) improve by less in relative terms.

## Reproduce

```
codeindex build <prometheus-clone>
python3 bench/efficacy.py --binary <codeindex> --repo <prometheus-clone> \
    --issue prometheus/prometheus#11505 --symbols HasDuplicateLabelNames,Labels \
    --out bench/results/efficacy-11505.json
```
