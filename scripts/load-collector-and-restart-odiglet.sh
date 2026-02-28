#!/usr/bin/env bash
# Load the collector image into kind and restart odiglet (use after image is already built).
# Run from repo root: ./odigos/scripts/load-collector-and-restart-odiglet.sh
set -e
TAG="${TAG:-feat-core-609}"
ORG="${ORG:-registry.odigos.io}"
CLUSTER_NAME="${CLUSTER_NAME:-local-dev-cluster}"

echo "Loading collector image into kind (${CLUSTER_NAME})..."
kind load docker-image "${ORG}/odigos-collector:${TAG}" --name "${CLUSTER_NAME}"

echo "Restarting odiglet so data-collection containers pick up the new image..."
kubectl rollout restart daemonset/odiglet -n odigos-system

echo "Waiting for odiglet rollout..."
kubectl rollout status daemonset/odiglet -n odigos-system --timeout=120s

echo "Done. Generate traffic and check traces:"
echo "  ./kv-mall/odigos-action-demo/trace-dump-and-use-templatization.sh"
echo "  # In Signoz/Jaeger: namespace=odigos-action-demo, service=python-app — span name should be GET /items/{id}"
