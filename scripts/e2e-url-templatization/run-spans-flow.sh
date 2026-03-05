#!/usr/bin/env bash
# Spans flow: apply URL templatization for go-app only → then for both go-app and caller → cleanup.
# Use this with go-app and caller running in the kind cluster to verify templated spans in Jaeger.
#
# Prerequisites:
#   - kubectl pointing at your kind cluster
#   - Odigos installed (odigos-system), demo namespace (odigos-action-demo) with go-app and caller
#   - A trace backend (e.g. Jaeger) configured as a Destination
#
# Usage: ./run-spans-flow.sh
#
# Optional env:
#   ODIGOS_NS     default odigos-system
#   DEMO_NS       default odigos-action-demo
#   ACTION_NAME   default url-templatization-e2e
#   WAIT_SETTLE   seconds to wait after each apply (default 20)
#   SKIP_PROMPTS  set to 1 to run without "Press Enter" (e.g. for CI)

set -e
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ODIGOS_NS="${ODIGOS_NS:-odigos-system}"
DEMO_NS="${DEMO_NS:-odigos-action-demo}"
ACTION_NAME="${ACTION_NAME:-url-templatization-e2e}"
WAIT_SETTLE="${WAIT_SETTLE:-20}"
WAIT_RECONCILE="${WAIT_RECONCILE:-45}"

GO_APP_ONLY="$SCRIPT_DIR/action-url-templatization-go-app-only.yaml"
BOTH="$SCRIPT_DIR/action-url-templatization.yaml"

echo "=== URL Templatization Spans Flow (go-app only → both → cleanup) ==="
echo "  ODIGOS_NS=$ODIGOS_NS  DEMO_NS=$DEMO_NS  ACTION_NAME=$ACTION_NAME"
echo ""

# --- Clean start: remove existing action if present ---
if kubectl get action "$ACTION_NAME" -n "$ODIGOS_NS" &>/dev/null; then
  echo "Removing existing Action $ACTION_NAME for clean start..."
  kubectl delete action "$ACTION_NAME" -n "$ODIGOS_NS" --ignore-not-found --timeout=30s || true
  echo "Waiting for Processor to be removed..."
  for i in $(seq 1 "$WAIT_RECONCILE"); do
    if ! kubectl get processor odigos-url-templatization -n "$ODIGOS_NS" &>/dev/null; then
      echo "  Processor removed."
      break
    fi
    sleep 1
  done
  sleep 5
  echo ""
fi

# --- Step 1: Action for go-app only ---
echo "--- Step 1: Apply URL templatization for go-app only ---"
kubectl apply -f "$GO_APP_ONLY"
echo "Waiting up to ${WAIT_RECONCILE}s for Processor..."
for i in $(seq 1 "$WAIT_RECONCILE"); do
  if kubectl get processor odigos-url-templatization -n "$ODIGOS_NS" &>/dev/null; then
    echo "  Processor odigos-url-templatization found."
    break
  fi
  sleep 1
done
echo "Settling for ${WAIT_SETTLE}s..."
sleep "$WAIT_SETTLE"

echo ""
echo "  → Generate traffic (so spans appear in Jaeger):"
echo "    kubectl port-forward -n $DEMO_NS svc/go-app 8080:8080 &"
echo "    curl -s http://localhost:8080/items/42"
echo "  → View traces:"
echo "    kubectl port-forward -n tracing svc/jaeger 16686:16686 &"
echo "    Open http://localhost:16686  (Service: go-app; look for http.route like /items/{id})"
echo ""
if [ "${SKIP_PROMPTS:-0}" != "1" ]; then read -p "Press Enter to continue to Step 2 (apply for both go-app and caller)..."; fi

# --- Step 2: Action for both go-app and caller ---
echo ""
echo "--- Step 2: Apply URL templatization for both go-app and caller ---"
kubectl apply -f "$BOTH"
echo "Settling for ${WAIT_SETTLE}s..."
sleep "$WAIT_SETTLE"

echo ""
echo "  → Generate traffic for both:"
echo "    curl -s http://localhost:8080/items/123   # go-app"
echo "    kubectl port-forward -n $DEMO_NS svc/caller 8081:8080 &"
echo "    curl -s http://localhost:8081/items/456  # caller"
echo "  → In Jaeger: check services go-app and caller; http.route should be /items/{id}"
echo ""
if [ "${SKIP_PROMPTS:-0}" != "1" ]; then read -p "Press Enter to run cleanup (delete Action)..."; fi

# --- Step 3: Cleanup ---
echo ""
echo "--- Step 3: Cleanup — delete Action ---"
kubectl delete action "$ACTION_NAME" -n "$ODIGOS_NS" --ignore-not-found --timeout=30s || true
echo "Waiting for Processor to be removed..."
for i in $(seq 1 "$WAIT_RECONCILE"); do
  if ! kubectl get processor odigos-url-templatization -n "$ODIGOS_NS" &>/dev/null; then
    echo "  Processor odigos-url-templatization removed."
    break
  fi
  sleep 1
done
echo ""
echo "=== Spans flow complete ==="
