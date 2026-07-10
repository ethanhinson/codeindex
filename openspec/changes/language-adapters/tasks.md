## 1. Registry-driven walk (unblocks everything)

- [x] 1.1 Add `adapter.Extensions()` (sorted set of registered extensions); switch `merkle.Walk` and `engine.CountLines` to it; keep the skip-list (`vendor`, `node_modules`, `testdata`, `.git`, `.codeindex`)
- [x] 1.2 Test: polyglot fixture tree walks exactly the registered extensions; unsupported files skipped

## 2. TypeScript/JavaScript adapter (`internal/adapter/tsjs`)

- [x] 2.1 Grammar selection by extension (.ts→typescript, .tsx→tsx, .js/.jsx→javascript); parse + walk
- [x] 2.2 Symbols: function declarations (func), class declarations (type), method definitions (method), named top-level/exported arrow-function bindings (func); anonymous callbacks excluded
- [x] 2.3 Call sites: `call_expression` → final name (identifier or member-expression property), attributed to innermost enclosing symbol
- [x] 2.4 Fixture tests asserting exact symbols (name/kind/span) and edges

## 3. Python adapter (`internal/adapter/python`)

- [x] 3.1 Symbols: `function_definition` (method when inside `class_definition`), `class_definition` (type); lambdas excluded
- [x] 3.2 Call sites: `call` → final name (identifier or attribute), enclosing attribution
- [x] 3.3 Fixture tests

## 4. PHP adapter (`internal/adapter/php`)

- [x] 4.1 Symbols: `function_definition` (func), `method_declaration` (method), class/interface/trait declarations (type)
- [x] 4.2 Call sites: function/member/scoped call expressions → final name, enclosing attribution
- [x] 4.3 Fixture tests

## 5. Real-repo validation

- [x] 5.1 Pin Python and PHP reference repos in `bench/repos.json` (e.g. pallets/flask, laravel/framework at fixed tags); clone via the existing recipe
- [x] 5.2 Run `codeindex bench` on nest (TS), the Python repo, and the PHP repo: incremental==full MUST pass per language; record cold-build/patch numbers in `bench/engine/`
- [x] 5.3 Spot-check query quality per language (`callers` on a known symbol in each repo) and record one example per language in the findings note

## 6. Consumption surfaces

- [x] 6.1 `plugin/hooks/post_edit.py`: extension set `.go .ts .tsx .js .jsx .py .php`; fixture-payload test for a `.py` edit
- [x] 6.2 `plugin/hooks/prompt_context.py`: gate on any supported-language file/manifest (go.mod, package.json, pyproject/setup, composer.json); update wording; keep "measured on Go repos" qualifier
- [x] 6.3 MCP tool descriptions + plugin/README + root README: supported languages stated exactly; Go-derived numbers labeled

## 7. Verification

- [x] 7.1 `go test ./...` green; `gofmt` clean; rebuild `bench/agent_ab/.bin/codeindex`
- [x] 7.2 Mark `core-indexing-engine` 4.3 satisfied (TS/JS adapter) with a pointer to this change; `openspec validate language-adapters` passes
- [x] 7.3 Findings note (`bench/engine/FINDINGS-languages.md`): per-language bench numbers, incremental proof results, known extraction limits (anonymous functions, dynamic constructs)
