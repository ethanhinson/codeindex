import importlib.util
from pathlib import Path
import sys

_spec = importlib.util.spec_from_file_location(
    "impact_bench", Path(__file__).parent / "impact_bench.py"
)
impact_bench = importlib.util.module_from_spec(_spec)
sys.modules["impact_bench"] = impact_bench
_spec.loader.exec_module(impact_bench)


def test_score_sets_perfect_recall_and_precision():
    truth = {("a.go", "A.f"), ("b.go", "B.g")}
    pred = {("a.go", "A.f"), ("b.go", "B.g")}
    s = impact_bench.score_sets(truth, pred)
    assert s.tp == 2 and s.fn == 0 and s.fp == 0
    assert s.recall == 1.0 and s.precision == 1.0


def test_score_sets_missed_dependent_lowers_recall():
    truth = {("a.go", "A.f"), ("b.go", "B.g")}
    pred = {("a.go", "A.f")}  # missed B.g
    s = impact_bench.score_sets(truth, pred)
    assert s.tp == 1 and s.fn == 1 and s.fp == 0
    assert s.recall == 0.5 and s.precision == 1.0


def test_score_sets_false_positive_lowers_precision():
    truth = {("a.go", "A.f")}
    pred = {("a.go", "A.f"), ("noise.go", "N.x")}
    s = impact_bench.score_sets(truth, pred)
    assert s.tp == 1 and s.fn == 0 and s.fp == 1
    assert s.recall == 1.0 and s.precision == 0.5


def test_score_sets_empty_truth_is_recall_one():
    # a symbol with zero authored dependents: correct answer is empty impact set
    s = impact_bench.score_sets(set(), set())
    assert s.recall == 1.0 and s.precision == 1.0
    s2 = impact_bench.score_sets(set(), {("x.go", "X.y")})
    assert s2.recall == 1.0 and s2.precision == 0.0  # spurious dependents still hurt precision


def test_score_with_ambiguous_breaks_out_flagged_subset():
    truth = {("a.go", "A.f"), ("b.go", "B.g"), ("c.go", "C.h")}
    pred = {("a.go", "A.f"), ("b.go", "B.g"), ("c.go", "C.h")}
    ambiguous = {("b.go", "B.g"), ("c.go", "C.h")}  # two flagged [ambiguous]
    overall, amb = impact_bench.score_with_ambiguous(truth, pred, ambiguous)
    assert overall.recall == 1.0 and overall.precision == 1.0
    # ambiguous subset: 2 flagged predictions, both correct
    assert amb.tp == 2 and amb.fn == 0 and amb.fp == 0
    assert amb.recall == 1.0 and amb.precision == 1.0


if __name__ == "__main__":
    import sys
    fns = [v for k, v in sorted(globals().items())
           if k.startswith("test_") and callable(v)]
    for fn in fns:
        fn()
        print(f"ok {fn.__name__}")
    print(f"\n{len(fns)} passed")
