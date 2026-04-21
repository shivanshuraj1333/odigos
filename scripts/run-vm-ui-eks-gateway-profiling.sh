#!/usr/bin/env bash
# Start Odigos UI on this VM with OTLP on :4317 and point EKS gateway profiling export here.
#
# Run on the VM (repo root):  ./scripts/run-vm-ui-eks-gateway-profiling.sh
#
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NS="${ODIGOS_NAMESPACE:-odigos-system}"
LISTEN_PORT="${LISTEN_PORT:-3001}"
LOG="${ODIGOS_UI_LOCAL_LOG:-$ROOT/odigos-ui-local.log}"

# IP the cluster must reach for gateway→UI OTLP (gRPC :4317). Override with VM_IP=... if auto-detect is wrong.
detect_vm_ip() {
  if [[ -n "${VM_IP:-}" ]]; then
    printf '%s' "$VM_IP"
    return
  fi
  if command -v curl >/dev/null 2>&1; then
    local token ip
    token=$(curl -sS -m 1 -X PUT "http://169.254.169.254/latest/api/token" \
      -H "X-aws-ec2-Metadata-Token-Ttl-Seconds: 21600" 2>/dev/null || true)
    if [[ -n "$token" ]]; then
      ip=$(curl -sS -m 1 -H "X-aws-ec2-Metadata-Token: $token" \
        "http://169.254.169.254/latest/meta-data/local-ipv4" 2>/dev/null || true)
      if [[ -n "$ip" ]]; then
        printf '%s' "$ip"
        return
      fi
    fi
  fi
  # Prefer first non-loopback IPv4 from hostname -I (127.* / :: are wrong for EKS→VM routing)
  if [[ -n "$(hostname -I 2>/dev/null)" ]]; then
    local cand
    cand=$(hostname -I 2>/dev/null | tr ' ' '\n' | awk '$0 !~ /^127\./ && $0 !~ /^::/ && $0 != "" {print; exit}')
    if [[ -n "$cand" ]]; then
      printf '%s' "$cand"
      return
    fi
    hostname -I 2>/dev/null | awk '{print $1; exit}'
    return
  fi
  printf ''
}

VM_IP="$(detect_vm_ip)"
if [[ -z "$VM_IP" ]]; then
  echo "ERROR: could not determine VM_IP (set VM_IP to the ENI IP nodes can reach)." >&2
  exit 1
fi

echo "VM_IP=$VM_IP (EKS gateway will use profiling.gatewayUiOtlpEndpoint=${VM_IP}:4317)"

echo "==> stop processes that bind 4317 / old UI"
pkill -f '[/]odigos-ui-local' 2>/dev/null || true
pkill -f '[/]odigos-backend' 2>/dev/null || true
sleep 2
if ss -tlnp 2>/dev/null | grep -q ':4317 '; then
  echo "ERROR: port 4317 still in use — free it first." >&2
  ss -tlnp | grep ':4317 ' || true
  exit 1
fi

echo "==> yarn build ($ROOT/frontend/webapp)"
cd "$ROOT/frontend/webapp"
yarn install --ignore-engines --network-timeout 600000
yarn build

echo "==> go build ($ROOT/frontend)"
cd "$ROOT/frontend"
go build -o odigos-ui-local .

echo "==> Helm: gateway profiles OTLP → VM (not in-cluster ui:4317)"
helm upgrade odigos "$ROOT/helm/odigos" -n "$NS" --reuse-values \
  --set profiling.enabled=true \
  --set-string "profiling.gatewayUiOtlpEndpoint=${VM_IP}:4317"

echo "==> restart autoscaler + gateway (pick up profiles-to-ui endpoint → ${VM_IP}:4317)"
kubectl rollout restart deployment/odigos-autoscaler -n "$NS"
kubectl rollout status deployment/odigos-autoscaler -n "$NS" --timeout=300s
kubectl rollout restart deployment/odigos-gateway -n "$NS" 2>/dev/null || true
kubectl rollout status deployment/odigos-gateway -n "$NS" --timeout=300s 2>/dev/null || true

echo "==> start UI (HTTP :$LISTEN_PORT, OTLP :4317, pprof :16061)"
export ODIGOS_UI_PPROF_PORT="${ODIGOS_UI_PPROF_PORT:-16061}"
unset ODIGOS_UI_OTLP_PORT || true
# Run in background, new session (survives terminal close on typical Linux)
setsid env ODIGOS_UI_PPROF_PORT="$ODIGOS_UI_PPROF_PORT" "$ROOT/frontend/odigos-ui-local" \
  --address 0.0.0.0 \
  --port "$LISTEN_PORT" \
  --namespace "$NS" \
  --debug </dev/null >>"$LOG" 2>&1 &
echo $! >"$ROOT/odigos-ui-local.pid"
sleep 3

if ! ss -tlnp | grep -q ":$LISTEN_PORT"; then
  echo "UI did not bind; tail $LOG" >&2
  tail -40 "$LOG" >&2
  exit 1
fi
if ! ss -tlnp | grep -q ':4317'; then
  echo "OTLP did not bind on 4317; tail $LOG" >&2
  tail -40 "$LOG" >&2
  exit 1
fi

echo "OK — listeners:"
ss -tlnp | grep -E ":$LISTEN_PORT|:4317|:16061" || true
echo ""

echo "==> smoke: curl health + GraphQL (127.0.0.1 — avoids localhost/::1 issues)"
curl -sfS "http://127.0.0.1:$LISTEN_PORT/healthz" >/dev/null
curl -sfS "http://127.0.0.1:$LISTEN_PORT/graphql" \
  -H "Content-Type: application/json" \
  --cookie /dev/null \
  -d '{"query":"{ computePlatform { computePlatformType } }"}' | grep -q '"data"'
echo "smoke OK (HTTP + GraphQL)"

echo ""
echo "Open UI:  http://${VM_IP}:$LISTEN_PORT/overview"
NETCHECK_POD="netcheck-otlp-$(date +%s)"
echo "From cluster, TCP test (unique pod name):"
echo "  kubectl run $NETCHECK_POD -n $NS --rm -i --restart=Never --image=busybox:1.36 --command -- nc -zv -w3 $VM_IP 4317"
