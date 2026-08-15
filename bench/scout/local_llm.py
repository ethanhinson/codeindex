#!/usr/bin/env python3
"""Thin client for a LOCAL LLM via LM Studio's OpenAI-compatible API.

Everything Scout needs a model for — paraphrasing tasks into naturalistic
phrasings, semantic label judging — runs here, on-device, no external calls.
Defaults to whatever LM Studio has loaded on :1234.
"""
from __future__ import annotations
import json, urllib.request

BASE = "http://localhost:1234/v1"

def _models():
    with urllib.request.urlopen(f"{BASE}/models", timeout=5) as r:
        return [m["id"] for m in json.load(r)["data"]]

def default_model(prefer=("qwen2.5-coder", "qwen2.5-7b", "qwen")):
    ms = _models()
    for p in prefer:
        for m in ms:
            if p in m and "embed" not in m:
                return m
    return next(m for m in ms if "embed" not in m)

def chat(prompt, model=None, temperature=0.7, max_tokens=64, system=None, json_mode=False):
    body = {
        "model": model or default_model(),
        "messages": ([{"role": "system", "content": system}] if system else [])
                    + [{"role": "user", "content": prompt}],
        "temperature": temperature, "max_tokens": max_tokens,
    }
    if json_mode:
        body["response_format"] = {"type": "json_object"}
    req = urllib.request.Request(f"{BASE}/chat/completions",
                                 data=json.dumps(body).encode(),
                                 headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=120) as r:
        return json.load(r)["choices"][0]["message"]["content"].strip()

def available():
    try:
        _models(); return True
    except Exception:
        return False

if __name__ == "__main__":
    print("local models:", _models())
    print("default:", default_model())
    print("test:", chat("Say 'ok' and nothing else.", max_tokens=5))
