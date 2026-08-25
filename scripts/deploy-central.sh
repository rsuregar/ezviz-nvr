#!/usr/bin/env bash
# Builds and (re)installs apps/api and apps/web as systemd services on this
# VPS. Run this ON the VPS itself, as a user that can sudo (systemctl and
# writing to /etc/systemd/system need root).
#
# This does NOT touch the edge agent — that always runs at each site's own
# location, on its own machine, never on this central VPS (see README
# "Site vs Workspace"). See scripts/deploy-edge-agent.sh for that, run
# separately on each site's own box.
#
# Usage:
#   INSTALL_DIR=/opt/nvr RUN_USER=$(whoami) ./scripts/deploy-central.sh
#
# Env overrides:
#   INSTALL_DIR=/opt/nvr     where binaries + the production .env live
#   RUN_USER=$(whoami)       system user the services run as
#   SKIP_BUILD=1             reuse whatever's already in INSTALL_DIR
#                            (skips `go build`/`npm run build`)

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

install_dir="${INSTALL_DIR:-/opt/nvr}"
run_user="${RUN_USER:-$(whoami)}"

if [ "$(id -u)" -ne 0 ] && ! sudo -n true 2>/dev/null; then
  echo "Script ini menulis systemd unit dan butuh sudo — jalankan sebagai root, atau pastikan sudo tanpa password tersedia." >&2
fi

sudo mkdir -p "$install_dir"
sudo chown "$run_user" "$install_dir"

if [ ! -f "$install_dir/.env" ]; then
  echo "Belum ada $install_dir/.env — menyalin dari .env.example." >&2
  echo "EDIT DULU dengan nilai produksi (JWT_SECRET, STORAGE_ENCRYPTION_KEY, MYSQL_DSN, domain di" >&2
  echo "GOOGLE_OAUTH_REDIRECT_URL/WEB_BASE_URL) sebelum menjalankan script ini lagi." >&2
  cp .env.example "$install_dir/.env"
  exit 1
fi

if [ "${SKIP_BUILD:-0}" != "1" ]; then
  echo "Building api..."
  (cd apps/api && CGO_ENABLED=0 go build -o "$install_dir/api" ./cmd/api)

  echo "Building web..."
  (cd apps/web && npm ci && npm run build)
  rm -rf "$install_dir/web"
  mkdir -p "$install_dir/web"
  cp -r apps/web/.output/. "$install_dir/web/"
fi

render_unit() {
  # $1 = template path, $2 = output path
  sed -e "s#{{INSTALL_DIR}}#$install_dir#g" -e "s#{{RUN_USER}}#$run_user#g" "$1" | sudo tee "$2" > /dev/null
}

render_unit scripts/systemd/nvr-api.service.template /etc/systemd/system/nvr-api.service
render_unit scripts/systemd/nvr-web.service.template /etc/systemd/system/nvr-web.service

sudo systemctl daemon-reload
sudo systemctl enable nvr-api nvr-web
sudo systemctl restart nvr-api nvr-web

echo ""
echo "Selesai. Status:"
sudo systemctl --no-pager --lines=0 status nvr-api nvr-web || true
echo ""
echo "Ingat: reverse-proxy domain API dan domain dashboard ke 127.0.0.1:8080 dan 127.0.0.1:3000"
echo "(lihat README bagian 'Deploy ke VPS'). Log: journalctl -u nvr-api -f / -u nvr-web -f"
