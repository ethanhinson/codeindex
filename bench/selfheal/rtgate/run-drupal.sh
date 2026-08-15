#!/usr/bin/env bash
# Host side: profile real Drupal code paths in the rtgate container and land
# cxprof spool files in bench/selfheal/rtgate/spool/drupal/.
# Composer vendor deps are cached in the docker volume rtgate-drupal-deps.
set -euo pipefail
cd "$(dirname "$0")"

REPO="$(cd ../../repos/drupal && pwd)"
COMMIT="$(git -C "$REPO" rev-parse HEAD 2>/dev/null || true)"

docker build -q -t codeindex-rtgate . >/dev/null

mkdir -p spool/drupal
rm -f spool/drupal/*.cxprof.jsonl

docker run --rm \
  -v "$REPO:/site/core:ro" \
  -v "$PWD:/rtgate:ro" \
  -v "$PWD/spool/drupal:/spool" \
  -v rtgate-drupal-deps:/deps \
  -e RTGATE_COMMIT="$COMMIT" \
  -e RTGATE_PERIOD="${RTGATE_PERIOD:-0.003}" \
  codeindex-rtgate sh /rtgate/drupal-container.sh

ls -la spool/drupal/
