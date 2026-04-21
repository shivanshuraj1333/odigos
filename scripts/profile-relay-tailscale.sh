#!/usr/bin/env bash
# Relay Odigos gateway OTLP profiles (gRPC) from this VM's VPC interface to your Mac's
# Tailscale IP, where the Odigos UI listens on :4317.
#
# Usage:
#   export MAC_TAILSCALE_IP=100.x.y.z   # from Mac: tailscale ip -4
#   sudo ./scripts/profile-relay-tailscale.sh
#
# Optional: bind only on VPC private IP (default 0.0.0.0 = all interfaces).
#   export RELAY_BIND=172.31.x.x
#   export RELAY_PORT=4317
#
# Persistent relay on this VM (systemd): edit /etc/odigos-profile-relay.env
# (see /etc/odigos-profile-relay.env.example), then:
#   sudo systemctl enable --now odigos-profile-relay
set -euo pipefail

MAC_IP="${MAC_TAILSCALE_IP:-${1:-}}"
if [[ -z "${MAC_IP}" ]]; then
  echo "Set MAC_TAILSCALE_IP to your Mac's Tailscale IPv4 (tailscale ip -4 on the Mac), or pass it as the first argument." >&2
  exit 1
fi

BIND="${RELAY_BIND:-0.0.0.0}"
PORT="${RELAY_PORT:-4317}"

echo "Relay ${BIND}:${PORT} -> ${MAC_IP}:4317 (stop with Ctrl+C or kill the process)"
exec socat "TCP-LISTEN:${PORT},bind=${BIND},fork,reuseaddr" "TCP:${MAC_IP}:4317"
