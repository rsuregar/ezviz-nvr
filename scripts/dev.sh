#!/usr/bin/env bash
# Runs the whole NVR EZVIZ stack locally with one command: API, web dev
# server, and (if the `mediamtx` binary is installed) the live-view relay.
# Everything runs under this script; Ctrl+C stops all of it cleanly.
#
# Prereqs (see README "Jalan di lokal" for full setup): Go 1.25+, Node 22+,
# a reachable MySQL/MariaDB matching MYSQL_DSN in .env, and `mediamtx`
# installed if you want live view.
#
# Env overrides:
#   WITH_MEDIAMTX=0   skip starting mediamtx even if it's installed
#   WITH_AGENT=1      also start the edge agent (only useful if this
#                     machine is on the same LAN as a real camera)

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

if [ ! -f .env ]; then
  echo "No .env found — copying from .env.example. Edit it (DB creds, Google OAuth, etc.) then re-run." >&2
  cp .env.example .env
  exit 1
fi

# Read just the two values this script needs directly off the .env file,
# rather than `source .env` — values like MYSQL_DSN contain `&` (its query
# string), which bash interprets as a background-job operator when sourced
# as a script instead of as plain KEY=VALUE text. Grepping avoids that.
env_value() {
  grep -E "^$1=" .env | tail -1 | cut -d'=' -f2-
}
MYSQL_DSN=$(env_value MYSQL_DSN)
LISTEN_ADDR=$(env_value LISTEN_ADDR)

PIDS=()
cleanup() {
  echo ""
  echo "Stopping..."
  for pid in "${PIDS[@]:-}"; do
    [ -n "$pid" ] && kill "$pid" 2>/dev/null || true
  done
  wait 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# --- MySQL reachability check ---
db_host_port=$(echo "${MYSQL_DSN:-}" | sed -n 's/.*@tcp(\([^)]*\)).*/\1/p')
db_host=${db_host_port%%:*}
db_port=${db_host_port##*:}
if ! nc -z -w2 "${db_host:-127.0.0.1}" "${db_port:-3306}" 2>/dev/null; then
  echo "MySQL/MariaDB tidak terjangkau di ${db_host_port:-127.0.0.1:3306} — jalankan dulu" >&2
  echo "(mis. 'brew services start mariadb' atau 'docker compose up -d mysql')." >&2
  exit 1
fi

# --- MediaMTX (optional) ---
if [ "${WITH_MEDIAMTX:-1}" = "1" ] && command -v mediamtx >/dev/null 2>&1; then
  echo "Starting MediaMTX..."
  api_port="${LISTEN_ADDR#*:}"
  MTX_AUTHHTTPADDRESS="http://localhost:${api_port:-8080}/api/mediamtx/auth" \
    mediamtx infra/mediamtx.yml > /tmp/nvr-mediamtx.log 2>&1 &
  PIDS+=("$!")
else
  echo "Melewati MediaMTX (tidak terpasang, atau WITH_MEDIAMTX=0) — live view tetap offline, recording tetap jalan."
fi

# --- API ---
echo "Starting API..."
(cd apps/api && go run ./cmd/api) > /tmp/nvr-api.log 2>&1 &
PIDS+=("$!")

# --- Edge agent (optional, off by default — only makes sense if a real
#     camera is reachable from this machine's own LAN) ---
if [ "${WITH_AGENT:-0}" = "1" ]; then
  echo "Starting edge agent..."
  (cd apps/edge-agent && go run ./cmd/agent) > /tmp/nvr-agent.log 2>&1 &
  PIDS+=("$!")
fi

if [ ! -d apps/web/node_modules ]; then
  echo "Installing web dependencies (first run)..."
  (cd apps/web && npm install)
fi

# Give the API a moment to come up before the web dev server's first request.
sleep 2

echo ""
echo "Logs: /tmp/nvr-api.log, /tmp/nvr-mediamtx.log, /tmp/nvr-agent.log"
echo "API : http://localhost:${LISTEN_ADDR#*:}"
echo "Web : http://localhost:3000 (starting now, in foreground)"
echo "Ctrl+C untuk menghentikan semuanya."
echo ""

# Web runs in the foreground: Ctrl+C here is what triggers the cleanup trap.
cd apps/web && npm run dev
