#!/bin/bash
# Embed-pool timing sweep (kubernetes-scale embed budget; registered bar:
# <=2 min added at 182k symbols).
#
# ENVIRONMENT REQUIREMENT — the reason this is a script and not history:
# run ONLY on an awake, plugged-in, display-on machine. Overnight/lidded
# macOS throttles background work onto E-cores and dozes between phases;
# it inflated a 5-minute laravel embed to 13m48s and a 19s gin build to
# 439s real. caffeinate -dimsu is used but does NOT fully compensate.
# Sanity gate: the first run below (gin 1x8) must land within ~25% of the
# recorded awake baseline (17.8s internal) or every following number is
# garbage — abort and rerun when the machine is genuinely active.
#
# Usage: bash bench/embed_pool_sweep.sh /path/to/codeindex-binary
set -euo pipefail
BIN=${1:?usage: embed_pool_sweep.sh <codeindex-binary>}
cd "$(dirname "$0")/repos"

run() { # ctxs threads repo
  rm -rf "$3/.codeindex"
  echo "=== $3 ctx=$1 threads=$2 ==="
  CODEINDEX_EMBED_CTX="$1" CODEINDEX_EMBED_THREADS="$2" \
    caffeinate -dimsu /usr/bin/time -p "$BIN" build "$3" 2>&1 | tail -4
}

echo "== phase 1: gin shape sweep (each ~20-60s; 1x8 first = sanity gate) =="
run 1 8 gin
run 2 8 gin
run 2 4 gin
run 4 4 gin
run 4 2 gin
run 6 2 gin
run 8 1 gin

echo "== phase 2: laravel-framework (28k symbols) — clean 1x8 re-baseline + best-2 from phase 1 =="
echo "== EDIT the two configs below to phase 1's winners before this phase =="
run 1 8 laravel-framework
run 4 4 laravel-framework
run 2 8 laravel-framework

echo "== done: extrapolate ms/card x 182k for the kubernetes bar; record in FINDINGS =="
