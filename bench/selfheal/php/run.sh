#!/usr/bin/env bash
# Build the PHP+Excimer lab image, profile the hook app inside a container,
# and land the resulting cxprof spool file in app/.codeindex/runtime/.
set -euo pipefail
cd "$(dirname "$0")"

docker build -t codeindex-php-lab .

mkdir -p spool app/.codeindex/runtime
rm -f spool/*.cxprof.jsonl app/.codeindex/runtime/*.cxprof.jsonl

docker run --rm \
  -v "$PWD/app:/app" \
  -v "$PWD/spool:/spool" \
  -v "$PWD/adapter.php:/adapter.php:ro" \
  codeindex-php-lab php /adapter.php

latest="$(ls -t spool/*-php.cxprof.jsonl 2>/dev/null | head -1 || true)"
if [ -z "$latest" ]; then
  echo "run.sh: FAIL — no spool file produced" >&2
  exit 1
fi

records=$(( $(wc -l < "$latest") - 1 ))  # minus the header line
if [ "$records" -lt 5 ]; then
  echo "run.sh: FAIL — only $records stack records in $latest (need >=5)" >&2
  exit 1
fi

cp "$latest" app/.codeindex/runtime/
echo "run.sh: OK — $latest has $records stack records; copied to app/.codeindex/runtime/"
