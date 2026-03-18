#!/usr/bin/env bash
# Build (no cache) and push Odigos images to public.ecr.aws/odigos/dev/coretestbed in parallel.
# Image format: public.ecr.aws/odigos/dev/coretestbed:odigos-<component>-<7-char-sha>
#
# Export TOKEN for ECR login (use if already exported on the VM):
#   export TOKEN=$(aws ecr-public get-login-password --region us-east-1)
#   ./scripts/build-push-coretestbed-parallel.sh odiglet collector
#
# Usage (from repo root):
#   ./scripts/build-push-coretestbed-parallel.sh                    # build all: autoscaler collector ui odiglet instrumentor scheduler
#   ./scripts/build-push-coretestbed-parallel.sh ui                  # build and push only ui (single-arch)
#   ./scripts/build-push-coretestbed-parallel.sh --multi-arch odiglet collector   # multi-arch (amd64+arm64) for daemonset on mixed nodes
#   ./scripts/build-push-coretestbed-parallel.sh odiglet collector   # odiglet + collector in parallel (single-arch)
#
# Options:
#   --multi-arch  Build linux/amd64 + linux/arm64 and push (use for odiglet, collector so DaemonSet runs on all nodes).
#   Builds run in parallel; no cache (DOCKER_EXTRA_ARGS=--no-cache).

set -e
REPO="${ECR_CORETESTBED:-public.ecr.aws/odigos/dev/coretestbed}"
SHORT_SHA="$(git rev-parse --short HEAD 2>/dev/null || echo 'local')"
# Ensure 7-char sha
if [ ${#SHORT_SHA} -gt 7 ]; then
  SHORT_SHA="${SHORT_SHA:0:7}"
fi

# Parse args: optional --multi-arch and --no-cache, then component list
MULTI_ARCH=false
COMPONENTS=()
while [ $# -gt 0 ]; do
  case "$1" in
    --multi-arch)
      MULTI_ARCH=true
      shift
      ;;
    autoscaler|collector|ui|odiglet|instrumentor|scheduler)
      COMPONENTS+=("$1")
      shift
      ;;
    *)
      echo "Unknown argument: $1 (use: --multi-arch, or components: autoscaler collector ui odiglet instrumentor scheduler)" >&2
      exit 1
      ;;
  esac
done
if [ ${#COMPONENTS[@]} -eq 0 ]; then
  COMPONENTS=(autoscaler collector ui odiglet instrumentor scheduler)
fi

echo "Repo: $REPO"
echo "Tag suffix: <component>-$SHORT_SHA (e.g. odigos-odiglet-$SHORT_SHA)"
echo "Components: ${COMPONENTS[*]}"
[ "$MULTI_ARCH" = true ] && echo "Multi-arch: linux/amd64,linux/arm64"
echo ""

# Login: use TOKEN if exported, else aws ecr-public
if [ -n "${TOKEN:-}" ]; then
  echo "Logging into public.ecr.aws using TOKEN..."
  echo "$TOKEN" | docker login --username AWS --password-stdin public.ecr.aws
else
  echo "Logging into public.ecr.aws using aws ecr-public get-login-password..."
  make ecr-login
fi

# For multi-arch, ensure buildx has a builder that supports multiple platforms (e.g. docker buildx create --name multi --use)

# Build/push one component in a subshell (for parallel runs)
do_one() {
  local c="$1"
  local tag="odigos-$c-$SHORT_SHA"
  local summary desc dockerfile build_dir
  case "$c" in
    autoscaler)  summary="Autoscaler for Odigos"; desc="Autoscaler manages the installation of Odigos components." ; dockerfile="Dockerfile" ; build_dir="." ;;
    scheduler)   summary="Scheduler for Odigos"; desc="Scheduler manages the installation of OpenTelemetry Collectors with Odigos." ; dockerfile="Dockerfile" ; build_dir="." ;;
    instrumentor) summary="Instrumentor for Odigos"; desc="Instrumentor manages auto-instrumentation for workloads with Odigos." ; dockerfile="Dockerfile" ; build_dir="." ;;
    odiglet)     summary="Odiglet for Odigos"; desc="Odiglet is the core component of Odigos managing auto-instrumentation." ; dockerfile="odiglet/Dockerfile" ; build_dir="." ;;
    collector)   summary="Odigos Collector"; desc="The Odigos build of the OpenTelemetry Collector." ; dockerfile="collector/Dockerfile" ; build_dir="." ;;
    ui)          summary="UI for Odigos"; desc="UI provides the frontend webapp for managing an Odigos installation." ; dockerfile="frontend/Dockerfile" ; build_dir="." ;;
  esac
  if [ "$MULTI_ARCH" = true ]; then
    echo "[$c] Building multi-arch (amd64+arm64)..."
    make build-tag-push-ecr-image-multiarch/"$c" \
      TAG="$tag" \
      IMG_PREFIX="$REPO" \
      DOCKERFILE="$dockerfile" \
      BUILD_DIR="$build_dir" \
      DOCKER_EXTRA_ARGS="--no-cache" \
      SUMMARY="$summary" \
      DESCRIPTION="$desc"
  else
    echo "[$c] Building (no cache)..."
    make build-tag-push-ecr-image/"$c" \
      ORG=registry.odigos.io \
      TAG="$tag" \
      IMG_PREFIX="$REPO" \
      ECR_SINGLE_REPO=1 \
      DOCKERFILE="$dockerfile" \
      BUILD_DIR="$build_dir" \
      DOCKER_EXTRA_ARGS="--no-cache" \
      SUMMARY="$summary" \
      DESCRIPTION="$desc"
  fi
  echo "[$c] Pushed $REPO:$tag"
}

# Run all in parallel
for c in "${COMPONENTS[@]}"; do
  do_one "$c" &
done
wait

echo ""
echo "Done. Pushed to $REPO with tags:"
for c in "${COMPONENTS[@]}"; do
  echo "  $REPO:odigos-$c-$SHORT_SHA"
done
echo ""
echo "Set in Helm (e.g. values-profiles-override.yaml or --set images.<component>=...):"
for c in "${COMPONENTS[@]}"; do
  echo "  images.$c: $REPO:odigos-$c-$SHORT_SHA"
done
