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

TMP="$(mktemp)"
TMPN="$(mktemp)"
ERRF="$(mktemp)"
ERRN="$(mktemp)"
trap 'rm -f "$TMP" "$TMPN" "$ERRF" "$ERRN"' EXIT

# Odigos collector image is distroless: no `cat`/`sh`/`tar`, so kubectl exec cannot stream files.
kubectl exec -n "$NS" deploy/odigos-gateway -- cat "$GATEWAY_JSONL" 2>"$ERRF" >"$TMP" || true
if grep -qE 'executable file not found|not found in \$PATH' "$ERRF" 2>/dev/null; then
  note "SKIP: gateway collector is distroless (no cat). Cannot read profiles.jsonl from outside the pod."
  note "ConfigMap checks: use ./scripts/verify-profiling-pipeline.sh --skip-runtime. For payloads, port-forward UI and use /debug/* or pull profile dumps."
  exit 0
fi
[[ -s "$TMP" ]] || fail "gateway $GATEWAY_JSONL missing or empty — enable profiling.gatewayFileExport and wait for samples"

head -n "$MAX_LINES" "$TMP" >"${TMP}.h" && mv "${TMP}.h" "$TMP"

note "gateway: checking dictionary on first $MAX_LINES lines..."
python3 "$PY" --min-lines 1 --require-nonempty-dictionary --audit-dictionary "$TMP" >/dev/null || fail "gateway OTLP lines lack usable dictionary (UI will show frame_N)"

note "gateway: OK (dictionary present — matches what UI backend should receive over gRPC)."

if [[ "$CHECK_NODE" == true ]]; then
  POD="$(kubectl get pods -n "$NS" -l app.kubernetes.io/name=odiglet -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
  [[ -n "$POD" ]] || fail "no odiglet pod"
  ERRN="$(mktemp)"
  kubectl exec -n "$NS" "$POD" -c data-collection -- cat "$NODE_JSONL" 2>"$ERRN" >"$TMPN" || true
  if grep -qE 'executable file not found|not found in \$PATH' "$ERRN" 2>/dev/null; then
    note "SKIP: data-collection is distroless (no cat); cannot read node JSONL via kubectl exec."
    exit 0
  fi
  [[ -s "$TMPN" ]] || fail "node $NODE_JSONL missing or empty — enable profiling.nodeFileExport and wait for samples"
  head -n "$MAX_LINES" "$TMPN" >"${TMPN}.h" && mv "${TMPN}.h" "$TMPN"
  note "node (DC): checking dictionary..."
  python3 "$PY" --min-lines 1 --require-nonempty-dictionary --audit-dictionary "$TMPN" >/dev/null || \
    fail "node hop has no dictionary — issue is before gateway (profiler/collector)"
  note "node (DC): OK."
fi

note "All dictionary checks passed."
