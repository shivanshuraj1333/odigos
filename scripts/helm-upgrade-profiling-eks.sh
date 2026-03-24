#!/usr/bin/env bash
# One-shot Helm upgrade for profiling on EKS using public ECR profiler tags (see Makefile).
set -euo pipefail
NS="${ODIGOS_NAMESPACE:-odigos-system}"
RELEASE="${HELM_RELEASE:-odigos}"
CHART="${HELM_CHART:-$(cd "$(dirname "$0")/.." && pwd)/helm/odigos}"
SHA="${PROFILER_SHA:-$(git -C "$(dirname "$0")/.." rev-parse --short=8 HEAD)}"
# Default matches repository name "odigos/core/profiler" under registry alias "odigos" → …/odigos/odigos/core/profiler
REG="${PROFILER_ECR_IMAGE:-public.ecr.aws/odigos/odigos/core/profiler}"
# Odigos release tag for upstream multi-arch images (odiglet, instrumentor, agents). Match your cluster / chart AppVersion.
UPSTREAM_TAG="${UPSTREAM_ODIGOS_VERSION:-v1.22.0}"
UPSTREAM_REGISTRY="${UPSTREAM_ODIGOS_REGISTRY:-registry.odigos.io}"

echo "Using SHA=$SHA registry image=$REG"
echo "Upstream (odiglet, instrumentor, agents): ${UPSTREAM_REGISTRY}/*:${UPSTREAM_TAG}"
echo "Chart: $CHART"
echo "Requires an existing Odigos Helm release (preserves other values via --reuse-values)."

helm upgrade "$RELEASE" "$CHART" \
  --namespace "$NS" \
  --reuse-values \
  -f "$(dirname "$0")/profiling-enable-values.yaml" \
  --set "images.ui=${REG}:ui-${SHA}" \
  --set "images.autoscaler=${REG}:autoscaler-${SHA}" \
  --set "images.collector=${REG}:collector-${SHA}" \
  --set "images.scheduler=${REG}:scheduler-${SHA}" \
  --set "images.odiglet=${UPSTREAM_REGISTRY}/odigos-odiglet:${UPSTREAM_TAG}" \
  --set "images.instrumentor=${UPSTREAM_REGISTRY}/odigos-instrumentor:${UPSTREAM_TAG}" \
  --set "images.agents=${UPSTREAM_REGISTRY}/odigos-agents:${UPSTREAM_TAG}"
