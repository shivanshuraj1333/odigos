#!/usr/bin/env bash
# Brute-force: enable profiling + GET for every Deployment in a namespace (default online-boutique).
# Requires: kubectl, curl, python3; UI port-forward to localhost:3000
#
# Usage:
#   kubectl port-forward -n odigos-system svc/ui 3000:3000   # terminal 1
#   PROFILE_NS=online-boutique ./scripts/profiling-brute-force-sources.sh
#
set -euo pipefail
BASE="${PROFILE_UI_URL:-http://127.0.0.1:3000}"
NS="${PROFILE_NS:-online-boutique}"

echo "UI=$BASE namespace=$NS"
echo "Waiting 2s for port-forward..."
sleep 2

ticks() {
  curl -sS "${BASE}/api/sources/$1/$2/$3/profiling" 2>/dev/null | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('flamebearer',{}).get('numTicks',-1))" 2>/dev/null || echo -1
}

# Deployments from the cluster (source of truth)
mapfile -t DEPLOYS < <(kubectl get deploy -n "$NS" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null || true)
if [[ ${#DEPLOYS[@]} -eq 0 ]]; then
  echo "No deployments in $NS (check kubectl / PROFILE_NS)" >&2
  exit 1
fi

BEST_NAME=""
BEST_TICKS=-1

for d in "${DEPLOYS[@]}"; do
  echo ""
  echo "--- Deployment/$d ---"
  curl -sS -X PUT "${BASE}/api/sources/${NS}/Deployment/${d}/profiling/enable" | python3 -m json.tool 2>/dev/null || true
  sleep 0.3
  nt=$(ticks "$NS" "Deployment" "$d")
  echo "numTicks=$nt"
  if [[ "$nt" =~ ^[0-9]+$ ]] && [[ "$nt" -gt "$BEST_TICKS" ]]; then
    BEST_TICKS=$nt
    BEST_NAME=$d
  fi
done

echo ""
echo "=== Summary ==="
echo "Best: Deployment/$BEST_NAME numTicks=$BEST_TICKS"
if [[ "$BEST_TICKS" -le 0 ]]; then
  echo "No buffered profile data yet. The API path is correct; you need OTLP profiles flowing + a few minutes of CPU load."
  echo "Try: put load on the cluster (e.g. browse the shop, hey/k6s load), wait 60s, re-run this script."
  echo "Check: curl -sS ${BASE}/api/debug/profiling-slots"
fi
