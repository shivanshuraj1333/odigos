#!/usr/bin/env bash
# Smoke test via curl — Odigos UI profiling GraphQL.
# Starts kubectl port-forward to the UI service, then runs HTTP checks (curl only for HTTP;
# jq is only used to build JSON bodies for GraphQL variables).
#
# Usage:
#   ./scripts/smoke-test-curl.sh <namespace> <workload-name>
# Example:
#   ./scripts/smoke-test-curl.sh default frontend
#
# Env overrides:
#   BASE=http://127.0.0.1:3000          # must match LOCAL_UI_PORT if you change the port
#   LOCAL_UI_PORT=3000                # local port for kubectl port-forward (and default BASE)
#   PF_NAMESPACE=odigos-system        # Kubernetes namespace of the UI Service
#   PF_SERVICE=ui                     # Service name (port 3000 expected)
#   PF_REMOTE_PORT=3000               # container/service port
#   SKIP_PORT_FORWARD=0               # set to 1 if you already run port-forward elsewhere
#   KUBECTL_CONTEXT=                  # optional: kubectl --context
#   COOKIE_JAR=/tmp/odigos-ui-cookies.txt
#   PROFILE_KIND=Deployment
#   WAIT_SECONDS=15
#   PROBE_METRICS=0
#   HEALTH_WAIT_SECONDS=60            # max time to wait for /healthz after port-forward
#   PROFILE_POLL_MAX_SECONDS=120      # after WAIT_SECONDS, poll sourceProfiling until samples or timeout
#   PROFILE_POLL_INTERVAL=5           # seconds between polls
#   SMOKE_FAIL_ON_EMPTY=0             # set to 1 to exit 1 if no profile samples after polling
#   PROFILE_DEBUG_INJECT=0            # set to 1 to POST /api/debug/profiling/inject-sample (requires UI ODIGOS_PROFILE_DEBUG_INJECT=true)

set -euo pipefail

LOCAL_UI_PORT="${LOCAL_UI_PORT:-3000}"
BASE="${BASE:-http://127.0.0.1:${LOCAL_UI_PORT}}"
COOKIE_JAR="${COOKIE_JAR:-/tmp/odigos-ui-cookies.txt}"
PROFILE_KIND="${PROFILE_KIND:-Deployment}"
WAIT_SECONDS="${WAIT_SECONDS:-15}"
PROBE_METRICS="${PROBE_METRICS:-0}"
PF_NAMESPACE="${PF_NAMESPACE:-odigos-system}"
PF_SERVICE="${PF_SERVICE:-ui}"
PF_REMOTE_PORT="${PF_REMOTE_PORT:-3000}"
SKIP_PORT_FORWARD="${SKIP_PORT_FORWARD:-0}"
KUBECTL_CONTEXT="${KUBECTL_CONTEXT:-}"
HEALTH_WAIT_SECONDS="${HEALTH_WAIT_SECONDS:-60}"
PROFILE_POLL_MAX_SECONDS="${PROFILE_POLL_MAX_SECONDS:-120}"
PROFILE_POLL_INTERVAL="${PROFILE_POLL_INTERVAL:-5}"
SMOKE_FAIL_ON_EMPTY="${SMOKE_FAIL_ON_EMPTY:-0}"
PROFILE_DEBUG_INJECT="${PROFILE_DEBUG_INJECT:-0}"

PF_PID=""

cleanup() {
  if [[ -n "${PF_PID}" ]] && kill -0 "${PF_PID}" 2>/dev/null; then
    kill "${PF_PID}" 2>/dev/null || true
    wait "${PF_PID}" 2>/dev/null || true
  fi
}
trap cleanup EXIT

if [[ $# -ge 2 ]]; then
  PROFILE_NAMESPACE="$1"
  PROFILE_NAME="$2"
elif [[ -n "${PROFILE_NAMESPACE:-}" && -n "${PROFILE_NAME:-}" ]]; then
  :
else
  echo "Usage: $0 <namespace> <workload-name>"
  exit 1
fi

if ! command -v jq &>/dev/null; then
  echo "jq is required to build GraphQL JSON bodies safely."
  exit 1
fi

# macOS Bash 3.2 + set -u: "${empty[@]}" is treated as unbound — branch instead of expanding an empty array.
kubectl_port_forward() {
  if [[ -n "${KUBECTL_CONTEXT}" ]]; then
    kubectl --context "${KUBECTL_CONTEXT}" "$@"
  else
    kubectl "$@"
  fi
}

if [[ "${SKIP_PORT_FORWARD}" != "1" ]]; then
  echo "==> kubectl port-forward -n ${PF_NAMESPACE} svc/${PF_SERVICE} ${LOCAL_UI_PORT}:${PF_REMOTE_PORT}"
  kubectl_port_forward port-forward -n "${PF_NAMESPACE}" "svc/${PF_SERVICE}" "${LOCAL_UI_PORT}:${PF_REMOTE_PORT}" &
  PF_PID=$!

  echo "==> wait for ${BASE}/healthz (up to ${HEALTH_WAIT_SECONDS}s)"
  _deadline=$((SECONDS + HEALTH_WAIT_SECONDS))
  while (( SECONDS < _deadline )); do
    if curl -sf -o /dev/null "${BASE}/healthz"; then
      echo "    healthz OK"
      break
    fi
    sleep 1
  done
  if ! curl -sf -o /dev/null "${BASE}/healthz"; then
    echo "Timed out waiting for ${BASE}/healthz — check cluster / port / BASE."
    exit 1
  fi
else
  echo "==> SKIP_PORT_FORWARD=1 — assuming UI is already reachable at ${BASE}"
  if ! curl -sf -o /dev/null "${BASE}/healthz"; then
    echo "UI not reachable at ${BASE}/healthz — start port-forward or fix BASE."
    exit 1
  fi
fi

rm -f "${COOKIE_JAR}"

echo "==> curl GET ${BASE}/healthz"
curl -sS -o /dev/null -w "HTTP %{http_code}\n" "${BASE}/healthz"

echo "==> curl GET ${BASE}/auth/csrf-token (cookie jar + CSRF)"
CSRF_JSON=$(curl -sS -c "${COOKIE_JAR}" -b "${COOKIE_JAR}" "${BASE}/auth/csrf-token")
CSRF_TOKEN="$(echo "$CSRF_JSON" | sed -n 's/.*"csrf_token":"\([^"]*\)".*/\1/p')"
if [[ -z "$CSRF_TOKEN" ]]; then
  echo "Failed to parse csrf_token"
  exit 1
fi

HDR=( -H "Content-Type: application/json" -H "X-CSRF-Token: ${CSRF_TOKEN}" -b "${COOKIE_JAR}" )

# Fetch sourceProfiling JSON and print jq summary: numTicks, chunkCount, debugReason.
fetch_source_profiling_json() {
  curl -sS "${HDR[@]}" \
    -d "$(jq -nc \
      --arg ns "$PROFILE_NAMESPACE" \
      --arg k "$PROFILE_KIND" \
      --arg n "$PROFILE_NAME" \
      '{query: "query ($ns: String!, $k: String!, $n: String!) { sourceProfiling(namespace: $ns, kind: $k, name: $n, debug: true) { profileJson debugJson debugReason } }", variables: {ns: $ns, k: $k, n: $n}}')" \
    "${BASE}/graphql"
}

echo "==> curl POST ${BASE}/graphql — profilingSlots"
curl -sS "${HDR[@]}" \
  -d '{"query":"query { profilingSlots { activeKeys keysWithData totalBytesUsed maxSlots slotMaxBytes maxTotalBytesBudget slotTtlSeconds } }"}' \
  "${BASE}/graphql" | jq .

echo "==> curl POST ${BASE}/graphql — enableSourceProfiling"
curl -sS "${HDR[@]}" \
  -d "$(jq -nc \
    --arg ns "$PROFILE_NAMESPACE" \
    --arg k "$PROFILE_KIND" \
    --arg n "$PROFILE_NAME" \
    '{query: "mutation ($ns: String!, $k: String!, $n: String!) { enableSourceProfiling(namespace: $ns, kind: $k, name: $n) { status sourceKey maxSlots activeSlots } }", variables: {ns: $ns, k: $k, n: $n}}')" \
  "${BASE}/graphql" | jq .

if [[ "${PROFILE_DEBUG_INJECT}" == "1" ]]; then
  echo "==> curl POST ${BASE}/api/debug/profiling/inject-sample (synthetic chunk; UI must have ODIGOS_PROFILE_DEBUG_INJECT=true)"
  _inj_resp="$(curl -sS -X POST "${BASE}/api/debug/profiling/inject-sample" \
    -H "Content-Type: application/json" \
    -d "$(jq -nc \
      --arg ns "$PROFILE_NAMESPACE" \
      --arg k "$PROFILE_KIND" \
      --arg n "$PROFILE_NAME" \
      '{namespace: $ns, kind: $k, name: $n}')")"
  echo "${_inj_resp}" | jq . 2>/dev/null || echo "${_inj_resp}"
fi

echo "==> sleep ${WAIT_SECONDS}s (initial wait for OTLP profile batches)"
sleep "${WAIT_SECONDS}"

echo "==> poll sourceProfiling (debug) — up to ${PROFILE_POLL_MAX_SECONDS}s every ${PROFILE_POLL_INTERVAL}s until numTicks>0 or chunkCount>0"
_deadline_poll=$((SECONDS + PROFILE_POLL_MAX_SECONDS))
LAST_SP_JSON=""
HAS_SAMPLES=0
_poll_n=0
while (( SECONDS < _deadline_poll )); do
  LAST_SP_JSON="$(fetch_source_profiling_json)"
  _poll_n=$((_poll_n + 1))
  NT="$(echo "${LAST_SP_JSON}" | jq -r 'try (.data.sourceProfiling.profileJson | fromjson | .flamebearer.numTicks) catch 0')"
  CC="$(echo "${LAST_SP_JSON}" | jq -r 'try (.data.sourceProfiling.debugJson | fromjson | .chunkCount) catch 0')"
  DR="$(echo "${LAST_SP_JSON}" | jq -r '.data.sourceProfiling.debugReason // empty')"
  echo "    poll #${_poll_n}: numTicks=${NT} chunkCount=${CC} debugReason=${DR}"
  if [[ "${NT}" =~ ^[0-9]+$ ]] && (( NT > 0 )); then
    HAS_SAMPLES=1
    echo "==> SUCCESS: profile has numTicks=${NT} (profiling data visible for this source)"
    echo "${LAST_SP_JSON}" | jq .
    break
  fi
  if (( SECONDS + PROFILE_POLL_INTERVAL >= _deadline_poll )); then
    break
  fi
  sleep "${PROFILE_POLL_INTERVAL}"
done

if [[ "${HAS_SAMPLES}" != "1" && -n "${LAST_SP_JSON}" ]]; then
  echo "==> last sourceProfiling response (no numTicks>0):"
  echo "${LAST_SP_JSON}" | jq .
fi

if [[ "${HAS_SAMPLES}" != "1" ]]; then
  echo "==> NO_SAMPLES after poll: no numTicks>0 (check gateway→UI OTLP profiles export, k8s attributes on profiles, and odigos_ui_profiling_* metrics). Rebuild odigos-ui with frontend deps aligned to odigos-gateway (OpenTelemetry Collector v0.148 OTLP profiles gRPC)."
  if [[ "${SMOKE_FAIL_ON_EMPTY}" == "1" ]]; then
    exit 1
  fi
fi

if [[ "${PROBE_METRICS}" == "1" ]]; then
  echo "==> curl GET ${BASE}/metrics (dev)"
  curl -sS "${BASE}/metrics" | grep '^odigos_ui_profiling_' || true
fi

echo "Done."
