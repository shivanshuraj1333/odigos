#!/usr/bin/env bash
# End-to-end profiling smoke test:
# 1) Assumes gateway file exporter is ON (Helm: scripts/profiling-enable-values.yaml → profiling.gatewayFileExport).
# 2) Calls UI HTTP API (curl) to enable a slot and dumps JSON for flamegraph / debug.
# 3) Optionally copies gateway profiles.jsonl from the odigos-gateway pod.
#
# Prereq: kubectl port-forward -n "${ODIGOS_NAMESPACE}" svc/ui "${UI_PORT}:3000"
#   (or set UI_BASE_URL to an existing UI URL).
#
# Usage:
#   NS=default KIND=Deployment NAME=my-app ./scripts/profiling-test-and-dump.sh
#   OUT_DIR=./profiling-dumps NS=odigos-system KIND=Deployment NAME=scheduler ./scripts/profiling-test-and-dump.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

ODIGOS_NAMESPACE="${ODIGOS_NAMESPACE:-odigos-system}"
UI_PORT="${UI_PORT:-3000}"
UI_BASE_URL="${UI_BASE_URL:-http://127.0.0.1:${UI_PORT}}"
NS="${NS:-default}"
KIND="${KIND:-Deployment}"
NAME="${NAME:-}"
OUT_DIR="${OUT_DIR:-${REPO_ROOT}/profiling-dumps/run-$(date -u +%Y%m%dT%H%M%SZ)}"
GATEWAY_PROFILES_PATH="${GATEWAY_PROFILES_PATH:-/var/odigos/profiles-export/profiles.jsonl}"
START_PORT_FORWARD="${START_PORT_FORWARD:-false}"
PF_PID=""

cleanup() {
  if [[ -n "${PF_PID}" ]] && kill -0 "${PF_PID}" 2>/dev/null; then
    kill "${PF_PID}" 2>/dev/null || true
    wait "${PF_PID}" 2>/dev/null || true
  fi
}
trap cleanup EXIT

need_cmd() { command -v "$1" >/dev/null 2>&1 || { echo "missing: $1" >&2; exit 1; }; }

if [[ -z "$NAME" ]]; then
  echo "Set NAME= to a workload Odigos knows about (source), e.g. NAME=my-deployment" >&2
  echo "Example: NS=default KIND=Deployment NAME=frontend $0" >&2
  exit 1
fi

need_cmd curl
if [[ "${START_PORT_FORWARD}" == "true" ]]; then
  need_cmd kubectl
  kubectl port-forward -n "${ODIGOS_NAMESPACE}" svc/ui "${UI_PORT}:3000" &
  PF_PID=$!
  sleep "${PORT_FORWARD_WAIT_SECONDS:-5}"
fi

mkdir -p "${OUT_DIR}"

enc_kind() {
  # Gin path — encode spaces / special chars lightly (Deployment stays as-is)
  printf '%s' "$1" | sed 's/ /%20/g'
}

KIND_ENC="$(enc_kind "$KIND")"
BASE_API="${UI_BASE_URL}/api"

echo "=== Profiling dump ===" | tee "${OUT_DIR}/00-info.txt"
{
  echo "UI_BASE_URL=${UI_BASE_URL}"
  echo "source namespace/kind/name=${NS}/${KIND}/${NAME}"
  echo "OUT_DIR=${OUT_DIR}"
  echo "gateway file path (in pod)=${GATEWAY_PROFILES_PATH}"
  echo "Helm: merge scripts/profiling-enable-values.yaml so profiling.gatewayFileExport.enabled is true"
} | tee -a "${OUT_DIR}/00-info.txt"

echo ""
echo "=== 1) Debug: active slots (before) ==="
curl -sS "${BASE_API}/debug/profiling-slots" | tee "${OUT_DIR}/01-profiling-slots-before.json"
echo ""

echo ""
echo "=== 2) PUT enable profiling slot ==="
curl -sS -X PUT "${BASE_API}/sources/${NS}/${KIND_ENC}/${NAME}/profiling/enable" \
  | tee "${OUT_DIR}/02-enable-response.json"
echo ""

echo ""
echo "=== 3) GET aggregated profile (Flamebearer JSON for UI) ==="
curl -sS "${BASE_API}/sources/${NS}/${KIND_ENC}/${NAME}/profiling" \
  | tee "${OUT_DIR}/03-profile-flamebearer.json"
echo ""

echo ""
echo "=== 4) GET profile with debug=1 ==="
curl -sS "${BASE_API}/sources/${NS}/${KIND_ENC}/${NAME}/profiling?debug=1" \
  | tee "${OUT_DIR}/04-profile-debug.json"
echo ""

echo ""
echo "=== 5) GET raw first OTLP chunk (debug) ==="
set +e
curl -sS -o "${OUT_DIR}/05-profiling-chunk-raw.json" -w "http_code=%{http_code}\n" \
  "${BASE_API}/debug/sources/${NS}/${KIND_ENC}/${NAME}/profiling-chunk"
CHUNK_RC=$?
set -e
echo "chunk curl exit=${CHUNK_RC} (404 is OK if no data yet)"

echo ""
echo "=== 6) Debug: active slots (after) ==="
curl -sS "${BASE_API}/debug/profiling-slots" | tee "${OUT_DIR}/06-profiling-slots-after.json"
echo ""

if command -v kubectl >/dev/null 2>&1 && kubectl cluster-info >/dev/null 2>&1; then
  echo ""
  echo "=== 7) Gateway file exporter (profiles.jsonl on disk) ==="
  {
    echo "Helm: profiling.gatewayFileExport.enabled (see scripts/profiling-enable-values.yaml)."
    echo "Path in gateway pod: ${GATEWAY_PROFILES_PATH}"
    echo "Note: odigos-gateway often uses a distroless image (no tar/sh). kubectl cp may fail; use curl dumps (03/04) for Flamebearer JSON."
  } | tee "${OUT_DIR}/07-gateway-file-export-README.txt"
  if kubectl get deploy odigos-gateway -n "${ODIGOS_NAMESPACE}" >/dev/null 2>&1; then
    GW_POD="$(kubectl get pod -n "${ODIGOS_NAMESPACE}" -l odigos.io/collector-role=CLUSTER_GATEWAY -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
    if [[ -n "${GW_POD}" ]]; then
      echo "gateway pod=${GW_POD}" | tee -a "${OUT_DIR}/07-gateway-file-export-README.txt"
      if kubectl cp "${ODIGOS_NAMESPACE}/${GW_POD}:${GATEWAY_PROFILES_PATH}" "${OUT_DIR}/07-gateway-profiles.jsonl" 2>"${OUT_DIR}/07-gateway-kubectl-cp.log"; then
        wc -c < "${OUT_DIR}/07-gateway-profiles.jsonl" | tee "${OUT_DIR}/07-gateway-jsonl-bytes.txt"
        tail -n 50 "${OUT_DIR}/07-gateway-profiles.jsonl" | tee "${OUT_DIR}/07-gateway-profiles-tail.jsonl"
      else
        cat "${OUT_DIR}/07-gateway-kubectl-cp.log" >> "${OUT_DIR}/07-gateway-file-export-README.txt" 2>/dev/null || true
        echo "Optional: kubectl debug -n ${ODIGOS_NAMESPACE} pod/${GW_POD} -it --image=busybox:1.36 --target=gateway -- sh" >> "${OUT_DIR}/07-gateway-file-export-README.txt"
      fi
    else
      echo "no odigos-gateway pod" | tee "${OUT_DIR}/07-gateway-skip.txt"
    fi
  else
    echo "deploy/odigos-gateway not found in ${ODIGOS_NAMESPACE}" | tee "${OUT_DIR}/07-gateway-skip.txt"
  fi
else
  echo "kubectl not configured — see 07-gateway-file-export-README.txt for manual steps" | tee "${OUT_DIR}/07-gateway-skip.txt"
fi

echo ""
echo "Done. Artifacts under: ${OUT_DIR}"
echo "Tip: generate CPU load on ${NS}/${NAME}, wait ~30–120s, re-run GET steps or re-fetch chunk."
