#!/usr/bin/env bash
# E2E test for URL templatization (CORE-609).
# Restarts Odigos components, captures logs to local files, applies/updates/deletes
# an Action CR, and records cluster state and generated configs at each step.
# Usage: ./run-e2e.sh [output-base-dir]
# Example: ./run-e2e.sh ~/odigos-e2e-runs
# Default output: <repo>/scripts/e2e-url-templatization/runs/<timestamp>

set -e
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
BASE_DIR="${1:-$SCRIPT_DIR/runs}"
RUN_DIR="$BASE_DIR/$(date +%Y%m%d-%H%M%S)"
ODIGOS_NS="${ODIGOS_NS:-odigos-system}"
DEMO_NS="${DEMO_NS:-odigos-action-demo}"
ACTION_NAME="${ACTION_NAME:-url-templatization-e2e}"
WAIT_SETTLE="${WAIT_SETTLE:-25}"
WAIT_RECONCILE="${WAIT_RECONCILE:-45}"

echo "=== E2E URL Templatization (CORE-609) ==="
echo "Run directory: $RUN_DIR"
mkdir -p "$RUN_DIR"/{logs,config-snapshots,cluster-states,analysis}

# Ensure demo namespace exists
kubectl get namespace "$DEMO_NS" &>/dev/null || kubectl create namespace "$DEMO_NS"
echo "Using namespace: $DEMO_NS"

# PIDs of background log capture jobs
LOG_PIDS=()

start_log_capture() {
  local component label logpath
  # Deployments: one log file per deployment (kubectl logs -f deploy/... follows one pod)
  for component in autoscaler instrumentor scheduler ui; do
    logpath="$RUN_DIR/logs/odigos-$component.log"
    kubectl logs -f "deployment/odigos-$component" -n "$ODIGOS_NS" --all-containers 2>&1 > "$logpath" &
    LOG_PIDS+=($!)
    echo "[$(date -Iseconds)] Started log capture: odigos-$component -> $logpath"
  done
  # Gateway = cluster collector (extension odigos_config_k8s + processor odigosurltemplate; logs to odigos-gateway.log)
  logpath="$RUN_DIR/logs/odigos-gateway.log"
  kubectl logs -f deployment/odigos-gateway -n "$ODIGOS_NS" --all-containers 2>&1 > "$logpath" &
  LOG_PIDS+=($!)
  echo "[$(date -Iseconds)] Started log capture: odigos-gateway (collector: extension + processor) -> $logpath"
  # Odiglet (one log file per pod)
  for pod in $(kubectl get pods -n "$ODIGOS_NS" -l app.kubernetes.io/name=odiglet -o name 2>/dev/null); do
    name="${pod#pod/}"
    logpath="$RUN_DIR/logs/odiglet-${name}.log"
    kubectl logs -f "pod/$name" -n "$ODIGOS_NS" --all-containers 2>&1 > "$logpath" &
    LOG_PIDS+=($!)
    echo "[$(date -Iseconds)] Started log capture: odiglet $name -> $logpath"
  done
}

stop_log_capture() {
  for pid in "${LOG_PIDS[@]}"; do
    kill "$pid" 2>/dev/null || true
  done
  LOG_PIDS=()
  echo "[$(date -Iseconds)] Stopped all log capture"
}

save_cluster_state() {
  local subdir="$1"
  local dir="$RUN_DIR/cluster-states/$subdir"
  mkdir -p "$dir"
  # Actions and Processors live in odigos-system
  kubectl get action -n "$ODIGOS_NS" -o yaml > "$dir/actions-odigos-system.yaml" 2>/dev/null || true
  kubectl get processor -n "$ODIGOS_NS" -o yaml > "$dir/processors-odigos-system.yaml" 2>/dev/null || true
  # InstrumentationConfigs and workloads in demo namespace
  kubectl get instrumentationconfig -n "$DEMO_NS" -o yaml > "$dir/instrumentationconfigs-$DEMO_NS.yaml" 2>/dev/null || true
  kubectl get deploy -n "$DEMO_NS" -o yaml > "$dir/deployments-$DEMO_NS.yaml" 2>/dev/null || true
  kubectl get pods -n "$DEMO_NS" -o wide > "$dir/pods-$DEMO_NS.txt" 2>/dev/null || true
  kubectl get instrumentationconfig -n "$ODIGOS_NS" -o yaml > "$dir/instrumentationconfigs-odigos-system.yaml" 2>/dev/null || true
  echo "[$(date -Iseconds)] Saved cluster state -> $dir"
}

wait_for_processor() {
  local timeout="${1:-$WAIT_RECONCILE}"
  local elapsed=0
  # Processor is created in the same namespace as the Action (odigos-system)
  while [ "$elapsed" -lt "$timeout" ]; do
    if kubectl get processor odigos-url-templatization -n "$ODIGOS_NS" &>/dev/null; then
      echo "[$(date -Iseconds)] Processor odigos-url-templatization found in $ODIGOS_NS"
      return 0
    fi
    sleep 5
    elapsed=$((elapsed + 5))
  done
  echo "[$(date -Iseconds)] WARNING: Processor not found after ${timeout}s" >&2
  return 1
}

wait_for_processor_gone() {
  local timeout="${1:-$WAIT_RECONCILE}"
  local elapsed=0
  while [ "$elapsed" -lt "$timeout" ]; do
    if ! kubectl get processor odigos-url-templatization -n "$ODIGOS_NS" &>/dev/null; then
      echo "[$(date -Iseconds)] Processor odigos-url-templatization removed from $ODIGOS_NS"
      return 0
    fi
    sleep 5
    elapsed=$((elapsed + 5))
  done
  echo "[$(date -Iseconds)] WARNING: Processor still present after ${timeout}s" >&2
  return 1
}

# --- Step 0: Restart components and start log capture ---
echo ""
echo "--- Step 0: Restart Odigos components and start log capture ---"
kubectl rollout restart deployment/odigos-autoscaler deployment/odigos-instrumentor deployment/odigos-scheduler deployment/odigos-ui -n "$ODIGOS_NS"
kubectl rollout restart deployment/odigos-gateway -n "$ODIGOS_NS"
kubectl rollout restart daemonset/odiglet -n "$ODIGOS_NS" 2>/dev/null || \
  kubectl patch daemonset odiglet -n "$ODIGOS_NS" -p "{\"spec\":{\"template\":{\"metadata\":{\"annotations\":{\"kubectl.kubernetes.io/restartedAt\":\"$(date -Iseconds)\"}}}}}" 2>/dev/null || true
echo "Waiting for rollouts to settle..."
kubectl rollout status deployment/odigos-autoscaler deployment/odigos-instrumentor deployment/odigos-scheduler deployment/odigos-ui deployment/odigos-gateway -n "$ODIGOS_NS" --timeout=120s
sleep "$WAIT_SETTLE"
start_log_capture
sleep 5

# --- Step 1: Initial cluster state ---
echo ""
echo "--- Step 1: Record initial cluster state ---"
save_cluster_state "01-initial"

# --- Step 2: Create Action CR ---
echo ""
echo "--- Step 2: Create Action CR (go-app + caller, template /items/{id}) ---"
kubectl apply -f "$SCRIPT_DIR/action-url-templatization.yaml"
echo "Waiting up to ${WAIT_RECONCILE}s for reconciliation (Processor + ICs)..."
wait_for_processor "$WAIT_RECONCILE" || true
sleep "$WAIT_SETTLE"

# --- Step 3: Capture configs after create ---
echo ""
echo "--- Step 3: Capture generated configs after create ---"
save_cluster_state "03-after-create"
kubectl get action "$ACTION_NAME" -n "$ODIGOS_NS" -o yaml > "$RUN_DIR/config-snapshots/02-action-after-create.yaml" 2>/dev/null || true
kubectl get processor odigos-url-templatization -n "$ODIGOS_NS" -o yaml > "$RUN_DIR/config-snapshots/03-processor-after-create.yaml" 2>/dev/null || true
kubectl get instrumentationconfig -n "$DEMO_NS" -o yaml > "$RUN_DIR/config-snapshots/03-instrumentationconfigs-after-create.yaml" 2>/dev/null || true

# --- Step 4: Update Action (target caller -> callersvc) ---
echo ""
echo "--- Step 4: Update Action (target callersvc instead of caller) ---"
kubectl apply -f "$SCRIPT_DIR/action-url-templatization-updated.yaml"
sleep "$WAIT_SETTLE"

# --- Step 5: Cluster state after update ---
echo ""
echo "--- Step 5: Record cluster state after update ---"
save_cluster_state "04-after-update"
kubectl get action "$ACTION_NAME" -n "$ODIGOS_NS" -o yaml > "$RUN_DIR/config-snapshots/04-action-after-update.yaml" 2>/dev/null || true
kubectl get instrumentationconfig -n "$DEMO_NS" -o yaml > "$RUN_DIR/config-snapshots/04-instrumentationconfigs-after-update.yaml" 2>/dev/null || true
sleep 10
save_cluster_state "05-after-update-settled"

# --- Step 6: Delete Action ---
echo ""
echo "--- Step 6: Delete Action CR ---"
kubectl delete action "$ACTION_NAME" -n "$ODIGOS_NS" --ignore-not-found --timeout=30s || true
echo "Waiting for Processor to be removed..."
wait_for_processor_gone "$WAIT_RECONCILE" || true
sleep 10

# --- Step 7: Cluster state after delete ---
echo ""
echo "--- Step 7: Record cluster state after delete ---"
save_cluster_state "07-after-delete"
kubectl get processor -n "$ODIGOS_NS" -o yaml > "$RUN_DIR/config-snapshots/07-processors-after-delete.yaml" 2>/dev/null || true
kubectl get instrumentationconfig -n "$DEMO_NS" -o yaml > "$RUN_DIR/config-snapshots/07-instrumentationconfigs-after-delete.yaml" 2>/dev/null || true

# --- Step 8: Stop log capture and analyze ---
echo ""
echo "--- Step 8: Stop log capture and analyze logs ---"
stop_log_capture
sleep 2

echo "Analyzing logs for errors and warnings..."
for f in "$RUN_DIR"/logs/*.log; do
  [ -f "$f" ] || continue
  base=$(basename "$f" .log)
  {
    echo "=== $base ==="
    grep -E "error|Error|ERROR|warn|Warn|WARN|panic|fail|Fail|FAIL" "$f" 2>/dev/null || true
    echo ""
  } >> "$RUN_DIR/analysis/errors-warnings.txt"
done

# Summary counts
{
  echo "=== Summary ==="
  echo "Error/warn line counts per file:"
  for f in "$RUN_DIR"/logs/*.log; do
    [ -f "$f" ] || continue
    count=$(grep -c -E "error|Error|ERROR|warn|Warn|WARN|panic|fail|Fail|FAIL" "$f" 2>/dev/null) || count=0
    echo "  $(basename "$f"): $count"
  done
  echo ""
  echo "Run directory: $RUN_DIR"
  echo "Cluster states: $RUN_DIR/cluster-states/"
  echo "Config snapshots: $RUN_DIR/config-snapshots/"
  echo "Logs: $RUN_DIR/logs/"
} | tee "$RUN_DIR/analysis/summary.txt"
cat "$RUN_DIR/analysis/summary.txt"

echo ""
echo "=== E2E test complete ==="
echo "Output: $RUN_DIR"
echo "Review: $RUN_DIR/analysis/errors-warnings.txt"
