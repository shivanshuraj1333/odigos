#!/usr/bin/env bash
# Build and push Odigos images to public.ecr.aws/odigos/dev/coretestbed with tag:
#   odigos-<component>-<short_sha>
# Usage:
#   ./scripts/push-ecr-coretestbed.sh              # push autoscaler only (required for profiles debug export)
#   ./scripts/push-ecr-coretestbed.sh autoscaler   # same
#   ./scripts/push-ecr-coretestbed.sh all          # push autoscaler, collector, ui, odiglet, instrumentor, scheduler
#
# Then set the new tag in helm/odigos/values-profiles-override.yaml images.autoscaler (and others if you pushed all).

set -e
REPO="${ECR_CORETESTBED:-public.ecr.aws/odigos/dev/coretestbed}"
SHORT_SHA="$(git rev-parse --short HEAD 2>/dev/null || echo 'local')"
COMPONENT="${1:-autoscaler}"

make ecr-login

case "$COMPONENT" in
  autoscaler)
    make build-tag-push-ecr-image/autoscaler \
      IMG_PREFIX="$REPO" TAG="odigos-autoscaler-$SHORT_SHA" ORG=registry.odigos.io \
      SUMMARY="Autoscaler for Odigos" DESCRIPTION="Autoscaler manages the installation of Odigos components."
    echo "Pushed $REPO:odigos-autoscaler-$SHORT_SHA"
    ;;
  collector)
    make build-tag-push-ecr-image/collector \
      IMG_PREFIX="$REPO" TAG="odigos-collector-$SHORT_SHA" ORG=registry.odigos.io \
      DOCKERFILE=collector/Dockerfile BUILD_DIR=. \
      SUMMARY="Odigos Collector" DESCRIPTION="The Odigos build of the OpenTelemetry Collector."
    echo "Pushed $REPO:odigos-collector-$SHORT_SHA"
    ;;
  ui)
    make build-tag-push-ecr-image/ui \
      IMG_PREFIX="$REPO" TAG="odigos-ui-$SHORT_SHA" ORG=registry.odigos.io \
      DOCKERFILE=frontend/Dockerfile \
      SUMMARY="UI for Odigos" DESCRIPTION="UI provides the frontend webapp for managing an Odigos installation."
    echo "Pushed $REPO:odigos-ui-$SHORT_SHA"
    ;;
  odiglet)
    make build-tag-push-ecr-image/odiglet \
      IMG_PREFIX="$REPO" TAG="odigos-odiglet-$SHORT_SHA" ORG=registry.odigos.io \
      DOCKERFILE=odiglet/Dockerfile \
      SUMMARY="Odiglet for Odigos" DESCRIPTION="Odiglet is the core component of Odigos managing auto-instrumentation."
    echo "Pushed $REPO:odigos-odiglet-$SHORT_SHA"
    ;;
  instrumentor)
    make build-tag-push-ecr-image/instrumentor \
      IMG_PREFIX="$REPO" TAG="odigos-instrumentor-$SHORT_SHA" ORG=registry.odigos.io \
      SUMMARY="Instrumentor for Odigos" DESCRIPTION="Instrumentor manages auto-instrumentation for workloads with Odigos."
    echo "Pushed $REPO:odigos-instrumentor-$SHORT_SHA"
    ;;
  scheduler)
    make build-tag-push-ecr-image/scheduler \
      IMG_PREFIX="$REPO" TAG="odigos-scheduler-$SHORT_SHA" ORG=registry.odigos.io \
      SUMMARY="Scheduler for Odigos" DESCRIPTION="Scheduler manages the installation of OpenTelemetry Collectors with Odigos."
    echo "Pushed $REPO:odigos-scheduler-$SHORT_SHA"
    ;;
  all)
    for c in autoscaler collector ui odiglet instrumentor scheduler; do
      "$0" "$c"
    done
    ;;
  *)
    echo "Usage: $0 [autoscaler|collector|ui|odiglet|instrumentor|scheduler|all]" >&2
    echo "Default: autoscaler (required for profiles debug export)" >&2
    exit 1
    ;;
esac
