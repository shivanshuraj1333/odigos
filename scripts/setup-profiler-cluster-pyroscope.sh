#!/usr/bin/env bash
# Deploy Grafana Pyroscope + OTel Collector bridge so Odigos gateway can send profiles to Pyroscope.
# Prerequisites: Odigos installed with odiglet.ebpfProfilerEnabled=true, and some workloads to profile (e.g. demo app).
# Run from repo root. Uses KUBE_CONTEXT (default: kind-profiler).

set -e
CONTEXT="${KUBE_CONTEXT:-kind-profiler}"
ODIGOS_NS="${ODIGOS_NS:-odigos-system}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

run() { kubectl --context="$CONTEXT" "$@"; }

echo "[1/4] Adding Grafana Helm repo and installing Pyroscope (single-binary)..."
helm repo add grafana https://grafana.github.io/helm-charts || true
helm repo update
run create namespace pyroscope 2>/dev/null || true
helm upgrade --install pyroscope grafana/pyroscope -n pyroscope --kube-context="$CONTEXT"

echo "[2/4] Deploying OTel Collector bridge (OTLP 4317 -> Pyroscope 4040)..."
run apply -f "$REPO_ROOT/profiler-cluster/pyroscope-otel-bridge.yaml"
sleep 20
run get pods -n pyroscope --no-headers 2>/dev/null | grep -q Running && echo "  Pyroscope namespace is up" || true

echo "[3/4] Creating Pyroscope destination in Odigos (profiles follow traces)..."
run get namespace "$ODIGOS_NS" &>/dev/null || run create namespace "$ODIGOS_NS"
sed "s/namespace: .*/namespace: $ODIGOS_NS/" "$REPO_ROOT/profiler-cluster/pyroscope-destination.yaml" | run apply -f -

echo "[4/4] Ensuring demo app is deployed for profiling..."
run apply -f "$REPO_ROOT/tests/common/apply/install-simple-demo.yaml" 2>/dev/null || true

echo ""
echo "Done. Flow: node collector (eBPF) -> gateway -> otel-collector-pyroscope:4317 -> Pyroscope:4040."
echo "Port-forward Pyroscope UI:  kubectl --context=$CONTEXT -n pyroscope port-forward svc/pyroscope 4040"
echo "Then open:  http://localhost:4040"
echo "Ensure Odigos was installed with odiglet.ebpfProfilerEnabled=true so node collectors run the eBPF profiler."
