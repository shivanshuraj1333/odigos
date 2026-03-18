#!/usr/bin/env bash
# Capture logs from the node collector (data-collection container in odiglet daemonset) to a file.
# Requires OTEL_LOG_LEVEL=debug (e.g. collectorNode.logLevel: "debug" in values-profiles-override.yaml).
set -e

NAMESPACE="${NAMESPACE:-odigos-system}"
OUTPUT="${OUTPUT:-node-collector-debug.log}"
TAIL="${TAIL:-5000}"

echo "Capturing node collector logs (namespace=$NAMESPACE, tail=$TAIL) to $OUTPUT ..."
: > "$OUTPUT"
for p in $(kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/name=odiglet --field-selector=status.phase=Running -o jsonpath='{.items[*].metadata.name}'); do
  kubectl logs -n "$NAMESPACE" "$p" -c data-collection --tail="$TAIL" --prefix 2>/dev/null >> "$OUTPUT" || true
done
echo "Wrote $(wc -l < "$OUTPUT") lines to $OUTPUT"
