#!/usr/bin/env bash
# Build and push Odigos images to your AWS ECR (private), for EKS to pull.
# Run from the Odigos repo root: ./scripts/build-push-ecr-eks.sh
#
# Prerequisites:
#   - AWS CLI configured (aws sts get-caller-identity works)
#   - Docker running
#   - ECR repos created (or IAM allows auto-create on first push):
#     odigos-ui, odigos-collector, odigos-odiglet, odigos-autoscaler, odigos-scheduler, odigos-instrumentor
#
# Usage:
#   # Push all images to private ECR with tag = git short SHA
#   ECR_REGISTRY=123456789012.dkr.ecr.us-east-1.amazonaws.com ./scripts/build-push-ecr-eks.sh
#
#   # Push all with a custom tag
#   ECR_REGISTRY=123456789012.dkr.ecr.us-east-1.amazonaws.com TAG=my-release ./scripts/build-push-ecr-eks.sh
#
#   # Push only UI (e.g. after profiling frontend changes)
#   ECR_REGISTRY=123456789012.dkr.ecr.us-east-1.amazonaws.com ./scripts/build-push-ecr-eks.sh ui
#
#   # Push only selected components
#   ECR_REGISTRY=123456789012.dkr.ecr.us-east-1.amazonaws.com ./scripts/build-push-ecr-eks.sh ui collector
#
# Then upgrade Helm so EKS uses the new images:
#   helm upgrade odigos ./helm/odigos -n odigos-system -f helm/odigos/values-profiles-override.yaml \
#     --set ui.image.repository=<ECR_REGISTRY>/odigos-ui \
#     --set ui.image.tag=<TAG> \
#     --set collector.image.repository=<ECR_REGISTRY>/odigos-collector \
#     --set collector.image.tag=<TAG>
#   (repeat for other components you pushed)

set -e

ECR_REGISTRY="${ECR_REGISTRY:?Set ECR_REGISTRY e.g. 123456789012.dkr.ecr.us-east-1.amazonaws.com}"
TAG="${TAG:-$(git rev-parse --short HEAD 2>/dev/null || echo 'dev')}"
COMPONENTS=("$@")
if [ ${#COMPONENTS[@]} -eq 0 ]; then
  COMPONENTS=(autoscaler scheduler odiglet instrumentor collector ui)
fi

# Derive AWS region from registry (e.g. us-east-1 from .dkr.ecr.us-east-1.amazonaws.com)
if [[ "$ECR_REGISTRY" =~ \.dkr\.ecr\.([^.]+)\.amazonaws\.com ]]; then
  AWS_REGION="${BASH_REMATCH[1]}"
else
  echo "ECR_REGISTRY should look like <account>.dkr.ecr.<region>.amazonaws.com" >&2
  exit 1
fi

echo "ECR_REGISTRY=$ECR_REGISTRY"
echo "TAG=$TAG"
echo "AWS_REGION=$AWS_REGION"
echo "Components: ${COMPONENTS[*]}"
echo "Logging into ECR..."
aws ecr get-login-password --region "$AWS_REGION" | docker login --username AWS --password-stdin "$ECR_REGISTRY"

push_one() {
  local name="$1"
  local summary="Odigos $name"
  local desc="Odigos $name"
  local dockerfile=""
  local build_dir="."
  case "$name" in
    autoscaler)  summary="Autoscaler for Odigos"; desc="Autoscaler manages the installation of Odigos components." ;;
    scheduler)   summary="Scheduler for Odigos"; desc="Scheduler manages the installation of OpenTelemetry Collectors with Odigos." ;;
    instrumentor) summary="Instrumentor for Odigos"; desc="Instrumentor manages auto-instrumentation for workloads with Odigos." ;;
    odiglet)     summary="Odiglet for Odigos"; desc="Odiglet is the core component of Odigos managing auto-instrumentation." ; dockerfile="odiglet/Dockerfile" ;;
    collector)   summary="Odigos Collector"; desc="The Odigos build of the OpenTelemetry Collector." ; dockerfile="collector/Dockerfile" ;;
    ui)          summary="UI for Odigos"; desc="UI provides the frontend webapp for managing an Odigos installation." ; dockerfile="frontend/Dockerfile" ;;
  esac
  echo "--- Building and pushing odigos-$name ---"
  if [ -n "$dockerfile" ]; then
    make build-tag-push-ecr-image/"$name" \
      ORG=registry.odigos.io \
      TAG="$TAG" \
      IMG_PREFIX="$ECR_REGISTRY" \
      ECR_SINGLE_REPO=0 \
      DOCKERFILE="$dockerfile" \
      BUILD_DIR="." \
      SUMMARY="$summary" \
      DESCRIPTION="$desc"
  else
    make build-tag-push-ecr-image/"$name" \
      ORG=registry.odigos.io \
      TAG="$TAG" \
      IMG_PREFIX="$ECR_REGISTRY" \
      ECR_SINGLE_REPO=0 \
      SUMMARY="$summary" \
      DESCRIPTION="$desc"
  fi
  echo "Pushed $ECR_REGISTRY/odigos-$name:$TAG"
}

for c in "${COMPONENTS[@]}"; do
  case "$c" in
    autoscaler|scheduler|instrumentor|odiglet|collector|ui)
      push_one "$c"
      ;;
    *)
      echo "Unknown component: $c (use: autoscaler scheduler instrumentor odiglet collector ui)" >&2
      exit 1
      ;;
  esac
done

echo ""
echo "Done. Images pushed to $ECR_REGISTRY with tag $TAG"
echo "Update Helm so EKS uses them. Option A (imagePrefix + tag):"
echo "  helm upgrade odigos ./helm/odigos -n odigos-system \\"
echo "    --set imagePrefix=$ECR_REGISTRY --set image.tag=$TAG \\"
echo "    -f helm/odigos/values-profiles-override.yaml  # optional"
echo "Option B (per-component images):"
echo "  helm upgrade odigos ./helm/odigos -n odigos-system \\"
echo "    --set image.tag=$TAG \\"
for c in "${COMPONENTS[@]}"; do
  echo "    --set images.$c=$ECR_REGISTRY/odigos-$c:$TAG \\"
done
echo "    -f helm/odigos/values-profiles-override.yaml  # optional"
