#!/usr/bin/env bash
# Copy profile dump files from odigos-ui to the current directory.
# Requires the UI backend to be reachable (e.g. kubectl port-forward deployment/odigos-ui 3000:3000)
# and the backend must be built with the debug profile-dumps endpoints.
#
# Usage: ./scripts/copy-profile-dumps.sh [BASE_URL]
#   BASE_URL defaults to http://localhost:3000 (use the port you forward to the Go backend)

set -e
BASE_URL="${1:-http://localhost:3000}"
OUT_DIR="${2:-./profile-dumps-from-pod}"
API="${BASE_URL}/api/debug/profile-dumps"

echo "Listing dumps from ${API} ..."
RESP=$(curl -s "${API}")
if echo "$RESP" | grep -q '"files"'; then
  if command -v jq &>/dev/null; then
    FILES=$(echo "$RESP" | jq -r '.files[]? // empty')
  else
    FILES=$(echo "$RESP" | grep -oE '"[a-zA-Z0-9_.-]+\.json"' | tr -d '"')
  fi
else
  echo "Backend did not return JSON (got HTML or error). Ensure:"
  echo "  1. Port-forward targets the Go backend: kubectl -n odigos-system port-forward deployment/odigos-ui 3000:3000"
  echo "  2. The deployed image includes the debug profile-dumps endpoints (rebuild and redeploy)."
  exit 1
fi
if [ -z "$FILES" ]; then
  echo "No dump files yet. Trigger profiling for a source so the gateway sends data; dumps will appear."
  exit 0
fi

mkdir -p "$OUT_DIR"
for f in $FILES; do
  [ -z "$f" ] && continue
  echo "Downloading $f ..."
  curl -s "${API}/${f}" -o "${OUT_DIR}/${f}"
done
echo "Saved to ${OUT_DIR}/"
ls -la "$OUT_DIR"
