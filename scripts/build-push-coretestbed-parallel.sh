#!/usr/bin/env bash
# Build (no cache) and push Odigos images to public.ecr.aws/odigos/dev/coretestbed in parallel.
# Image format: public.ecr.aws/odigos/dev/coretestbed:odigos-<component>-<7-char-sha>
#
# Export TOKEN at the start for ECR login (e.g. from aws ecr-public get-login-password):
#   export TOKEN=$(aws ecr-public get-login-password --region us-east-1)
#   ./scripts/build-push-coretestbed-parallel.sh ui collector autoscaler
#
# Usage (from repo root):
#   ./scripts/build-push-coretestbed-parallel.sh                    # build all: autoscaler collector ui odiglet instrumentor scheduler
#   ./scripts/build-push-coretestbed-parallel.sh ui                  # build and push only ui
#   ./scripts/build-push-coretestbed-parallel.sh ui collector        # build ui and collector in parallel
#
# Options:
#   --no-cache   Build without cache (default: always no-cache in this script)
#   Builds run in parallel; no cache (DOCKER_EXTRA_ARGS=--no-cache).

set -e
REPO="${ECR_CORETESTBED:-public.ecr.aws/odigos/dev/coretestbed}"
SHORT_SHA="$(git rev-parse --short HEAD 2>/dev/null || echo 'local')"
# Ensure 7-char sha
if [ ${#SHORT_SHA} -gt 7 ]; then
  SHORT_SHA="${SHORT_SHA:0:7}"
fi

# Parse args: optional --no-cache (we always use it), then component list
COMPONENTS=()
while [ $# -gt 0 ]; do
  case "$1" in
    autoscaler|collector|ui|odiglet|instrumentor|scheduler)
      COMPONENTS+=("$1")
      ;;
    *)
      echo "Unknown component: $1 (use: autoscaler collector ui odiglet instrumentor scheduler)" >&2
      exit 1
      ;;
  esac
  shift
done
if [ ${#COMPONENTS[@]} -eq 0 ]; then
  COMPONENTS=(autoscaler collector ui odiglet instrumentor scheduler)
fi

echo "Repo: $REPO"
echo "Tag suffix: <component>-$SHORT_SHA (e.g. odigos-ui-$SHORT_SHA)"
echo "Components: ${COMPONENTS[*]} (building in parallel, no cache)"
echo ""

# Login: use TOKEN if exported, else aws ecr-public
if [ -n "${TOKEN:-}" ]; then
  echo "Logging into public.ecr.aws using TOKEN..."
  echo "$TOKEN" | docker login --username AWS --password-stdin public.ecr.aws
else
  echo "Logging into public.ecr.aws using aws ecr-public get-login-password..."
  make ecr-login
fi

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
