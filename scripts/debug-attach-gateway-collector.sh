#!/usr/bin/env bash
# Attach Delve to the odigos-gateway collector (headless dlv in container).
# Run from repo root. Requires: dlv on PATH (go install github.com/go-delve/delve/cmd/dlv@latest),
# kubectl, and odigos-gateway running the debug image with port 2345 exposed.
set -e

REPO_ROOT="${REPO_ROOT:-$(git rev-parse --show-toplevel 2>/dev/null)}"
if [[ -z "$REPO_ROOT" ]]; then
  echo "Error: not in a git repo. Set REPO_ROOT to the odigos repo root."
  exit 1
fi
cd "$REPO_ROOT"

PORT=2345
NAMESPACE="${ODIGOS_NAMESPACE:-odigos-system}"

# Optional: start port-forward if nothing is listening on 2345
if ! command -v nc &>/dev/null; then
  echo "Note: install nc to auto-start port-forward when 2345 is not in use."
fi
if command -v nc &>/dev/null && ! nc -z 127.0.0.1 $PORT 2>/dev/null; then
  echo "Port $PORT not in use. Starting port-forward in background..."
  kubectl port-forward -n "$NAMESPACE" deployment/odigos-gateway ${PORT}:${PORT} &
  PF_PID=$!
  trap "kill $PF_PID 2>/dev/null || true" EXIT
  for i in {1..10}; do
    if nc -z 127.0.0.1 $PORT 2>/dev/null; then break; fi
    sleep 1
  done
  if ! nc -z 127.0.0.1 $PORT 2>/dev/null; then
    echo "Error: port-forward did not become ready."
    exit 1
  fi
fi

# Init file so path substitution is applied after connect (dlv terminal commands)
INIT_FILE="${REPO_ROOT}/.dlv/gateway-collector-init.txt"
mkdir -p "$(dirname "$INIT_FILE")"
cat > "$INIT_FILE" << EOF
config substitute-path /go/src/collector ${REPO_ROOT}/collector
config substitute-path /go/src/common ${REPO_ROOT}/common
EOF

if ! command -v dlv &>/dev/null; then
  echo "Error: dlv not on PATH. Install with: go install github.com/go-delve/delve/cmd/dlv@latest"
  exit 1
fi

echo "Connecting to 127.0.0.1:${PORT} (path map: /go/src/collector -> ${REPO_ROOT}/collector)"
echo ""
echo "Set breakpoints with the CONTAINER path (not your local path), e.g.:"
echo "  (dlv) break /go/src/collector/processors/odigosurltemplateprocessor/processor.go:131"
echo "  (dlv) continue"
echo ""
exec dlv connect "127.0.0.1:${PORT}" --init "$INIT_FILE"
