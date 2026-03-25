#!/usr/bin/env bash
# Snapshot DC (node data-collection) vs gateway profile file exports for E2E comparison.
# Prereqs: profiling.nodeFileExport and profiling.gatewayFileExport enabled (see scripts/profiling-enable-values.yaml).
#
# Usage:
#   ODIGOS_NAMESPACE=odigos-system ./scripts/profiling-dc-gateway-snapshot.sh
#   OUT_DIR=/tmp/odigos-prof-snap ./scripts/profiling-dc-gateway-snapshot.sh
set -euo pipefail

NS="${ODIGOS_NAMESPACE:-odigos-system}"
OUT_DIR="${OUT_DIR:-./profiling-snapshot-$(date +%Y%m%d-%H%M%S)}"
GATEWAY_PATH="${GATEWAY_PROFILES_JSONL:-/var/odigos/profiles-export/profiles.jsonl}"
NODE_PATH="${NODE_PROFILES_JSONL:-/var/odigos/node-profiles-export/profiles.jsonl}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

mkdir -p "$OUT_DIR"

need_cmd() { command -v "$1" >/dev/null 2>&1 || { echo "missing: $1" >&2; exit 1; }; }
need_cmd kubectl

echo "Namespace: $NS  OUT_DIR: $OUT_DIR" >&2

kubectl get cm odigos-configuration -n "$NS" -o jsonpath='{.data.config\.yaml}' >"$OUT_DIR/odigos-configuration.yaml" 2>/dev/null || true
kubectl get cm odigos-gateway -n "$NS" -o jsonpath="{.data['collector-conf']}" >"$OUT_DIR/odigos-gateway-collector-conf.yaml" 2>/dev/null || true
kubectl get cm odigos-data-collection -n "$NS" -o jsonpath='{.data.conf}' >"$OUT_DIR/odigos-data-collection-conf.yaml" 2>/dev/null || true

ODIGLET_POD="$(kubectl get pods -n "$NS" -l app.kubernetes.io/name=odiglet -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
if [[ -n "$ODIGLET_POD" ]]; then
  echo "DC pod: $ODIGLET_POD (data-collection container)" >&2
  if kubectl exec -n "$NS" "$ODIGLET_POD" -c data-collection -- test -f "$NODE_PATH" 2>/dev/null; then
    kubectl exec -n "$NS" "$ODIGLET_POD" -c data-collection -- cat "$NODE_PATH" 2>/dev/null | head -n 50 >"$OUT_DIR/node-profiles.head50.jsonl" || true
  else
    echo "WARN: node file export not found at $NODE_PATH (enable profiling.nodeFileExport; wait for samples)" >&2
  fi
else
  echo "WARN: no odiglet pod found" >&2
fi

if kubectl exec -n "$NS" deploy/odigos-gateway -- test -f "$GATEWAY_PATH" 2>/dev/null; then
  kubectl exec -n "$NS" deploy/odigos-gateway -- cat "$GATEWAY_PATH" 2>/dev/null | head -n 50 >"$OUT_DIR/gateway-profiles.head50.jsonl" || true
else
  echo "WARN: gateway file export not found at $GATEWAY_PATH" >&2
fi

if [[ -f "$OUT_DIR/node-profiles.head50.jsonl" ]] && [[ -s "$OUT_DIR/node-profiles.head50.jsonl" ]]; then
  python3 "$SCRIPT_DIR/parse_profiles_jsonl.py" --audit-dictionary "$OUT_DIR/node-profiles.head50.jsonl" >/dev/null 2>"$OUT_DIR/node-profiles.audit.stderr" || true
fi
if [[ -f "$OUT_DIR/gateway-profiles.head50.jsonl" ]] && [[ -s "$OUT_DIR/gateway-profiles.head50.jsonl" ]]; then
  python3 "$SCRIPT_DIR/parse_profiles_jsonl.py" --audit-dictionary "$OUT_DIR/gateway-profiles.head50.jsonl" >/dev/null 2>"$OUT_DIR/gateway-profiles.audit.stderr" || true
fi

echo "Done. Compare:" >&2
echo "  $OUT_DIR/odigos-data-collection-conf.yaml   (expect file/node-profiles exporter when nodeFileExport enabled)" >&2
echo "  $OUT_DIR/odigos-gateway-collector-conf.yaml   (expect file/gateway-profiles)" >&2
echo "  $OUT_DIR/node-profiles.head50.jsonl vs gateway-profiles.head50.jsonl" >&2
echo "$OUT_DIR"
