#!/usr/bin/env bash
# Point frontend/webapp at a sibling ui-kit checkout, build ui-kit, and yarn install.
# Revert: git restore frontend/webapp/package.json frontend/webapp/yarn.lock && (cd frontend/webapp && yarn install --ignore-engines)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WEBAPP="$ROOT/frontend/webapp"
UIKIT="${ODIGOS_UI_KIT_DIR:-$ROOT/../ui-kit}"

if [[ ! -f "$UIKIT/package.json" ]]; then
  echo "error: ui-kit not found at $UIKIT (set ODIGOS_UI_KIT_DIR)" >&2
  exit 1
fi

relpath="$(python3 -c "import os.path; print(os.path.relpath(os.path.abspath('$UIKIT'), os.path.abspath('$WEBAPP')))")"
spec="file:${relpath}"

echo "Using @odigos/ui-kit -> $spec"
(cd "$UIKIT" && yarn install --ignore-engines && yarn build)
node <<NODE
const fs = require('fs');
const p = '$WEBAPP/package.json';
const j = JSON.parse(fs.readFileSync(p, 'utf8'));
j.dependencies['@odigos/ui-kit'] = '$spec';
fs.writeFileSync(p, JSON.stringify(j, null, 2) + '\n');
NODE
(cd "$WEBAPP" && yarn install --ignore-engines)
echo "Done. In frontend/webapp run: yarn dev --port 3000"
echo "Backend (separate shell, if 4317 is busy set ODIGOS_UI_OTLP_PORT):"
echo "  cd $ROOT/frontend && ODIGOS_UI_OTLP_PORT=\${ODIGOS_UI_OTLP_PORT:-14318} ./odigos-backend --port 8085 --debug --address 0.0.0.0"
