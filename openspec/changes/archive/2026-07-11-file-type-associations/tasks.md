## 1. Engine

- [x] 1.1 internal/config: Load(root) for .codeindex.json (missing→empty, malformed→error); adapter.Name() on all four adapters; broadened default extensions + tsjs grammar routing (.mts/.cts ts, .mjs/.cjs js, unknown→ts)
- [x] 1.2 adapter.SetAssociations (validated, sorted rules, precedence over extensions) + Indexable(rel); merkle.Walk uses Indexable; engine Build/Patch (and depmap generation) load config at entry
- [x] 1.3 Tests: Drupal-shaped routing + cross-file resolution; inc==full under association add AND remove; unknown-language loud failure; new-default parse tests

## 1b. Content detection

- [x] 1.4 adapter.SniffLang (php tags/shebangs/binary guard) + SetExactRoutes/ForName; merkle WalkWith/DetectWith hook; engine sniffer with persistent sniffcache (negatives cached, per-run route reset); depmap overlay sniff fallback
- [x] 1.5 Tests: sniff unit table; zero-config Drupal; sniff-transition equivalence; association-overrides-sniff

## 2. Surfaces and gates

- [x] 2.1 editors/vscode: association loading/matching in core.ts (+ node tests), config watch + save filter in extension.ts; tsc + tests green
- [x] 2.2 Six-repo gate re-run (no configs → byte-identical results); suite green

## 3. Close-out

- [x] 3.1 README note (.codeindex.json); FINDINGS entry; openspec validate; commit + push; archive
