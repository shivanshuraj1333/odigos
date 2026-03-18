#!/usr/bin/env bash
# Debug script: curl key UI/API endpoints and dump responses.
# Run while port-forward is active: kubectl -n <ns> port-forward deployment/odigos-ui 3000:3000
# Usage: ./scripts/debug-ui-endpoints.sh [BASE_URL]
# Example: ./scripts/debug-ui-endpoints.sh http://localhost:3000

set -e
BASE="${1:-http://localhost:3000}"
DUMP_DIR="${DUMP_DIR:-./ui-debug-dump}"
mkdir -p "$DUMP_DIR"
echo "Dumping endpoints from $BASE to $DUMP_DIR"

dump() {
  local path="$1"
  local out="$DUMP_DIR/$(echo "$path" | tr '/' '_').txt"
  local code
  code=$(curl -s -o "$out" -w "%{http_code}" "$BASE$path" 2>/dev/null || echo "000")
  echo "$path -> $code (saved to $out)"
  if [ "$code" != "200" ] && [ "$code" != "302" ]; then
    echo "  --- first 500 chars ---"
    head -c 500 "$out" | cat -v
    echo
  fi
}

dump_post() {
  local path="$1"
  local data="$2"
  local out="$DUMP_DIR/$(echo "POST_${path}" | tr '/' '_').txt"
  local code
  code=$(curl -s -o "$out" -w "%{http_code}" -X POST "$BASE$path" -H "Content-Type: application/json" -d "$data" 2>/dev/null || echo "000")
  echo "POST $path -> $code (saved to $out)"
  if [ "$code" != "200" ]; then
    echo "  --- first 500 chars ---"
    head -c 500 "$out" | cat -v
    echo
  fi
}

echo "=== GET endpoints ==="
dump /
dump /overview
dump /sources
dump /healthz
dump /readyz
dump /auth/csrf-token
dump /api/v1/health 2>/dev/null || true

echo "=== GraphQL ==="
dump_post /graphql '{"query":"{ __typename }"}'
dump_post /graphql '{"query":"query { workloads(filter: {}) { id { namespace kind name } } }"}'

echo "=== Done. Check $DUMP_DIR for non-200 responses (head of each file). ==="
echo "Files with non-HTML content (likely API):"
grep -L "<!DOCTYPE" "$DUMP_DIR"/*.txt 2>/dev/null || true
