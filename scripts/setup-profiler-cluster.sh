#!/usr/bin/env bash
# End-to-end setup for the profiler Kind cluster: OTel Collector OTLP sink (accepts OTLP profiles) + Odigos destination + demo app.
# Run from repo root. Uses KUBE_CONTEXT (default: kind-profiler).
# The sink receives OTLP and exports profiles/traces to file (no UI). For a UI, use Grafana Pyroscope when it adds OTLP support.

set -e
CONTEXT="${KUBE_CONTEXT:-kind-profiler}"
ODIGOS_NS="${ODIGOS_NS:-odigos-system}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

run() { kubectl --context="$CONTEXT" "$@"; }

echo "[1/3] Deploying OTel Collector OTLP sink (accepts OTLP profiles + traces, exports to file)..."
run apply -f "$REPO_ROOT/profiler-cluster/otelcol-otlp-sink.yaml"
sleep 20
run get pods -n profiling --no-headers 2>/dev/null | grep -q Running && echo "  OTLP sink is up" || true

echo "[2/3] Creating OTLP sink destination in Odigos (profiles follow traces)..."
run get namespace "$ODIGOS_NS" &>/dev/null || run create namespace "$ODIGOS_NS"
sed "s/namespace: .*/namespace: $ODIGOS_NS/" "$REPO_ROOT/profiler-cluster/otlp-sink-destination.yaml" | run apply -f -

echo "[3/3] Deploying simple demo app (default namespace) for profiling..."
run apply -f "$REPO_ROOT/tests/common/apply/install-simple-demo.yaml"

echo ""
echo "Done. Odigos sends profiles (and traces) to otlp-sink.profiling.svc.cluster.local:4317."
echo "Profiles are written to file in the collector pod (no UI). To inspect: kubectl exec -n profiling deploy/otelcol-otlp-sink -- cat /var/out/profiles.json | head"
echo "For a profiling UI with OTLP, use Grafana Pyroscope when it adds native OTLP ingest; see docs/profiling-destinations.md"
