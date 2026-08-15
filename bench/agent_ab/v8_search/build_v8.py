#!/usr/bin/env python3
"""v8: concept-class agent A/B (does `codeindex search` make real agents
better/cheaper at feature-location than native tools?). Tasks sampled SEEDED
from the frozen curated-x2 fixtures; thresholds pre-registered in the header
BEFORE any run."""
import json, random
from pathlib import Path

HERE = Path(__file__).resolve().parent
BENCH = HERE.parent.parent
SEED, PER_REPO = 4242, 12
REPOS = {"gin": "bench/repos/gin", "laravel-framework": "bench/repos/laravel-framework"}

PROMPT = (
    "In the repository at {REPO_PATH}, locate where the following capability is "
    "implemented: \"%s\". Answer with a 'SYMBOLS:' section listing the 1-3 "
    "primary function/method/class names that implement it, and a 'LOCATIONS:' "
    "section with file:line for each, nothing else. Base every claim on "
    "evidence from the repository — do not answer from memory."
)

tasks = []
for repo in REPOS:
    fix = json.loads((BENCH / "concept_sets/x2" / f"{repo}.json").read_text())
    rng = random.Random(SEED)
    qs = sorted(fix["questions"], key=lambda q: q["q"])
    rng.shuffle(qs)
    for q in qs[:PER_REPO]:
        tasks.append({
            "id": f"concept-{repo[:3]}-{abs(hash(q['q'])) % 99999}",
            "type": "concept", "repo": repo,
            "prompt": PROMPT % q["q"],
            "ground_truth": {"accept": q["accept"], "question": q["q"]},
        })

header = {
    "experiment": "v8-search-concept",
    "generated_seed": SEED,
    "repo_pins": {r: {"path": p} for r, p in REPOS.items()},
    "model": "claude-sonnet-4-6",
    "thresholds": {
        "metric": "success (any accept symbol named in SYMBOLS section, word-boundary) + median paired total_cost_usd delta",
        "green": "success_delta >= +15pp AND median paired cost delta <= +10% AND arm-B adoption >= 70%",
        "yellow": "success_delta >= +15pp with cost delta <= +30%; OR success_delta in [0,+15) with savings >= 20%; adoption >= 40%",
        "red": "otherwise (any success regression, or adoption < 40%)",
    },
    "n_tasks": len(tasks), "reps": 2,
    "registered_before_any_run": True,
    "known_bias_disclosed": ("tasks come from the same frozen fixtures the retrieval pipeline was "
        "tuned on (curated-x2). This inflates arm-B RETRIEVAL quality vs unseen questions; the "
        "experiment measures AGENT-level utility (cost/success), which retrieval tuning does not "
        "automatically confer. A held-out-question replication is the follow-up if GREEN."),
}
out = {"header": header, "tasks": tasks}
(HERE / "tasks.json").write_text(json.dumps(out, indent=1))
print(f"wrote {len(tasks)} tasks")
