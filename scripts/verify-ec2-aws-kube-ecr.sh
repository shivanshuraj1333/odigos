#!/usr/bin/env bash
# Quick checks from an EC2 (or laptop) that AWS identity, optional EKS kubectl,
# Docker, and ECR Public work before profiling E2E.
#
# Usage:
#   ./scripts/verify-ec2-aws-kube-ecr.sh
#   KUBECONFIG=~/.kube/eks.config EKS_CONTEXT=arn:aws:eks:...:cluster/foo ./scripts/verify-ec2-aws-kube-ecr.sh
#   PROFILER_ECR_IMAGE=public.ecr.aws/odigos/odigos/core/profiler ./scripts/verify-ec2-aws-kube-ecr.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${0}")/.." && pwd)"
PROFILER_ECR_IMAGE="${PROFILER_ECR_IMAGE:-public.ecr.aws/odigos/odigos/core/profiler}"
TEST_TAG="${TEST_TAG:-verify-pull-$(date +%s)}"

echo "== 1) AWS identity =="
aws sts get-caller-identity

echo ""
echo "== 2) kubectl (optional) =="
if ! command -v kubectl >/dev/null 2>&1; then
  echo "kubectl not installed — skip cluster check"
elif [[ -z "${KUBECONFIG:-}" ]] && ! kubectl config current-context >/dev/null 2>&1; then
  echo "No kube context — set KUBECONFIG or run: kubectl config use-context <your-eks-context>"
else
  if [[ -n "${EKS_CONTEXT:-}" ]]; then
    kubectl config use-context "$EKS_CONTEXT"
  fi
  kubectl cluster-info
  kubectl get ns --request-timeout=15s | head -20 || true
fi

echo ""
echo "== 3) Docker =="
docker info >/dev/null
echo "docker ok"

echo ""
echo "== 4) Registry reachability (replace TAG with a tag you have pushed) =="
echo "Example: docker pull ${PROFILER_ECR_IMAGE}:ui-\$(git rev-parse --short=8 HEAD)"
if [[ -n "${PROFILER_PULL_TAG:-}" ]]; then
  REF="${PROFILER_ECR_IMAGE}:${PROFILER_PULL_TAG}"
  docker pull --quiet "${REF}"
  echo "pull ok: ${REF}"
  docker rmi "${REF}" 2>/dev/null || true
else
  echo "skip (set PROFILER_PULL_TAG=ui-<sha> to test a real tag)"
fi

echo ""
echo "== 5) ECR Public login + push test (needs IAM ecr-public permissions) =="
if [[ -f "${ROOT}/scripts/aws-ecr-login-from-env.sh" ]]; then
  if "${ROOT}/scripts/aws-ecr-login-from-env.sh" public 2>/dev/null; then
    echo "from busybox (tiny): push probe tag ${TEST_TAG}"
    docker pull -q busybox:1.36.1
    docker tag busybox:1.36.1 "${PROFILER_ECR_IMAGE}:${TEST_TAG}" || true
    if docker push "${PROFILER_ECR_IMAGE}:${TEST_TAG}" 2>/dev/null; then
      echo "push ok — remove remote tag in ECR console if desired: ${TEST_TAG}"
    else
      echo "push failed (check ECR Public repo policy and iam-ecr-profiler-push.json)"
    fi
    docker rmi "${PROFILER_ECR_IMAGE}:${TEST_TAG}" 2>/dev/null || true
  else
    echo "skip push test (login failed — set scripts/eks-vm-aws.env or AWS_PROFILE)"
  fi
else
  echo "aws-ecr-login-from-env.sh missing"
fi

echo ""
echo "Done. For full profiler push: make profiler-ecr-public-login push-profiler-images-eks"
