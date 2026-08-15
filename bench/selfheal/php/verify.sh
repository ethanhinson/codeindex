#!/usr/bin/env bash
# Host-side verification: index the app, ingest the runtime profile produced
# by run.sh, and confirm search results carry observed (runtime) evidence.
set -euo pipefail
cd "$(dirname "$0")"

REPO_ROOT="$(cd ../../.. && pwd)"
BIN="${CODEINDEX_BIN:-/tmp/codeindex-selfheal}"

if [ ! -x "$BIN" ]; then
  echo "verify.sh: building codeindex binary at $BIN"
  if [ ! -d "$REPO_ROOT/third_party/llama.cpp/build" ]; then
    (cd "$REPO_ROOT" && ./scripts/vendor-llama.sh)
  fi
  (cd "$REPO_ROOT" && go build -o "$BIN" ./cmd/codeindex)
fi

"$BIN" build app

ingest_out="$("$BIN" ingest app 2>&1)"
echo "$ingest_out"
if ! echo "$ingest_out" | grep -Eq -- '-> [1-9][0-9]* observed edges'; then
  echo "verify.sh: FAIL — ingest reported no observed edges" >&2
  exit 1
fi

search_out="$("$BIN" search app "which handler runs when the invoice settled event is dispatched" --limit 5 2>&1)"
echo "$search_out"
if ! echo "$search_out" | grep -q "observed"; then
  echo "verify.sh: FAIL — search output has no observed marker" >&2
  exit 1
fi

echo "verify.sh: OK"
