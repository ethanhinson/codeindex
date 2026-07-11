## 1. Schema and write paths

- [x] 1.1 v7 schema: strs table, symbols_t/edges_t with int refs + int indexes, symbols/edges views reconstructing TEXT columns
- [x] 1.2 Store intern cache (reset on schema init); rewrite the 9 write statements (PutFile symbols+calls+deps, deleteFileGraph, ReResolveNames UPDATE→edges_t, depmaps PutDepSymbols/AttachMap/OverlayDepFile inserts+deletes)
- [x] 1.3 Full test suite green (engine, depmap, search, query untouched by design)

## 2. Gates and metric

- [x] 2.1 Six-repo bench: inc==full ×6; build/query budgets; apply D4 fallback if planner degrades, re-gate
- [x] 2.2 Size metric vs bench/engine/size-baseline-v6.json — absolute reduction on all six, laravel ≤2× by bench ratio; FINDINGS-interning.md

## 3. Close-out

- [x] 3.1 openspec validate; commit + push; archive
