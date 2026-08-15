#!/usr/bin/env python3
"""Embedding + linear-classifier routing baseline (Gate 3).

The honest minimal baseline for 3-way routing: embed the task with the LOCAL
embedding model, train a logistic-regression head. Trains in seconds, runs
anywhere, no LLM at inference. If it clears the 58% rule and approaches the 93%
ceiling, a distilled LLM is unnecessary for routing.

Uses calibrated, intent-preserving naturalistic phrasings (multiple per symbol
for augmentation) + accept-set scoring (callers~grep interchangeable).
All embeddings computed locally via LM Studio (:1234).
"""
from __future__ import annotations
import json, sys, random
import numpy as np
from sklearn.linear_model import LogisticRegression
from sklearn.model_selection import train_test_split

sys.path.insert(0, ".")
from measure_ceiling import PARA, make_task, accept_set  # reuse calibrated templates

# Local embedding model (cached in HF hub; runs on-device via sentence-transformers/MPS).
EMBED_MODEL = "BAAI/bge-base-en-v1.5"
_ST = None

def embed(texts, batch=64):
    global _ST
    if _ST is None:
        from sentence_transformers import SentenceTransformer
        import torch
        dev = "mps" if torch.backends.mps.is_available() else "cpu"
        _ST = SentenceTransformer(EMBED_MODEL, device=dev)
    return _ST.encode(texts, batch_size=batch, normalize_embeddings=True,
                      show_progress_bar=False).astype(np.float32)

def build_split_by_template(rows, seed, holdout_templates=2):
    """Split by TEMPLATE, not phrasing: test phrasings use templates the
    classifier NEVER saw in training. This measures generalization to novel
    phrasing — the real deployment condition — not template memorization."""
    rng = random.Random(seed)
    # per type, hold out the last `holdout_templates` template indices for test
    n_tmpl = {t: len(PARA[t]) for t in PARA}
    test_idx = {t: set(range(n_tmpl[t] - holdout_templates, n_tmpl[t])) for t in PARA}
    train_idx = {t: set(range(n_tmpl[t])) - test_idx[t] for t in PARA}

    Xtr, ytr, Xte, yte = [], [], [], []
    for r in rows:
        gold = r["gold_trajectory"][0]["tool"]
        for i in train_idx[r["type"]]:
            Xtr.append(make_task(r, i)); ytr.append(gold)
        for i in test_idx[r["type"]]:
            Xte.append(make_task(r, i)); yte.append(gold)
    return Xtr, np.array(ytr), Xte, np.array(yte)

def accept_score(y_true, y_pred):
    strict = np.mean(y_true == y_pred)
    soft = np.mean([p in accept_set(t) for t, p in zip(y_true, y_pred)])
    return strict, soft

def main():
    seed = 13
    rows = [json.loads(l) for l in open("gin_tier1.jsonl")]
    # HELD-OUT-TEMPLATE split: test uses phrasings the model never trained on.
    Xtr_t, ytr, Xte_t, yte = build_split_by_template(rows, seed, holdout_templates=2)
    print(f"train {len(Xtr_t)} (seen templates) / test {len(Xte_t)} (HELD-OUT templates)")
    print("embedding (local)...")
    Xtr = embed(Xtr_t); Xte = embed(Xte_t)

    clf = LogisticRegression(max_iter=2000, C=2.0)
    clf.fit(Xtr, ytr)
    ypred = clf.predict(Xte)
    strict, soft = accept_score(yte, ypred)

    print(f"\n=== embedding + logreg routing ===")
    print(f"  train {len(ytr)} / test {len(yte)}  | embed dim {Xtr.shape[1]}")
    print(f"  STRICT accuracy:     {strict*100:.0f}%")
    print(f"  ACCEPT-SET accuracy: {soft*100:.0f}%   <- compare to rule 58%, 7B ceiling 93%")
    # per-class
    print("  per gold tool (accept-set):")
    for t in sorted(set(yte)):
        m = yte == t
        _, s = accept_score(yte[m], ypred[m])
        print(f"    {t:<18} {s*100:3.0f}%  (n={int(m.sum())})")
    print(f"\nGATE: if accept-set >= ~85%, a linear classifier suffices — no "
          f"distilled LLM needed for routing.")

if __name__ == "__main__":
    main()
