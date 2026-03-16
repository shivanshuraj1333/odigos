#!/usr/bin/env bash
# Build multi-arch Odigos collector (linux/amd64 + linux/arm64) and push to ECR.
# Uses all available cores (default 32) for the Go build. Run from repo root.
#
# Usage:
#   ./scripts/build-collector-multiarch.sh [image-tag]
#
# Examples:
#   ./scripts/build-collector-multiarch.sh
#   ./scripts/build-collector-multiarch.sh odigos-collector-demo
#   MAX_PROCS=16 ./scripts/build-collector-multiarch.sh
#
# Prereqs: docker buildx, AWS CLI (for ECR login). For multi-arch you need a
# buildx builder with docker-container driver (script creates one if missing).

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

IMAGE_NAME="${1:-odigos-collector-demo}"
REGISTRY="public.ecr.aws/odigos/dev/coretestbed"
FULL_IMAGE="${REGISTRY}:${IMAGE_NAME}"

# Use all cores for Go build (default: all on this machine, e.g. 32)
MAX_PROCS="${MAX_PROCS:-$(nproc 2>/dev/null || echo 32)}"

BUILDER_NAME="odigos-multiarch"

# Ensure buildx exists and we have a multi-platform builder
if ! docker buildx version &>/dev/null; then
  echo "error: docker buildx is required"
  exit 1
fi

if ! docker buildx inspect "$BUILDER_NAME" &>/dev/null; then
  echo "Creating buildx builder '$BUILDER_NAME' (docker-container driver for multi-arch)..."
  docker buildx create --name "$BUILDER_NAME" --driver docker-container --use
else
  docker buildx use "$BUILDER_NAME"
fi

# Bootstrap QEMU for cross-platform (arm64 on amd64 host)
docker run --rm --privileged multiarch/qemu-user-static --reset -p yes 2>/dev/null || true

# Login to public ECR (required for push)
echo "Logging in to public ECR..."
aws ecr-public get-login-password --region us-east-1 | docker login --username AWS --password-stdin public.ecr.aws

echo "Building collector for linux/amd64,linux/arm64 (MAX_PROCS=$MAX_PROCS) and pushing to $FULL_IMAGE ..."
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg MAX_PROCS="$MAX_PROCS" \
  --tag "$FULL_IMAGE" \
  --push \
  --file collector/Dockerfile \
  .

echo "Done. Image: $FULL_IMAGE (multi-arch manifest)"
docker buildx imagetools inspect "$FULL_IMAGE" 2>/dev/null || true
