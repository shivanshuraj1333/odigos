#!/usr/bin/env bash
# Run from repo root. Builds and loads Odigos images into kind and installs for local testing (feat/core-609).
set -e
TAG="${TAG:-feat-core-609}"
ORG="${ORG:-registry.odigos.io}"
CLUSTER_NAME="${CLUSTER_NAME:-local-dev-cluster}"

echo "Using TAG=$TAG ORG=$ORG CLUSTER_NAME=$CLUSTER_NAME"

if ! kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
  echo "Creating kind cluster..."
  kind create cluster --config=tests/common/apply/kind-config.yaml --name="$CLUSTER_NAME"
fi

echo "Building images..."
make build-images TAG="$TAG" ORG="$ORG"

echo "Loading images into kind..."
make load-to-kind TAG="$TAG" ORG="$ORG"

echo "Installing Odigos (helm) with image.tag=$TAG..."
make helm-install ODIGOS_CLI_VERSION="$TAG" CLUSTER_NAME="$CLUSTER_NAME"

echo "Done. To test URL templatization: create an Action with urlTemplatization, deploy a workload (e.g. Python), then check node collector logs / spans."
