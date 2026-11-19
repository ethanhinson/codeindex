#!/usr/bin/env bash
# Expose a local port through an ngrok tunnel.
#
# Usage:
#   scripts/ngrok-tunnel.sh [PORT]            # default port 7676 (codeindex serve)
#   scripts/ngrok-tunnel.sh --serve [REPO]    # also start `codeindex serve` on 7676
#
# Environment:
#   NGROK_AUTHTOKEN   ngrok token; only needed once if not already configured
#   NGROK_BASIC_AUTH  "user:password" to put the tunnel behind basic auth (recommended)
#   NGROK_DOMAIN      reserved/static ngrok domain, e.g. myname.ngrok-free.app
set -euo pipefail

PORT=7676
SERVE=0
REPO="."

while [[ $# -gt 0 ]]; do
  case "$1" in
    --serve) SERVE=1; shift; if [[ $# -gt 0 && "$1" != -* ]]; then REPO="$1"; shift; fi ;;
    -h|--help) sed -n '2,10p' "$0"; exit 0 ;;
    *) PORT="$1"; shift ;;
  esac
done

if ! command -v ngrok >/dev/null 2>&1; then
  echo "ngrok not found. Install it first:" >&2
  echo "  macOS:  brew install ngrok/ngrok/ngrok" >&2
  echo "  Linux:  https://ngrok.com/download (or: snap install ngrok)" >&2
  exit 1
fi

if [[ -n "${NGROK_AUTHTOKEN:-}" ]]; then
  ngrok config add-authtoken "$NGROK_AUTHTOKEN" >/dev/null
fi

SERVE_PID=""
cleanup() { [[ -n "$SERVE_PID" ]] && kill "$SERVE_PID" 2>/dev/null || true; }
trap cleanup EXIT INT TERM

if [[ "$SERVE" == 1 ]]; then
  command -v codeindex >/dev/null 2>&1 || { echo "codeindex not on PATH" >&2; exit 1; }
  (cd "$REPO" && codeindex serve --addr "127.0.0.1:$PORT") &
  SERVE_PID=$!
  # Wait up to ~10s for the server to come up.
  for _ in $(seq 1 20); do
    curl -fs "http://127.0.0.1:$PORT/" >/dev/null 2>&1 && break
    sleep 0.5
  done
fi

if [[ -z "${NGROK_BASIC_AUTH:-}" ]]; then
  echo "WARNING: tunnel is public with no auth. Set NGROK_BASIC_AUTH=user:pass to protect it." >&2
fi

args=(http "127.0.0.1:$PORT")
[[ -n "${NGROK_BASIC_AUTH:-}" ]] && args+=(--basic-auth "$NGROK_BASIC_AUTH")
[[ -n "${NGROK_DOMAIN:-}" ]] && args+=(--url "$NGROK_DOMAIN")

echo "Tunneling http://127.0.0.1:$PORT  (inspector: http://127.0.0.1:4040)"
ngrok "${args[@]}"
