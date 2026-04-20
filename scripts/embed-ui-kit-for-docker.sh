#!/usr/bin/env bash
# Embeds a local @odigos/ui-kit clone into frontend/docker-build-context/ui-kit
# so docker build (context = odigos repo root) can COPY it. Run yarn build in
# ui-kit before rsync if you need a fresh lib/.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
UI_KIT_DIR="${UI_KIT_DIR:-$ROOT/../ui-kit}"
DEST="$ROOT/frontend/docker-build-context/ui-kit"
if [[ ! -f "$UI_KIT_DIR/package.json" ]]; then
  echo "embed-ui-kit-for-docker: UI_KIT_DIR=$UI_KIT_DIR is not a ui-kit repo (missing package.json)." >&2
  echo "Set UI_KIT_DIR to your ui-kit checkout, e.g. UI_KIT_DIR=/path/to/ui-kit $0" >&2
  exit 1
fi
mkdir -p "$DEST"
# Build ui-kit on the host so Docker skips rollup (faster, less RAM in the image layer).
if [[ "${UI_KIT_HOST_BUILD:-1}" == "1" ]] && command -v yarn >/dev/null 2>&1; then
  echo "embed-ui-kit-for-docker: yarn install + build in $UI_KIT_DIR"
  (cd "$UI_KIT_DIR" && yarn install --ignore-engines --network-timeout 600000 && yarn build)
fi
rsync -a --delete \
  --exclude node_modules \
  --exclude .git \
  "$UI_KIT_DIR/" "$DEST/"
echo "embed-ui-kit-for-docker: synced ui-kit to $DEST"
