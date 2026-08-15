#!/usr/bin/env bash
# Host side: profile real WordPress code paths in the rtgate container and
# land cxprof spool files in bench/selfheal/rtgate/spool/wordpress/.
set -euo pipefail
cd "$(dirname "$0")"

REPO="$(cd ../../repos/wordpress && pwd)"
COMMIT="$(git -C "$REPO" rev-parse HEAD 2>/dev/null || true)"

docker build -q -t codeindex-rtgate . >/dev/null

mkdir -p spool/wordpress
rm -f spool/wordpress/*.cxprof.jsonl

docker run --rm \
  -v "$REPO:/repo:ro" \
  -v "$PWD:/rtgate:ro" \
  -v "$PWD/spool/wordpress:/spool" \
  -e RTGATE_COMMIT="$COMMIT" \
  -e RTGATE_PERIOD="${RTGATE_PERIOD:-0.003}" \
  codeindex-rtgate sh /rtgate/wp-container.sh

ls -la spool/wordpress/
