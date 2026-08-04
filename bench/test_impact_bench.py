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


def test_parse_callers_output_extracts_edges_and_ambiguous():
    raw = (
        "def  pkg.Foo  a.go:10  func Foo()\n"
        "callers (3):\n"
        "  b.go:20  B.callSite\n"
        "  c.go:30  C.other  [ambiguous]\n"
        "  d.go:40  topLevelFn\n"
        "referenced in 3 file(s): b.go c.go d.go\n"
    )
    edges, ambiguous = impact_bench.parse_callers_output(raw)
    assert edges == {("b.go", "B.callSite"), ("c.go", "C.other"), ("d.go", "topLevelFn")}
    assert ambiguous == {("c.go", "C.other")}


def test_parse_callers_output_empty_callers():
    raw = "def  pkg.Foo  a.go:10  func Foo()\ncallers (0):\nreferenced in 0 file(s):\n"
    edges, ambiguous = impact_bench.parse_callers_output(raw)
    assert edges == set()
    assert ambiguous == set()


def test_parse_callers_output_strips_repo_root():
    raw = "callers (1):\n  /tmp/repo/b.go:20  B.callSite\n"
    edges, _ = impact_bench.parse_callers_output(raw, repo_root="/tmp/repo")
    assert edges == {("b.go", "B.callSite")}


from pathlib import Path as _Path
_FIX = _Path(__file__).parent / "impact_fixtures"


def test_fixture_oracle_reads_python_manifest():
    oracle = impact_bench.FixtureOracle(str(_FIX), langs=["py"])
    syms = oracle.symbols("py")
    by_name = {s["symbol"]: s for s in syms}
    assert by_name["shared_helper"]["truth"] == {("app.py", "run"), ("worker.py", "process")}
    assert by_name["shared_helper"]["ambiguous"] is False
    assert by_name["collide"]["ambiguous"] is True
    assert by_name["collide"]["truth"] == {("mod_a.py", "use_a"), ("mod_b.py", "use_b")}
    assert oracle.repo_dir("py").endswith("impact_fixtures/py")


def test_fixture_oracle_repo_dir_returns_lang_subdir():
    oracle = impact_bench.FixtureOracle(str(_FIX))
    for lang in ("py", "js", "php", "go", "ts"):
        assert oracle.repo_dir(lang).endswith(f"impact_fixtures/{lang}")


def test_fixture_oracle_all_langs_have_two_symbols():
    oracle = impact_bench.FixtureOracle(str(_FIX))
    for lang in ("py", "js", "php", "go", "ts"):
        syms = oracle.symbols(lang)
        assert len(syms) == 2, f"{lang}: expected 2 symbols, got {len(syms)}"


def test_fixture_oracle_each_lang_has_one_clean_one_ambiguous():
    oracle = impact_bench.FixtureOracle(str(_FIX))
    for lang in ("py", "js", "php", "go", "ts"):
        syms = oracle.symbols(lang)
        ambiguous = [s for s in syms if s["ambiguous"]]
        clean = [s for s in syms if not s["ambiguous"]]
        assert len(ambiguous) == 1, f"{lang}: expected 1 ambiguous symbol"
        assert len(clean) == 1, f"{lang}: expected 1 clean symbol"
        # clean symbol must have exactly 2 dependents
        assert len(clean[0]["truth"]) == 2, f"{lang}: clean symbol should have 2 dependents"


def test_fixture_oracle_missing_lang_returns_empty():
    oracle = impact_bench.FixtureOracle(str(_FIX))
    assert oracle.symbols("rust") == []


def test_parse_go_diagnostics_extracts_sites():
    stderr = (
        "# impactfix\n"
        "./app.go:6:9: undefined: SharedHelper\n"
        "./worker.go:5:9: undefined: SharedHelper\n"
    )
    sites = impact_bench.parse_go_diagnostics(stderr, "/tmp/repo")
    assert ("app.go", 6) in sites
    assert ("worker.go", 5) in sites


def test_parse_tsc_diagnostics_extracts_sites():
    stdout = (
        "app.ts(4,10): error TS2304: Cannot find name 'sharedHelper'.\n"
        "worker.ts(3,10): error TS2304: Cannot find name 'sharedHelper'.\n"
    )
    sites = impact_bench.parse_tsc_diagnostics(stdout, "/tmp/repo")
    assert ("app.ts", 4) in sites
    assert ("worker.ts", 3) in sites


def test_map_site_to_enclosing_go(tmp_path):
    f = tmp_path / "app.go"
    f.write_text("package main\n\nfunc Run() int {\n    return SharedHelper()\n}\n")
    name = impact_bench.map_site_to_enclosing(str(f), 4, str(tmp_path))
    assert name == "Run"


if __name__ == "__main__":
    import sys
    import tempfile
    fns = [v for k, v in sorted(globals().items())
           if k.startswith("test_") and callable(v)]
    passed = 0
    for fn in fns:
        import inspect
        params = list(inspect.signature(fn).parameters)
        if params:
            # provide tmp_path manually
            if "tmp_path" in params:
                try:
                    fn(_Path(tempfile.mkdtemp()))
                    print(f"ok {fn.__name__}")
                    passed += 1
                except Exception as e:
                    print(f"FAIL {fn.__name__}: {e}")
            else:
                print(f"skip {fn.__name__} (unknown params)")
        else:
            try:
                fn()
                print(f"ok {fn.__name__}")
                passed += 1
            except Exception as e:
                print(f"FAIL {fn.__name__}: {e}")
    print(f"\n{passed} passed")
