#!/usr/bin/env bash
# Undo scripts/apply-local-ui-kit.sh (use before commit/CI).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
git restore frontend/webapp/package.json frontend/webapp/yarn.lock
(cd frontend/webapp && yarn install --ignore-engines)
echo "Restored @odigos/ui-kit to published version from package.json."
