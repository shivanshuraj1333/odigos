#!/usr/bin/env bash
# Build the Odigos UI image (Next + embedded Go binary), push to the same public ECR
# repository/tag shape your Helm release uses for images.ui, then helm upgrade --reuse-values.
#
# Defaults match a typical profiler testbed install:
#   images.ui: public.ecr.aws/odigos/odigos/core/profiler:ui-ship-<sha>-dirty-v1.23.2
#
# Usage (from odigos repo root):
#   ./scripts/deploy-ui-eks.sh
#   TAG=ui-ship-abc1234-dirty-v1.23.2 ./scripts/deploy-ui-eks.sh   # explicit tag
#   UI_KIT_DIR=/path/to/ui-kit ./scripts/deploy-ui-eks.sh
#
# Optional:
#   PROFILE_EXPORT_TO_CLUSTER_UI=1  — clear profiling.gatewayUiOtlpEndpoint so the gateway uses
#                                     in-cluster ui.<namespace>:4317 (default) instead of a VM IP.
#   STOP_VM_UI_AFTER_DEPLOY=1       — pkill local ./odigos-ui-local (VM dev binary).
#
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

RELEASE="${RELEASE:-odigos}"
NAMESPACE="${NAMESPACE:-odigos-system}"
UI_KIT_DIR="${UI_KIT_DIR:-$ROOT/../ui-kit}"
PUBLIC_UI_REPO="${PUBLIC_UI_REPO:-public.ecr.aws/odigos/odigos/core/profiler}"
DEPLOY_NAME="${DEPLOY_NAME:-odigos-ui}"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi
if ! command -v helm >/dev/null 2>&1; then
  echo "helm is required" >&2
  exit 1
fi
if ! command -v aws >/dev/null 2>&1; then
  echo "aws CLI is required for ecr-public login" >&2
  exit 1
fi

GIT_SHORT=$(git rev-parse --short HEAD)
DIRTY_SUFFIX=""
git diff --quiet 2>/dev/null || DIRTY_SUFFIX="-dirty"

IMAGE_BASE_VER="${IMAGE_BASE_VER:-}"
if [[ -z "${IMAGE_BASE_VER}" ]] && helm status "$RELEASE" -n "$NAMESPACE" &>/dev/null; then
  IMAGE_BASE_VER=$(helm get values "$RELEASE" -n "$NAMESPACE" -o yaml 2>/dev/null | awk '
    /^image:/ { inimg=1; next }
    inimg && /^  tag:/ { gsub(/"/, "", $2); print $2; exit }
    inimg && /^[^ ]/ { exit }
  ')
fi
IMAGE_BASE_VER="${IMAGE_BASE_VER:-v1.23.2}"

TAG="${TAG:-ui-ship-${GIT_SHORT}${DIRTY_SUFFIX}-${IMAGE_BASE_VER}}"

echo "deploy-ui-eks: TAG=$TAG"
echo "deploy-ui-eks: push ${PUBLIC_UI_REPO}:${TAG}"

mkdir -p frontend/docker-build-context/ui-kit
UI_KIT_DIR="$UI_KIT_DIR" ./scripts/embed-ui-kit-for-docker.sh

docker build --platform linux/amd64 \
  -t "registry.odigos.io/odigos-ui:${TAG}" \
  -f frontend/Dockerfile "$ROOT" \
  --build-arg SERVICE_NAME=ui \
  --build-arg ODIGOS_VERSION="${TAG}" \
  --build-arg VERSION="${TAG}" \
  --build-arg RELEASE="${TAG}" \
  --build-arg SUMMARY="UI for Odigos" \
  --build-arg DESCRIPTION="Odigos UI"

docker tag "registry.odigos.io/odigos-ui:${TAG}" "${PUBLIC_UI_REPO}:${TAG}"

aws ecr-public get-login-password --region us-east-1 | docker login --username AWS --password-stdin public.ecr.aws

docker push "${PUBLIC_UI_REPO}:${TAG}"

HELM_SET_UI=(--set "images.ui=${PUBLIC_UI_REPO}:${TAG}")
if [[ "${PROFILE_EXPORT_TO_CLUSTER_UI:-}" == "1" ]]; then
  HELM_SET_UI+=(--set-string profiling.gatewayUiOtlpEndpoint=)
fi

helm upgrade "$RELEASE" "$ROOT/helm/odigos" \
  -n "$NAMESPACE" \
  --reuse-values \
  "${HELM_SET_UI[@]}"

kubectl rollout status "deployment/${DEPLOY_NAME}" -n "$NAMESPACE" --timeout=300s

if [[ "${RESTART_GATEWAY_AFTER_UI:-}" == "1" ]]; then
  kubectl rollout restart deployment/odigos-gateway -n "$NAMESPACE" || true
  kubectl rollout status deployment/odigos-gateway -n "$NAMESPACE" --timeout=300s || true
fi

if [[ "${STOP_VM_UI_AFTER_DEPLOY:-}" == "1" ]]; then
  pkill -f '[/]odigos-ui-local' 2>/dev/null || true
  echo "deploy-ui-eks: stopped local odigos-ui-local (if it was running)."
fi

echo "deploy-ui-eks: done. UI image:"
kubectl get deploy "$DEPLOY_NAME" -n "$NAMESPACE" -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
