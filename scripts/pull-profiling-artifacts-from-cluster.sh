#!/usr/bin/env bash
# Pull profiling debug files from an EKS (or any) cluster to this machine (laptop / build host).
# Requires: kubectl with a valid kubeconfig (e.g. export KUBECONFIG=~/.kube/eks.config), namespace access.
#
# Writes:
#   gateway-profiles.jsonl       — same OTLP JSON lines the file exporter sees (compare to UI path)
#   node-profiles.jsonl          — first odiglet pod data-collection (if nodeFileExport enabled)
#   ui-profile-dumps.tar         — optional tar of /data/profile-dumps from odigos-ui (needs PROFILE_DEBUG_DUMP_DIR)
#
# Usage:
#   export KUBECONFIG=~/.kube/eks.config
#   ODIGOS_NAMESPACE=odigos-system ./scripts/pull-profiling-artifacts-from-cluster.sh
#   OUT_DIR=~/odigos-prof-pull ./scripts/pull-profiling-artifacts-from-cluster.sh --skip-ui-dumps

set -euo pipefail

NS="${ODIGOS_NAMESPACE:-odigos-system}"
OUT_DIR="${OUT_DIR:-./profiling-artifacts-$(date +%Y%m%d-%H%M%S)}"
GATEWAY_JSONL="${GATEWAY_PROFILES_JSONL:-/var/odigos/profiles-export/profiles.jsonl}"
NODE_JSONL="${NODE_PROFILES_JSONL:-/var/odigos/node-profiles-export/profiles.jsonl}"
SKIP_UI=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-ui-dumps) SKIP_UI=true; shift ;;
    -n|--namespace) NS="${2:-}"; shift 2 ;;
    *) echo "unknown arg: $1" >&2; exit 1 ;;
  esac
done

mkdir -p "$OUT_DIR"
echo "Namespace=$NS  OUT_DIR=$OUT_DIR" >&2

need() { command -v "$1" >/dev/null 2>&1 || { echo "missing: $1" >&2; exit 1; }; }
need kubectl

kubectl cluster-info >/dev/null 2>&1 || { echo "kubectl: cannot reach cluster (check KUBECONFIG)" >&2; exit 1; }

# Gateway: full jsonl (can be large; user may ctrl-c)
if kubectl -n "$NS" get deploy odigos-gateway >/dev/null 2>&1; then
  if kubectl exec -n "$NS" deploy/odigos-gateway -- test -f "$GATEWAY_JSONL" 2>/dev/null; then
    kubectl exec -n "$NS" deploy/odigos-gateway -- cat "$GATEWAY_JSONL" >"$OUT_DIR/gateway-profiles.jsonl" 2>/dev/null || true
    echo "Wrote $OUT_DIR/gateway-profiles.jsonl" >&2
  else
    echo "WARN: gateway file missing at $GATEWAY_JSONL (enable profiling.gatewayFileExport)" >&2
  fi
else
  echo "WARN: deploy/odigos-gateway not found in $NS" >&2
fi

POD="$(kubectl get pods -n "$NS" -l app.kubernetes.io/name=odiglet -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
if [[ -n "$POD" ]]; then
  if kubectl exec -n "$NS" "$POD" -c data-collection -- test -f "$NODE_JSONL" 2>/dev/null; then
    kubectl exec -n "$NS" "$POD" -c data-collection -- cat "$NODE_JSONL" >"$OUT_DIR/node-profiles.jsonl" 2>/dev/null || true
    echo "Wrote $OUT_DIR/node-profiles.jsonl (pod $POD)" >&2
  else
    echo "WARN: node file missing at $NODE_JSONL (enable profiling.nodeFileExport)" >&2
  fi
else
  echo "WARN: no odiglet pod" >&2
fi

if [[ "$SKIP_UI" != true ]] && kubectl -n "$NS" get deploy odigos-ui >/dev/null 2>&1; then
  if kubectl exec -n "$NS" deploy/odigos-ui -- test -d /data/profile-dumps 2>/dev/null; then
    kubectl exec -n "$NS" deploy/odigos-ui -- tar cf - -C /data profile-dumps 2>/dev/null >"$OUT_DIR/ui-profile-dumps.tar" || true
    if [[ -s "$OUT_DIR/ui-profile-dumps.tar" ]]; then
      echo "Wrote $OUT_DIR/ui-profile-dumps.tar (raw chunks as received by UI OTLP)" >&2
    fi
  else
    echo "INFO: /data/profile-dumps not on UI yet (set profiling.enabled + rollout UI with PROFILE_DEBUG_DUMP_DIR)" >&2
  fi
fi

echo "Done. Analyze locally:" >&2
echo "  python3 scripts/parse_profiles_jsonl.py --audit-dictionary --require-nonempty-dictionary $OUT_DIR/gateway-profiles.jsonl" >&2
echo "  tar -tf $OUT_DIR/ui-profile-dumps.tar  # if present" >&2
echo "$OUT_DIR"
