#!/usr/bin/env bash
# Build and push only the images that have profiling/code changes: autoscaler + collector.
# Run from repo root. On VM: set ECR_TOKEN below, then ./scripts/build-and-push-profiles-images.sh
#
# Images pushed:
#   - $ECR_REGISTRY/odigos-autoscaler:$IMAGE_TAG  (profiles pipeline + verification endpoint)
#   - $ECR_REGISTRY:$IMAGE_TAG                    (collector, multi-arch, profiles support)

set -e

# --- Set these (ECR token required for push) ---
ECR_TOKEN="${ECR_TOKEN:-}"
ECR_REGISTRY="${ECR_REGISTRY:-public.ecr.aws/odigos/dev/coretestbed}"
IMAGE_TAG="${IMAGE_TAG:-$(git rev-parse --short HEAD 2>/dev/null || echo 'profiles-dev')}"

# --- No need to edit below ---
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

# Registry host for docker login (first path component)
ECR_HOST="${ECR_REGISTRY%%/*}"

if [ -z "$ECR_TOKEN" ]; then
  echo "error: set ECR_TOKEN (e.g. paste output of: aws ecr-public get-login-password --region us-east-1)"
  echo "  export ECR_TOKEN=\$(aws ecr-public get-login-password --region us-east-1)"
  echo "  ./scripts/build-and-push-profiles-images.sh"
  exit 1
fi

echo "Logging in to $ECR_HOST ..."
echo "$ECR_TOKEN" | docker login --username AWS --password-stdin "$ECR_HOST"

# Buildx builder for multi-arch
BUILDER_NAME="odigos-multiarch"
if ! docker buildx version &>/dev/null; then
  echo "error: docker buildx is required"
  exit 1
fi
if ! docker buildx inspect "$BUILDER_NAME" &>/dev/null; then
  echo "Creating buildx builder '$BUILDER_NAME' ..."
  docker buildx create --name "$BUILDER_NAME" --driver docker-container --use
else
  docker buildx use "$BUILDER_NAME"
fi
docker run --rm --privileged multiarch/qemu-user-static --reset -p yes 2>/dev/null || true

MAX_PROCS="${MAX_PROCS:-$(nproc 2>/dev/null || echo 32)}"

echo "=== 1/2 Pushing autoscaler (profiles pipeline + verification endpoint) ==="
make push-autoscaler ORG="$ECR_REGISTRY" TAG="$IMAGE_TAG" PUSH_IMAGE=true
echo "Pushed $ECR_REGISTRY/odigos-autoscaler:$IMAGE_TAG"

echo "=== 2/2 Pushing collector (multi-arch, profiles support) ==="
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg MAX_PROCS="$MAX_PROCS" \
  --tag "$ECR_REGISTRY:$IMAGE_TAG" \
  --push \
  --file collector/Dockerfile \
  .
echo "Pushed $ECR_REGISTRY:$IMAGE_TAG"

echo ""
echo "Done. Use in Helm:"
echo "  images.collector: $ECR_REGISTRY:$IMAGE_TAG"
echo "  imagePrefix: <your-registry>/odigos  with image.tag: $IMAGE_TAG"
echo "  Or override autoscaler image to $ECR_REGISTRY/odigos-autoscaler:$IMAGE_TAG"
