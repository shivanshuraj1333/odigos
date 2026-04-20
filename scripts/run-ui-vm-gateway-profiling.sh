#!/usr/bin/env bash
# Run Odigos UI on this VM so the EKS cluster gateway can send profiling OTLP gRPC to
# <this host>:4317 (must match Helm values.profiling.gatewayUiOtlpEndpoint).
#
# Prerequisites:
#   - Same VPC (or routing) from EKS worker nodes to this instance on TCP 4317
#   - EC2 security group: inbound 4317 from EKS node / cluster SG (or VPC CIDR for testing)
#   - Helm: profiling.enabled=true and profiling.gatewayUiOtlpEndpoint="<THIS_IP>:4317"
#
# Usage:
#   ./scripts/run-ui-vm-gateway-profiling.sh
#   ODIGOS_NAMESPACE=odigos-system LISTEN_PORT=3001 ./scripts/run-ui-vm-gateway-profiling.sh
#
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEBAPP="$ROOT/frontend/webapp"
UI_KIT="${UI_KIT_DIR:-$ROOT/../ui-kit}"
LISTEN_PORT="${LISTEN_PORT:-3001}"
NS="${ODIGOS_NAMESPACE:-odigos-system}"
LOG="${ODIGOS_UI_LOCAL_LOG:-$ROOT/odigos-ui-local.log}"
# Set RESTART_GATEWAY=1 only if you can afford two gateway pods (HPA minReplicas is often 2; low CPU → Pending).
RESTART_GATEWAY="${RESTART_GATEWAY:-0}"

if [[ ! -f "$UI_KIT/package.json" ]]; then
  echo "Set UI_KIT_DIR to your ui-kit checkout (default: $ROOT/../ui-kit)" >&2
  exit 1
fi

THIS_IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
echo "==> VM primary IP: $THIS_IP (gateway should target ${THIS_IP}:4317)"

echo "==> stopping local processes that usually hold 4317/6060 (odigos-backend / old odigos-ui-local)"
pkill -f '[/]odigos-ui-local' 2>/dev/null || true
pkill -f '[/]odigos-backend' 2>/dev/null || true
sleep 2

if ss -tlnp 2>/dev/null | grep -q ':4317 '; then
  echo "ERROR: something is still listening on :4317 — free it before running this script." >&2
  ss -tlnp | grep ':4317 ' || true
  exit 1
fi

echo "==> yarn sync-ui-kit + build ($WEBAPP)"
cd "$WEBAPP"
UI_KIT_DIR="$UI_KIT" yarn sync-ui-kit
yarn build

echo "==> go build ($ROOT/frontend)"
cd "$ROOT/frontend"
go build -o odigos-ui-local .

echo "==> starting odigos-ui-local (HTTP 0.0.0.0:$LISTEN_PORT, OTLP 0.0.0.0:4317, pprof :16061) — log: $LOG"
# Unset ODIGOS_UI_OTLP_PORT so the UI uses the default 4317 (cluster gateway expectation).
unset ODIGOS_UI_OTLP_PORT || true
export ODIGOS_UI_PPROF_PORT="${ODIGOS_UI_PPROF_PORT:-16061}"
nohup env ODIGOS_UI_PPROF_PORT="$ODIGOS_UI_PPROF_PORT" \
  "$ROOT/frontend/odigos-ui-local" \
  --address 0.0.0.0 \
  --port "$LISTEN_PORT" \
  --namespace "$NS" \
  --debug >>"$LOG" 2>&1 &
echo $! >"$ROOT/odigos-ui-local.pid"
sleep 4

if ! ss -tlnp | grep -q ":$LISTEN_PORT"; then
  echo "HTTP listener failed; tail $LOG:" >&2
  tail -40 "$LOG" >&2
  exit 1
fi
if ! ss -tlnp | grep -q ':4317'; then
  echo "OTLP listener missing on :4317; tail $LOG:" >&2
  tail -40 "$LOG" >&2
  exit 1
fi

echo "Listeners:"
ss -tlnp | grep -E ":$LISTEN_PORT|:4317|:16061" || true
curl -sI "http://127.0.0.1:$LISTEN_PORT/overview" | head -3 || true

if [[ "$RESTART_GATEWAY" == "1" ]]; then
  echo "==> kubectl rollout restart deployment/odigos-gateway -n $NS (may wait if HPA cannot schedule second pod)"
  kubectl rollout restart deployment/odigos-gateway -n "$NS"
  kubectl rollout status deployment/odigos-gateway -n "$NS" --timeout=300s || true
fi

echo ""
echo "Done. Open UI: http://${THIS_IP}:$LISTEN_PORT/overview"
echo "Helm must have: profiling.gatewayUiOtlpEndpoint=\"${THIS_IP}:4317\""
echo "Check gateway logs if profiles do not arrive: kubectl logs -n $NS -l app.kubernetes.io/name=odigos-gateway --tail=80"
