#!/usr/bin/env bash
# Builds and installs the edge agent as a systemd service on a site's own
# machine — a mini PC/NUC/Raspberry Pi that stays permanently on the same
# LAN as the cameras there (see README "Rekomendasi hardware"). Never run
# this on the central VPS; a camera's RTSP is only reachable from its own
# LAN (see README "Site vs Workspace").
#
# No AGENT_TOKEN needed: the service starts in pairing mode on first boot.
# Generate a pairing code in Admin -> Sites & Kamera on the dashboard, then
# open this machine's local setup page in a browser on the same network
# (its address is printed to the service log — see the command this script
# prints at the end) and enter the code. The token is then saved locally
# (TOKEN_FILE) so every restart after that starts normally, no re-pairing.
#
# Usage:
#   API_BASE_URL=https://api.domain-kamu.com ./scripts/deploy-edge-agent.sh
#
# Env overrides:
#   INSTALL_DIR=/opt/nvr-agent
#   RUN_USER=$(whoami)
#   API_BASE_URL       REQUIRED — your central VPS's public domain
#   MEDIAMTX_HOST       optional, e.g. live.domain-kamu.com:8554, for live view push
#   PAIRING_PORT=8091   local setup page port (must be reachable from
#                       another device on this LAN during first-time pairing)

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

install_dir="${INSTALL_DIR:-/opt/nvr-agent}"
run_user="${RUN_USER:-$(whoami)}"
api_base_url="${API_BASE_URL:?Set API_BASE_URL ke domain VPS pusatmu, mis. API_BASE_URL=https://api.domain-kamu.com ./scripts/deploy-edge-agent.sh}"
mediamtx_host="${MEDIAMTX_HOST:-}"
pairing_port="${PAIRING_PORT:-8091}"

if [ "$(id -u)" -ne 0 ] && ! sudo -n true 2>/dev/null; then
  echo "Script ini menulis systemd unit dan butuh sudo — jalankan sebagai root, atau pastikan sudo tanpa password tersedia." >&2
fi

sudo mkdir -p "$install_dir/recordings"
sudo chown -R "$run_user" "$install_dir"

echo "Building edge agent..."
(cd apps/edge-agent && CGO_ENABLED=0 go build -o "$install_dir/agent" ./cmd/agent)

# Sengaja tidak menulis AGENT_TOKEN — biarkan kosong supaya masuk mode
# pairing di boot pertama. Kalau file .env ini sudah ada dari instalasi
# sebelumnya (mis. re-run script untuk update binary), JANGAN ditimpa —
# token yang sudah ke-pairing (kalau ada) tetap dipertahankan di TOKEN_FILE.
if [ ! -f "$install_dir/.env" ]; then
  cat <<EOF | sudo tee "$install_dir/.env" > /dev/null
API_BASE_URL=$api_base_url
RECORD_DIR=$install_dir/recordings
TOKEN_FILE=$install_dir/agent_token.json
PAIRING_PORT=$pairing_port
MEDIAMTX_HOST=$mediamtx_host
EOF
else
  echo "$install_dir/.env sudah ada, tidak ditimpa (biar token yang sudah ke-pairing tidak hilang)."
fi

sed -e "s#{{INSTALL_DIR}}#$install_dir#g" -e "s#{{RUN_USER}}#$run_user#g" \
  scripts/systemd/nvr-agent.service.template | sudo tee /etc/systemd/system/nvr-agent.service > /dev/null

sudo systemctl daemon-reload
sudo systemctl enable nvr-agent
sudo systemctl restart nvr-agent

echo ""
echo "Edge agent terpasang. Kalau ini instalasi baru (belum pernah pairing), cek log untuk alamat setup:"
echo "  sudo journalctl -u nvr-agent -f"
echo "Lalu buka alamat itu di browser pada jaringan yang sama, masukkan kode pairing dari dashboard."
