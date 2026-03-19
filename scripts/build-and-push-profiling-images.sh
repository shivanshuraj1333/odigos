#!/usr/bin/env bash
# Build frontend (ui), collector, and autoscaler images, tag as
# public.ecr.aws/odigos/dev/coretestbed:odigos-<component>-<short-sha>, and push to ECR Public.
#
# Prerequisites: AWS CLI configured, docker, make. Run from repo root.
#
# Usage:
#   ./scripts/build-and-push-profiling-images.sh [<git-ref>]
# If <git-ref> is omitted, uses $(git rev-parse --short HEAD).
#
# After push, run helm upgrade with the printed command (profiling enabled by default).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

ECR_REPO="public.ecr.aws/odigos/dev/coretestbed"
GIT_REF="${1:-$(git rev-parse --short HEAD)}"
# Use short SHA for tag (strip any non-alphanumeric so tag is valid)
SHA="$(echo "$GIT_REF" | sed 's/[^a-zA-Z0-9]/-/g' | cut -c1-12)"

echo "=== Building and pushing images (SHA=$SHA) to $ECR_REPO ==="

# ECR Public login (us-east-1)
aws ecr-public get-login-password --region us-east-1 | docker login --username AWS --password-stdin public.ecr.aws

build_tag_push() {
  local component="$1"
  local make_target="build-${component}"
  local tag="${ECR_REPO}:odigos-${component}-${SHA}"
  local local_name="registry.odigos.io/odigos-${component}:${SHA}"
  echo "--- Build odigos-${component} ---"
  make "$make_target" TAG="$SHA" ORG=registry.odigos.io
  echo "--- Tag and push $tag ---"
  docker tag "$local_name" "$tag"
  docker push "$tag"
  echo "  Pushed $tag"
}

build_tag_push ui
build_tag_push collector
build_tag_push autoscaler

echo ""
echo "=== Images pushed. Use the following helm upgrade (profiling on by default) ==="
echo ""
echo "helm upgrade --install odigos ./helm/odigos -n odigos-system -f ./helm/odigos/values-profiles-override.yaml \\"
echo "  --set images.ui=${ECR_REPO}:odigos-ui-${SHA} \\"
echo "  --set images.collector=${ECR_REPO}:odigos-collector-${SHA} \\"
echo "  --set images.autoscaler=${ECR_REPO}:odigos-autoscaler-${SHA}"
echo ""
echo "If your release namespace is not odigos-system, add:"
echo "  --set autoscaler.profilesOtlpEndpointUI=ui.<your-namespace>.svc.cluster.local:4318"
echo "Then open Sources -> Profiling in the UI and select a source to view data."
