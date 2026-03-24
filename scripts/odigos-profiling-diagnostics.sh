#!/usr/bin/env bash
# Read-only checks: why profiling is not active and what to deploy.
set -euo pipefail
NS="${ODIGOS_NAMESPACE:-odigos-system}"

need() { command -v "$1" >/dev/null 2>&1 || { echo "missing: $1" >&2; exit 1; }; }
need kubectl
kubectl get ns "$NS" >/dev/null 2>&1 || { echo "namespace $NS not found"; exit 1; }

echo "=== 1) Component images (expect custom tags for profiling branch) ==="
kubectl get deploy -n "$NS" -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.template.spec.containers[0].image}{"\n"}{end}' | grep -E 'odigos-ui|odigos-autoscaler|odigos-gateway|odigos-data-collection' || true

echo ""
echo "=== 2) odigos-configuration: profiling block ==="
if kubectl get cm odigos-configuration -n "$NS" -o jsonpath='{.data}' | grep -q config.yaml; then
  kubectl get cm odigos-configuration -n "$NS" -o jsonpath="{.data['config.yaml']}" | grep -E '^profiling:|enabled:|gatewayFileExport|otlpUiEndpoint' || echo "NO profiling: section in odigos-configuration (profiling not enabled in Helm / not merged)."
else
  echo "no config.yaml in odigos-configuration"
fi

echo ""
echo "=== 3) Gateway collector-conf: profiles pipeline ==="
GW=$(kubectl get cm odigos-gateway -n "$NS" -o jsonpath="{.data['collector-conf']}" 2>/dev/null || true)
if [[ -z "$GW" ]]; then
  echo "odigos-gateway ConfigMap missing or empty (autoscaler not reconciled)."
else
  if echo "$GW" | grep -q 'file/gateway-profiles'; then
    echo "profiles pipeline + file exporter: PRESENT"
    echo "$GW" | grep -E 'file/gateway-profiles|otlp/profiles-ui' || true
  else
    echo "profiles pipeline: ABSENT — install autoscaler image built from the profiling branch (stock v1.22.0 does not render it)."
  fi
fi

echo ""
echo "=== 4) Node collector conf: profiling receiver ==="
NC=$(kubectl get cm odigos-data-collection -n "$NS" -o jsonpath="{.data.conf}" 2>/dev/null || true)
if [[ -z "$NC" ]]; then
  echo "odigos-data-collection CM missing or empty."
else
  if echo "$NC" | grep -q '^  profiling:' || echo "$NC" | grep -q 'profiling:'; then
    echo "profiling receiver block: PRESENT"
  else
    echo "profiling receiver: ABSENT — need branch autoscaler/collector image."
  fi
fi

echo ""
echo "=== 5) effective-config: profiling ==="
kubectl get cm effective-config -n "$NS" -o jsonpath="{.data['config.yaml']}" 2>/dev/null | grep -E '^profiling:|enabled:' | head -6 || echo "effective-config missing or no profiling (scheduler not updated yet)."

echo ""
echo "=== 6) Recent gateway errors (own-telemetry to UI :4317) ==="
kubectl logs -n "$NS" deploy/odigos-gateway --tail=200 2>/dev/null | grep -iE 'error|timeout|4317|profiles' | tail -15 || echo "(no matching log lines)"

echo ""
echo "=== Summary ==="
echo "To enable end-to-end profiling + file export:"
echo "  1) Build and push multi-arch images: make profiler-ecr-public-login push-profiler-images-eks"
echo "  2) helm upgrade with profiling.enabled and images.* (see scripts/profiling-enable-values.yaml)"
echo "  3) Run: ./scripts/verify-profiling-pipeline.sh"
