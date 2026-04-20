#!/usr/bin/env bash
# Build embedded Next `out/` + Go UI, then run on the VM (avoids conflict with odigos-backend on 4317/6060).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEBAPP="$ROOT/frontend/webapp"
UI_KIT="${UI_KIT_DIR:-$ROOT/../ui-kit}"
LISTEN_PORT="${LISTEN_PORT:-3001}"
NS="${ODIGOS_NAMESPACE:-odigos-system}"
LOG="${ODIGOS_UI_LOCAL_LOG:-$ROOT/odigos-ui-local.log}"

if [[ ! -f "$UI_KIT/package.json" ]]; then
  echo "Set UI_KIT_DIR to your ui-kit checkout (default: $ROOT/../ui-kit)" >&2
  exit 1
fi

echo "==> yarn sync-ui-kit + build ($WEBAPP)"
cd "$WEBAPP"
UI_KIT_DIR="$UI_KIT" yarn sync-ui-kit
yarn build

echo "==> go build ($ROOT/frontend)"
cd "$ROOT/frontend"
go build -o odigos-ui-local .

pkill -f '[/]odigos-ui-local' 2>/dev/null || true
sleep 1

echo "==> starting odigos-ui-local (HTTP $LISTEN_PORT, OTLP 4327, pprof 16061) — log: $LOG"
export ODIGOS_UI_OTLP_PORT="${ODIGOS_UI_OTLP_PORT:-4327}"
export ODIGOS_UI_PPROF_PORT="${ODIGOS_UI_PPROF_PORT:-16061}"
nohup env ODIGOS_UI_OTLP_PORT="$ODIGOS_UI_OTLP_PORT" ODIGOS_UI_PPROF_PORT="$ODIGOS_UI_PPROF_PORT" \
  "$ROOT/frontend/odigos-ui-local" \
  --address 0.0.0.0 \
  --port "$LISTEN_PORT" \
  --namespace "$NS" \
  --debug >>"$LOG" 2>&1 &
echo $! >"$ROOT/odigos-ui-local.pid"
sleep 3
if ss -tlnp | grep -q ":$LISTEN_PORT"; then
  echo "Listening on 0.0.0.0:$LISTEN_PORT"
  curl -sI "http://127.0.0.1:$LISTEN_PORT/overview" | head -3 || true
else
  echo "Failed to listen; tail $LOG:" >&2
  tail -30 "$LOG" >&2
  exit 1
fi

PRIMARY_IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
echo ""
echo "Open: http://${PRIMARY_IP:-localhost}:$LISTEN_PORT/overview"
echo "(Profiling OTLP from the cluster uses 4317 on this VM; this local UI uses ODIGOS_UI_OTLP_PORT=$ODIGOS_UI_OTLP_PORT unless you stop odigos-backend.)"
