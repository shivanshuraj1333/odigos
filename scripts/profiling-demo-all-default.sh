#!/usr/bin/env bash
# Demo: enable profiling + inject synthetic OTLP chunk for every Deployment in a namespace that has
# replicas, then print profilingSlots and per-workload chunk counts (GraphQL).
#
# Share this with the UI team to verify Profiler / GraphQL end-to-end without live ebpf traffic.
#
# Prerequisites:
#   - kubectl context points at the cluster with Odigos UI
#   - UI pod has ODIGOS_PROFILE_DEBUG_INJECT=true (dev inject route)
#   - jq installed
#
# Usage (from repo root):
#   ./scripts/profiling-demo-all-default.sh
#
# Env:
#   PROFILE_NAMESPACE=default     # workload namespace
#   SKIP_PORT_FORWARD=0           # set 1 if you already port-forward svc/ui to LOCAL_UI_PORT
#   LOCAL_UI_PORT=3000
#   PF_NAMESPACE=odigos-system
#   KUBECTL_CONTEXT=              # optional

set -euo pipefail

PROFILE_NAMESPACE="${PROFILE_NAMESPACE:-default}"
LOCAL_UI_PORT="${LOCAL_UI_PORT:-3000}"
BASE="${BASE:-http://127.0.0.1:${LOCAL_UI_PORT}}"
PF_NAMESPACE="${PF_NAMESPACE:-odigos-system}"
PF_SERVICE="${PF_SERVICE:-ui}"
PF_REMOTE_PORT="${PF_REMOTE_PORT:-3000}"
SKIP_PORT_FORWARD="${SKIP_PORT_FORWARD:-0}"
HEALTH_WAIT_SECONDS="${HEALTH_WAIT_SECONDS:-60}"
COOKIE_JAR="${COOKIE_JAR:-/tmp/odigos-profiling-demo-cookies.txt}"
KUBECTL_CONTEXT="${KUBECTL_CONTEXT:-}"

kubectl_pf() {
  if [[ -n "${KUBECTL_CONTEXT}" ]]; then
    kubectl --context "${KUBECTL_CONTEXT}" "$@"
  else
    kubectl "$@"
  fi
}

PF_PID=""
cleanup() {
  if [[ -n "${PF_PID}" ]] && kill -0 "${PF_PID}" 2>/dev/null; then
    kill "${PF_PID}" 2>/dev/null || true
    wait "${PF_PID}" 2>/dev/null || true
  fi
}
trap cleanup EXIT

if ! command -v jq &>/dev/null; then
  echo "jq is required."
  exit 1
fi

if [[ "${SKIP_PORT_FORWARD}" != "1" ]]; then
  echo "==> kubectl port-forward -n ${PF_NAMESPACE} svc/${PF_SERVICE} ${LOCAL_UI_PORT}:${PF_REMOTE_PORT}"
  kubectl_pf port-forward -n "${PF_NAMESPACE}" "svc/${PF_SERVICE}" "${LOCAL_UI_PORT}:${PF_REMOTE_PORT}" &
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
    echo "Timed out waiting for ${BASE}/healthz."
    exit 1
  fi
else
  echo "==> SKIP_PORT_FORWARD=1 — using ${BASE}"
  if ! curl -sf -o /dev/null "${BASE}/healthz"; then
    echo "UI not reachable at ${BASE}/healthz"
    exit 1
  fi
fi

rm -f "${COOKIE_JAR}"
CSRF_JSON=$(curl -sS -c "${COOKIE_JAR}" -b "${COOKIE_JAR}" "${BASE}/auth/csrf-token")
CSRF_TOKEN="$(echo "$CSRF_JSON" | sed -n 's/.*"csrf_token":"\([^"]*\)".*/\1/p')"
if [[ -z "$CSRF_TOKEN" ]]; then
  echo "Failed to parse csrf_token"
  exit 1
fi
HDR=( -H "Content-Type: application/json" -H "X-CSRF-Token: ${CSRF_TOKEN}" -b "${COOKIE_JAR}" )

echo ""
echo "==> Deployments in namespace ${PROFILE_NAMESPACE} (replicas > 0)"
kubectl_pf get deploy -n "${PROFILE_NAMESPACE}" -o json | jq -r '.items[] | select((.spec.replicas // 0) > 0) | .metadata.name' || true

echo ""
echo "==> enableSourceProfiling + POST /api/debug/profiling/inject-sample per Deployment"
while IFS= read -r d; do
  [[ -z "$d" ]] && continue
  curl -sS "${HDR[@]}" -d "$(jq -nc --arg ns "$PROFILE_NAMESPACE" --arg n "$d" \
    '{query: "mutation ($ns: String!, $k: String!, $n: String!) { enableSourceProfiling(namespace: $ns, kind: $k, name: $n) { status sourceKey activeSlots } }", variables: {ns: $ns, k: "Deployment", n: $n}}')" \
    "${BASE}/graphql" >/dev/null
  INJ=$(curl -sS -X POST "${BASE}/api/debug/profiling/inject-sample" -H "Content-Type: application/json" \
    -d "$(jq -nc --arg ns "$PROFILE_NAMESPACE" --arg n "$d" '{namespace: $ns, kind: "Deployment", name: $n}')")
  OK=$(echo "$INJ" | jq -r '.ok // false')
  echo "    ${d}: inject ok=${OK}"
done < <(kubectl_pf get deploy -n "${PROFILE_NAMESPACE}" -o json | jq -r '.items[] | select((.spec.replicas // 0) > 0) | .metadata.name')

echo ""
echo "==> GraphQL profilingSlots (JSON — services with buffered profile data)"
curl -sS "${HDR[@]}" -d '{"query":"query { profilingSlots { activeKeys keysWithData totalBytesUsed maxSlots slotTtlSeconds } }"}' "${BASE}/graphql" | jq .

echo ""
echo "==> Per-workload sourceProfiling debug (chunkCount / numTicks)"
while IFS= read -r d; do
  [[ -z "$d" ]] && continue
  R=$(curl -sS "${HDR[@]}" -d "$(jq -nc --arg ns "$PROFILE_NAMESPACE" --arg n "$d" \
    '{query: "query ($ns: String!, $k: String!, $n: String!) { sourceProfiling(namespace: $ns, kind: $k, name: $n, debug: true) { debugJson debugReason } }", variables: {ns: $ns, k: "Deployment", n: $n}}')" \
    "${BASE}/graphql")
  echo -n "    ${d}: "
  echo "$R" | jq -r 'if .data then (.data.sourceProfiling.debugJson | fromjson | "chunkCount=\(.chunkCount) numTicks=\(.numTicks)") else .errors end'
done < <(kubectl_pf get deploy -n "${PROFILE_NAMESPACE}" -o json | jq -r '.items[] | select((.spec.replicas // 0) > 0) | .metadata.name')

echo ""
echo "==> UI verification"
echo "    Port-forward: kubectl port-forward -n ${PF_NAMESPACE} svc/${PF_SERVICE} ${LOCAL_UI_PORT}:${PF_REMOTE_PORT}"
echo "    Open Odigos UI → Profiler → pick a workload listed in keysWithData above."
echo "    Synthetic stacks (runtime.main / main.foo / main.bar) come from testdata chunk when using inject-sample."
echo ""
echo "Done."
