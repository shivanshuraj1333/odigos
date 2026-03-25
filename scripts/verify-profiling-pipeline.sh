#!/usr/bin/env bash
# Verify profiling pipeline per docs/plan: Helm-rendered config, live cluster checks, optional JSONL + UI.
# Usage:
#   ./scripts/verify-profiling-pipeline.sh --helm-only
#   ODIGOS_NAMESPACE=odigos-system ./scripts/verify-profiling-pipeline.sh
#   ./scripts/verify-profiling-pipeline.sh --skip-runtime   # static kubectl only (no exec/curl)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
NS="${ODIGOS_NAMESPACE:-odigos-system}"
VALUES_FILE="${PROFILING_HELM_VALUES:-${SCRIPT_DIR}/profiling-e2e-helm-values.example.yaml}"
CHART="${HELM_CHART:-${REPO_ROOT}/helm/odigos}"
HELM_RELEASE="${HELM_RELEASE:-odigos}"

HELM_ONLY=false
SKIP_RUNTIME=false
FAILURES=0

die() { echo "verify-profiling-pipeline: $*" >&2; exit 1; }
warn() { echo "verify-profiling-pipeline: WARN: $*" >&2; }
note() { echo "verify-profiling-pipeline: $*" >&2; }

fail() {
  warn "$*"
  FAILURES=$((FAILURES + 1))
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --helm-only) HELM_ONLY=true; shift ;;
    --skip-runtime) SKIP_RUNTIME=true; shift ;;
    --namespace|-n) NS="${2:-}"; shift 2 ;;
    *) die "unknown arg: $1" ;;
  esac
done

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

helm_check() {
  need_cmd helm
  [[ -f "$VALUES_FILE" ]] || die "values file not found: $VALUES_FILE"
  [[ -d "$CHART" ]] || die "chart dir not found: $CHART"
  note "Helm template: release=$HELM_RELEASE chart=$CHART values=$VALUES_FILE"
  local out
  out="$(helm template "$HELM_RELEASE" "$CHART" --namespace "$NS" -f "$VALUES_FILE" 2>&1)" || die "helm template failed"
  # Avoid `echo "$out" | grep -q` — large YAML triggers SIGPIPE on echo when grep exits early (pipefail).
  grep -q "profiling:" <<<"$out" || fail "rendered manifests missing profiling block"
  grep -q "gatewayFileExport" <<<"$out" || fail "rendered manifests missing gatewayFileExport"
  grep -q "nodeFileExport" <<<"$out" || fail "rendered manifests missing nodeFileExport (add profiling.nodeFileExport in values / chart defaults)"
  grep -A30 "profiling:" <<<"$out" | grep -q "enabled: true" || fail "expected profiling.enabled true under profiling block (check values file)"
  note "Helm template checks passed (odigos-configuration includes profiling / gatewayFileExport / nodeFileExport)."
}

kubectl_check() {
  need_cmd kubectl
  kubectl cluster-info >/dev/null 2>&1 || die "kubectl cannot reach cluster"
  kubectl get ns "$NS" >/dev/null 2>&1 || die "namespace not found: $NS (set ODIGOS_NAMESPACE)"

  note "Checking ConfigMap odigos-configuration..."
  local odc
  odc="$(kubectl get cm odigos-configuration -n "$NS" -o jsonpath='{.data.config\.yaml}' 2>/dev/null || true)"
  if [[ -z "$odc" ]]; then
    fail "odigos-configuration missing or empty data.config.yaml"
  else
    echo "$odc" | grep -q "profiling:" || fail "odigos-configuration config.yaml missing profiling:"
    echo "$odc" | grep -q "gatewayFileExport" || fail "odigos-configuration missing gatewayFileExport"
    echo "$odc" | grep -q "nodeFileExport" || fail "odigos-configuration missing nodeFileExport"
  fi

  note "Checking ConfigMap odigos-gateway (collector-conf)..."
  local gw
  gw="$(kubectl get cm odigos-gateway -n "$NS" -o jsonpath="{.data['collector-conf']}" 2>/dev/null || true)"
  if [[ -z "$gw" ]]; then
    warn "odigos-gateway ConfigMap missing — autoscaler may not have reconciled yet (skip gateway CM asserts)"
  else
    echo "$gw" | grep -q "profiles:" || fail "gateway collector-conf missing profiles pipeline"
    echo "$gw" | grep -q "file/gateway-profiles" || fail "gateway collector-conf missing file/gateway-profiles exporter"
    echo "$gw" | grep -q "otlp/profiles-ui" || fail "gateway collector-conf missing otlp/profiles-ui exporter"
    echo "$gw" | grep -q "receivers:" || true
  fi

  note "Checking Deployment odigos-gateway..."
  local dep
  dep="$(kubectl get deploy odigos-gateway -n "$NS" -o yaml 2>/dev/null || true)"
  if [[ -z "$dep" ]]; then
    fail "deployment odigos-gateway not found"
  else
    echo "$dep" | grep -q "service.profilesSupport" || fail "odigos-gateway missing --feature-gates=service.profilesSupport"
    echo "$dep" | grep -q "odigos-gateway-profiles-file-export" || fail "odigos-gateway missing profiles file export volume"
  fi

  note "Checking DaemonSet odiglet (node profiles file export volume when enabled)..."
  local odc_for_ds
  odc_for_ds="$(kubectl get cm odigos-configuration -n "$NS" -o jsonpath='{.data.config\.yaml}' 2>/dev/null || true)"
  local ds_odiglet
  ds_odiglet="$(kubectl get ds odiglet -n "$NS" -o yaml 2>/dev/null || true)"
  if [[ -n "$odc_for_ds" ]] && echo "$odc_for_ds" | grep -A15 "nodeFileExport:" | grep -q "enabled: true"; then
    if [[ -z "$ds_odiglet" ]]; then
      warn "DaemonSet odiglet not found (check odiglet.daemonsetName)"
    else
      echo "$ds_odiglet" | grep -q "odigos-node-profiles-file-export" || fail "odiglet DaemonSet missing odigos-node-profiles-file-export volume (nodeFileExport enabled)"
    fi
  fi

  note "Checking ConfigMap odigos-data-collection (merged node collector conf)..."
  local nc
  nc="$(kubectl get cm odigos-data-collection -n "$NS" -o jsonpath='{.data.conf}' 2>/dev/null || true)"
  if [[ -z "$nc" ]]; then
    warn "odigos-data-collection ConfigMap missing — node collector may not be materialized yet"
  else
    echo "$nc" | grep -q "profiles:" || fail "node collector conf missing profiles pipeline"
    echo "$nc" | grep -q "k8sattributes/profiles" || fail "node collector conf missing k8sattributes/profiles"
    echo "$nc" | grep -q "resource/profiles-service-name" || fail "node collector conf missing resource/profiles-service-name"
    echo "$nc" | grep -q "profiling:" || fail "node collector conf missing profiling receiver block"
    echo "$nc" | grep -q "otlp/out-cluster-collector-profiles" || fail "node collector conf missing profiles OTLP exporter"
    if echo "$odc_for_ds" | grep -A12 "nodeFileExport:" | grep -q "enabled: true"; then
      echo "$nc" | grep -q "file/node-profiles" || fail "node collector conf missing file/node-profiles (nodeFileExport enabled in odigos-configuration)"
    fi
  fi

  note "Checking ConfigMap effective-config (UI receiver)..."
  local eff
  eff="$(kubectl get cm effective-config -n "$NS" -o jsonpath='{.data.config\.yaml}' 2>/dev/null || true)"
  if [[ -z "$eff" ]]; then
    warn "effective-config missing — scheduler may not have reconciled yet"
  else
    echo "$eff" | grep -q "profiling:" || fail "effective-config missing profiling section"
  fi
}

runtime_jsonl() {
  need_cmd kubectl
  local py="$SCRIPT_DIR/parse_profiles_jsonl.py"
  [[ -f "$py" ]] || die "missing $py"
  note "Reading gateway profiles.jsonl (if present)..."
  local tmp errf
  tmp="$(mktemp)"
  errf="$(mktemp)"
  kubectl exec -n "$NS" deploy/odigos-gateway -- cat /var/odigos/profiles-export/profiles.jsonl 2>"$errf" >"$tmp" || true
  if grep -qE 'executable file not found|not found in \$PATH' "$errf" 2>/dev/null; then
    warn "gateway collector is distroless (no cat) — skip JSONL read; ConfigMap pipeline checks above are still valid"
    rm -f "$tmp" "$errf"
    return 0
  fi
  rm -f "$errf"
  if [[ ! -s "$tmp" ]]; then
    warn "profiles.jsonl not present yet or empty — generate CPU load on workloads"
    rm -f "$tmp"
    return 0
  fi
  head -n 100 "$tmp" | python3 "$py" --min-lines 1 --require-key service.name --require-key k8s.namespace.name \
    || fail "parse_profiles_jsonl could not validate JSONL resource attributes"
  rm -f "$tmp"
  note "profiles.jsonl sample lines parsed OK (k8s + service.name)."
}

runtime_jsonl_node() {
  need_cmd kubectl
  local py="$SCRIPT_DIR/parse_profiles_jsonl.py"
  [[ -f "$py" ]] || die "missing $py"
  local pod
  pod="$(kubectl get pods -n "$NS" -l app.kubernetes.io/name=odiglet -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
  if [[ -z "$pod" ]]; then
    warn "no odiglet pod — skip node profiles.jsonl"
    return 0
  fi
  note "Reading node profiles.jsonl from $pod (data-collection) if present..."
  local ntmp nerr
  ntmp="$(mktemp)"
  nerr="$(mktemp)"
  kubectl exec -n "$NS" "$pod" -c data-collection -- cat /var/odigos/node-profiles-export/profiles.jsonl 2>"$nerr" >"$ntmp" || true
  if grep -qE 'executable file not found|not found in \$PATH' "$nerr" 2>/dev/null; then
    warn "data-collection is distroless (no cat) — skip node JSONL read"
    rm -f "$ntmp" "$nerr"
    return 0
  fi
  rm -f "$nerr"
  if [[ ! -s "$ntmp" ]]; then
    warn "node profiles.jsonl not present or empty — enable profiling.nodeFileExport and wait for samples"
    rm -f "$ntmp"
    return 0
  fi
  head -n 100 "$ntmp" | python3 "$py" --min-lines 1 --audit-dictionary \
    || fail "parse_profiles_jsonl could not read node JSONL"
  rm -f "$ntmp"
  note "node profiles.jsonl sample parsed OK."
}

runtime_ui() {
  need_cmd kubectl
  command -v curl >/dev/null 2>&1 || { warn "curl missing — skip UI profiling debug endpoint"; return 0; }
  note "GET odigos-ui /api/debug/profiling-slots (port-forward + curl)..."
  local pf_port=$((31000 + RANDOM % 2000))
  kubectl port-forward --address 127.0.0.1 -n "$NS" "svc/ui" "${pf_port}:3000" >/dev/null 2>&1 &
  local pf=$!
  sleep 3
  local ok=0
  curl -sS -m 8 "http://127.0.0.1:${pf_port}/api/debug/profiling-slots" -o /tmp/odigos-profiling-slots.json 2>/dev/null || ok=1
  kill "$pf" 2>/dev/null || true
  wait "$pf" 2>/dev/null || true
  if [[ "$ok" -ne 0 ]]; then
    warn "could not reach UI profiling debug endpoint (port-forward or curl failed)"
    return 0
  fi
  if ! grep -q "activeKeys" /tmp/odigos-profiling-slots.json 2>/dev/null; then
    warn "unexpected response from profiling-slots"
    fail "profiling-slots JSON missing activeKeys"
  else
    note "UI profiling debug endpoint OK: $(head -c 200 /tmp/odigos-profiling-slots.json)..."
  fi
}

node_arch() {
  need_cmd kubectl
  note "Cluster node architectures:"
  kubectl get nodes -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.nodeInfo.architecture}{"\n"}{end}' || true
}

if [[ "$HELM_ONLY" == true ]]; then
  helm_check
  [[ "$FAILURES" -eq 0 ]] || die "helm checks failed ($FAILURES)"
  note "Done (--helm-only)."
  exit 0
fi

helm_check
kubectl_check

if [[ "$SKIP_RUNTIME" != true ]]; then
  runtime_jsonl || true
  runtime_jsonl_node || true
  runtime_ui || true
fi
node_arch

if [[ "$FAILURES" -gt 0 ]]; then
  die "$FAILURES check(s) failed"
fi
note "All checks passed."
