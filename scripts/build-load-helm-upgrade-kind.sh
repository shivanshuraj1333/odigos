#!/usr/bin/env bash
# Build Odigos images, load them into the existing Kind cluster, and helm upgrade
# so components use the new images. Does not destroy the kind cluster.
#
# Usage (from odigos repo root):
#   source scripts/kind-dev.env   # optional: sets CLUSTER_NAME, TAG, ORG
#   ./scripts/build-load-helm-upgrade-kind.sh
#
# Env (defaults below; override or use scripts/kind-dev.env):
#   CLUSTER_NAME  kind cluster name (default: local-dev-cluster)
#   TAG           image tag to build and use (default: dev)
#   ORG           image org/prefix (default: registry.odigos.io)
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ODIGOS_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ODIGOS_ROOT"

# Load kind-dev.env if present (same dir as this script)
if [[ -f "$SCRIPT_DIR/kind-dev.env" ]]; then
  set -a
  # shellcheck source=kind-dev.env
  source "$SCRIPT_DIR/kind-dev.env"
  set +a
fi

TAG="${TAG:-dev}"
ORG="${ORG:-registry.odigos.io}"
CLUSTER_NAME="${CLUSTER_NAME:-local-dev-cluster}"

KIND_CLUSTERS=$(kind get clusters 2>/dev/null || true)
if ! echo "$KIND_CLUSTERS" | grep -q "^${CLUSTER_NAME}$"; then
  echo "Kind cluster '$CLUSTER_NAME' not found."
  exit 1
fi

echo "=== Build tag: $TAG, org: $ORG, kind cluster: $CLUSTER_NAME ==="

echo "=== 1/3 Building images (autoscaler, instrumentor, collector, odiglet, scheduler, ui) ==="
make build-autoscaler   TAG="$TAG" ORG="$ORG"
make build-instrumentor TAG="$TAG" ORG="$ORG"
make build-collector    TAG="$TAG" ORG="$ORG"
make build-odiglet      TAG="$TAG" ORG="$ORG"
make build-scheduler    TAG="$TAG" ORG="$ORG"
make build-ui          TAG="$TAG" ORG="$ORG"

echo "=== 2/3 Loading images into kind (cluster: $CLUSTER_NAME, all nodes) ==="
kind load docker-image "$ORG/odigos-autoscaler:$TAG"   --name "$CLUSTER_NAME"
kind load docker-image "$ORG/odigos-instrumentor:$TAG" --name "$CLUSTER_NAME"
kind load docker-image "$ORG/odigos-collector:$TAG"    --name "$CLUSTER_NAME"
kind load docker-image "$ORG/odigos-odiglet:$TAG"     --name "$CLUSTER_NAME"
kind load docker-image "$ORG/odigos-scheduler:$TAG"  --name "$CLUSTER_NAME"
kind load docker-image "$ORG/odigos-ui:$TAG"          --name "$CLUSTER_NAME"

echo "=== 3/3 Helm upgrade (odigos-system, image.tag=$TAG) ==="
helm upgrade --install odigos ./helm/odigos \
  --namespace odigos-system \
  --set image.tag="$TAG" \
  --set clusterName="$CLUSTER_NAME" \
  ${CENTRAL_BACKEND_URL:+--set centralProxy.centralBackendURL="$CENTRAL_BACKEND_URL"} \
  ${ONPREM_TOKEN:+--set onPremToken="$ONPREM_TOKEN"} \
  --wait

echo "Done. Rollouts will pick up the new images; check with: kubectl get pods -n odigos-system -w"
