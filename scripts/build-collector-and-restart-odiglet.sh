#!/usr/bin/env bash
# Build the Odigos collector (with URL templatization processor fix), load into kind, restart odiglet.
# Run from repo root: ./odigos/scripts/build-collector-and-restart-odiglet.sh
# Requires: docker, kind, kubectl, make (in odigos/)
#
# First Docker build can take 45–90+ min (70+ components, full compile). Use DOCKER_BUILDKIT=1 so
# cache mounts reuse Go modules and build cache on later builds. SKIP_TESTS=1 skips tests in the image build.
set -e
export DOCKER_BUILDKIT=1
TAG="${TAG:-feat-core-609}"
ORG="${ORG:-registry.odigos.io}"
CLUSTER_NAME="${CLUSTER_NAME:-local-dev-cluster}"
SKIP_TESTS="${SKIP_TESTS:-1}"

echo "Building collector image (registry.odigos.io/odigos-collector:${TAG}) SKIP_TESTS=${SKIP_TESTS} (first build may take 45–90 min)..."
cd "$(dirname "$0")/.."
make build-collector TAG="$TAG" ORG="$ORG" BUILD_ARGS="--build-arg SKIP_TESTS=${SKIP_TESTS}"

echo "Loading collector image into kind (${CLUSTER_NAME})..."
kind load docker-image "${ORG}/odigos-collector:${TAG}" --name "${CLUSTER_NAME}"

echo "Restarting odiglet so data-collection containers pick up the new image..."
kubectl rollout restart daemonset/odiglet -n odigos-system

echo "Waiting for odiglet rollout..."
kubectl rollout status daemonset/odiglet -n odigos-system --timeout=120s

echo "Done. Generate traffic and check traces (span name and http.route should show GET /items/{id}):"
echo "  ./kv-mall/odigos-action-demo/trace-dump-and-use-templatization.sh"
echo "  # Then inspect in Signoz or Jaeger: namespace=odigos-action-demo, service=python-app"
