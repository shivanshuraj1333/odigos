#!/usr/bin/env bash
# End-to-end: verify gateway profile OTLP JSON lines carry a non-empty dictionary (required for UI flame graph symbols).
# Run from the repo root on any machine with kubectl → cluster (same as EKS control plane access).
#
# What this checks:
#   - gateway file exporter output (same payload shape as the OTLP exporter sends to the UI on gRPC)
#   - optional: node file export (DC hop) to see if dictionary is lost before the gateway
#
# Usage:
#   ODIGOS_NAMESPACE=odigos-system ./scripts/check-profiles-dictionary-e2e.sh
#   ./scripts/check-profiles-dictionary-e2e.sh --max-lines 20

set -euo pipefail

NS="${ODIGOS_NAMESPACE:-odigos-system}"
MAX_LINES="${MAX_LINES:-500}"
CHECK_NODE=false
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PY="${SCRIPT_DIR}/parse_profiles_jsonl.py"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --namespace|-n) NS="${2:-}"; shift 2 ;;
    --max-lines) MAX_LINES="${2:-}"; shift 2 ;;
    --check-node) CHECK_NODE=true; shift ;;
    *) echo "unknown arg: $1" >&2; exit 1 ;;
  esac
done

need() { command -v "$1" >/dev/null 2>&1 || { echo "missing: $1" >&2; exit 1; }; }
need kubectl
need python3
[[ -f "$PY" ]] || { echo "missing $PY" >&2; exit 1; }

GATEWAY_JSONL="${GATEWAY_PROFILES_JSONL:-/var/odigos/profiles-export/profiles.jsonl}"
NODE_JSONL="${NODE_PROFILES_JSONL:-/var/odigos/node-profiles-export/profiles.jsonl}"

fail() { echo "check-profiles-dictionary-e2e: FAIL: $*" >&2; exit 1; }
note() { echo "check-profiles-dictionary-e2e: $*" >&2; }

kubectl exec -n "$NS" deploy/odigos-gateway -- test -f "$GATEWAY_JSONL" 2>/dev/null || \
  fail "gateway $GATEWAY_JSONL not found — enable profiling.gatewayFileExport and wait for samples"

TMP="$(mktemp)"
TMPN="$(mktemp)"
trap 'rm -f "$TMP" "$TMPN"' EXIT

kubectl exec -n "$NS" deploy/odigos-gateway -- cat "$GATEWAY_JSONL" 2>/dev/null | head -n "$MAX_LINES" >"$TMP" || true
[[ -s "$TMP" ]] || fail "gateway jsonl empty — generate CPU load on workloads"

note "gateway: checking dictionary on first $MAX_LINES lines..."
python3 "$PY" --min-lines 1 --require-nonempty-dictionary --audit-dictionary "$TMP" >/dev/null || fail "gateway OTLP lines lack usable dictionary (UI will show frame_N)"

note "gateway: OK (dictionary present — matches what UI backend should receive over gRPC)."

if [[ "$CHECK_NODE" == true ]]; then
  POD="$(kubectl get pods -n "$NS" -l app.kubernetes.io/name=odiglet -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
  [[ -n "$POD" ]] || fail "no odiglet pod"
  kubectl exec -n "$NS" "$POD" -c data-collection -- test -f "$NODE_JSONL" 2>/dev/null || \
    fail "node $NODE_JSONL not found — enable profiling.nodeFileExport"
  kubectl exec -n "$NS" "$POD" -c data-collection -- cat "$NODE_JSONL" 2>/dev/null | head -n "$MAX_LINES" >"$TMPN" || true
  [[ -s "$TMPN" ]] || fail "node jsonl empty"
  note "node (DC): checking dictionary..."
  python3 "$PY" --min-lines 1 --require-nonempty-dictionary --audit-dictionary "$TMPN" >/dev/null || \
    fail "node hop has no dictionary — issue is before gateway (profiler/collector)"
  note "node (DC): OK."
fi

note "All dictionary checks passed."
